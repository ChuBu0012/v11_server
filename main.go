package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/gin-gonic/gin"

	"v11-server/modules/alerts"
	"v11-server/modules/bot"
	"v11-server/modules/dashboard"
	"v11-server/modules/digest"
	"v11-server/modules/endpoints"
	"v11-server/modules/insight"
	"v11-server/modules/location"
	"v11-server/modules/logs"
	"v11-server/modules/relay"
	"v11-server/modules/reminders"
	"v11-server/modules/system"
	"v11-server/modules/telemetry"
	"v11-server/shared/ai"
	"v11-server/shared/config"
	"v11-server/shared/db"
	"v11-server/shared/worker"
)

var startTime = time.Now()

func init() {
	setupAndroidDNS()
}

func setupAndroidDNS() {
	// If /etc/resolv.conf exists (standard on macOS, Linux desktops/servers),
	// netgo can resolve DNS normally using system resolver settings.
	// We only override for Android/Termux where /etc/resolv.conf is absent.
	if _, err := os.Stat("/etc/resolv.conf"); err == nil {
		return
	}

	// If /etc/resolv.conf does not exist (standard on non-rooted Android/Termux),
	// pure-Go netgo defaults to 127.0.0.1:53 / [::1]:53 which is rejected.
	// We read Termux's resolv.conf or fall back to public DNS (8.8.8.8:53).
	dnsServer := "8.8.8.8:53"
	if data, err := os.ReadFile("/data/data/com.termux/files/usr/etc/resolv.conf"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "nameserver ") {
				ip := strings.TrimSpace(strings.TrimPrefix(line, "nameserver "))
				if ip != "" {
					dnsServer = ip + ":53"
					break
				}
			}
		}
	}

	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: 5 * time.Second,
			}
			return d.DialContext(ctx, "udp4", dnsServer)
		},
	}
}

type App struct {
	cfg               *config.Config
	db                *sql.DB
	port              string
	system            *system.Module
	logs              *logs.Module
	telemetry         *telemetry.Module
	dashboard         *dashboard.Module
	collector         *telemetry.Collector
	worker            *worker.Worker
	botPoller         *bot.Poller
	botClient         *bot.Client
	digest            *digest.Module
	relay             *relay.Module
	endpoints         *endpoints.Module
	reminders         *reminders.Module
	reminderService   *reminders.Service
	locationModule    *location.Module
	locationService   *location.Service
	locationStore     *location.Store
	locationRetention *worker.Worker
	aiClient          *ai.Client
	insight           *insight.Service
}

func setupApp(dbConn *sql.DB, port string, cfg *config.Config) *App {
	if cfg == nil {
		var err error
		cfg, err = config.Load()
		if err != nil {
			log.Printf("Warning: failed to load config, using defaults: %v", err)
			cfg = &config.Config{
				SampleInterval:   60 * time.Second,
				RetentionMaxAge:  72 * time.Hour,
				RetentionMaxRows: 4320,
				BatteryThreshold: 20,
				DiskThresholdGB:  5.0,
				TelegramTimeout:  10 * time.Second,
			}
		}
	}

	sysMod := system.New(startTime)
	logsMod := logs.New(dbConn)
	dashMod := dashboard.New()
	endpointsMod := endpoints.New("")
	reminderStore, reminderErr := reminders.NewStore(dbConn)
	if reminderErr != nil {
		log.Printf("Warning: failed to initialize quiet schedule: %v", reminderErr)
	}

	thresholds := alerts.Thresholds{
		BatteryPercent: float64(cfg.BatteryThreshold),
		DiskFreeGB:     cfg.DiskThresholdGB,
	}

	alertStore := alerts.NewAlertStore(dbConn)
	notifier := alerts.NewTelegramNotifier(cfg.TelegramBotToken, cfg.TelegramChatID, cfg.TelegramTimeout)
	alertMgr := alerts.NewAlertManager(alertStore, notifier, thresholds, logsMod)

	telemetryRepo := telemetry.NewRepository(dbConn)
	collector := telemetry.NewCollector(telemetryRepo, startTime,
		telemetry.WithAlertProcessor(alertMgr),
		telemetry.WithRetention(cfg.RetentionMaxAge, cfg.RetentionMaxRows),
	)

	telemetryMod := telemetry.NewModule(dbConn, collector, alertStore, thresholds, cfg.SampleInterval)

	// Digest module initialization
	digestStore, err := digest.NewStore(dbConn)
	if err != nil {
		log.Printf("Warning: failed to initialize digest store: %v", err)
	}
	var digestSvc *digest.Service
	if digestStore != nil {
		geminiClient := digest.NewGeminiClient(cfg.GeminiAPIKey)
		digestSvc = digest.NewService(digestStore, geminiClient, notifier)
	}
	digestMod := digest.New(digestSvc)

	// Background telemetry sampling worker
	bgWorker := worker.New("telemetry-collector", cfg.SampleInterval, func(ctx context.Context) error {
		_, err := collector.CollectOnce(ctx)
		return err
	})

	var botPoller *bot.Poller
	var botClient *bot.Client
	var botRouter *bot.Router
	var reminderService *reminders.Service
	var reminderTelegramReady bool
	if cfg.TelegramEnabled() {
		var clientOpts []bot.ClientOption
		if apiURL := os.Getenv("TELEGRAM_API_URL"); apiURL != "" {
			clientOpts = append(clientOpts, bot.WithBaseURL(apiURL))
		}
		botClient = bot.NewClient(cfg.TelegramBotToken, clientOpts...)
		if reminderStore != nil {
			if chatID, err := strconv.ParseInt(cfg.TelegramChatID, 10, 64); err == nil && chatID != 0 {
				reminderService = reminders.NewService(reminderStore, botClient, chatID)
				reminderTelegramReady = true
			}
		}
		botAuth := bot.NewAuthorizer(cfg.TelegramChatID, logsMod)
		torchCtrl := bot.NewTorchController()
		execRunner := bot.NewSafeExecutor()
		statusProv := bot.NewDefaultStatusProvider(startTime)
		rebootHandler := func() {
			log.Printf("[bot] /reboot: triggering graceful shutdown for PM2 resurrection...")
			p, err := os.FindProcess(os.Getpid())
			if err == nil {
				_ = p.Signal(syscall.SIGTERM)
			}
		}

		var routerOpts []bot.RouterOption
		if reminderService != nil {
			routerOpts = append(routerOpts, bot.WithCallbackHandler(reminderService.HandleCallback))
		}
		routerOpts = append(routerOpts, bot.WithRebootFunc(rebootHandler))
		if cwd, err := os.Getwd(); err == nil {
			routerOpts = append(routerOpts, bot.WithDeployer(bot.NewDeployer(cwd)))
		}
		if digestSvc != nil {
			targetChatID, _ := strconv.ParseInt(cfg.TelegramChatID, 10, 64)
			digestSvc.SetMessenger(&botDigestMessenger{
				client: botClient,
				chatID: targetChatID,
			})
			routerOpts = append(routerOpts, bot.WithCallbackRoute("dg:", func(ctx context.Context, q *bot.CallbackQuery) error {
				text, buttons, ok := digestSvc.HandleCallback(ctx, q.Data)
				if !ok {
					_ = botClient.AnswerCallbackQuery(ctx, q.ID, "ไม่พบข้อมูลสรุปข่าว", false)
					return nil
				}
				var rows [][]bot.InlineKeyboardButton
				for _, row := range buttons {
					var btnRow []bot.InlineKeyboardButton
					for _, b := range row {
						btnRow = append(btnRow, bot.InlineKeyboardButton{
							Text:         b.Text,
							CallbackData: b.Data,
						})
					}
					rows = append(rows, btnRow)
				}
				markup := &bot.InlineKeyboardMarkup{InlineKeyboard: rows}
				if q.Message != nil {
					_ = botClient.EditMessageText(ctx, q.Message.Chat.ID, q.Message.MessageID, text, markup)
				}
				_ = botClient.AnswerCallbackQuery(ctx, q.ID, "", false)
				return nil
			}))
			routerOpts = append(routerOpts, bot.WithDigestFunc(func(ctx context.Context) (string, error) {
				rec, err := digestSvc.GenerateDigest(ctx, true)
				if err != nil {
					return "", err
				}
				if rec == nil {
					return "Digest was skipped (paused in settings).", nil
				}
				return "", nil
			}))
		}

		var runnerOpts []bot.AgentRunnerOption
		if binPath := os.Getenv("AGY_BIN_PATH"); binPath != "" {
			runnerOpts = append(runnerOpts, bot.WithCustomBinary(binPath))
		}
		if workDir := os.Getenv("AGY_WORK_DIR"); workDir != "" {
			runnerOpts = append(runnerOpts, bot.WithCustomWorkDir(workDir))
		}
		if timeoutStr := os.Getenv("AGY_TIMEOUT"); timeoutStr != "" {
			if d, err := time.ParseDuration(timeoutStr); err == nil && d > 0 {
				runnerOpts = append(runnerOpts, bot.WithCustomTimeout(d))
			} else {
				log.Printf("Warning: invalid AGY_TIMEOUT %q, using default", timeoutStr)
			}
		}
		agentRunner := bot.NewAgentRunner(runnerOpts...)
		routerOpts = append(routerOpts, bot.WithAgentRunner(agentRunner))

		botRouter = bot.NewRouter(botClient, botAuth, torchCtrl, execRunner, statusProv, routerOpts...)
		botPoller = bot.NewPoller(botClient, botRouter)
	}
	if reminderStore != nil && reminderService == nil {
		reminderService = reminders.NewService(reminderStore, nil, 0)
	}
	var reminderMod *reminders.Module
	if reminderStore != nil {
		reminderMod = reminders.NewModule(reminderStore, reminderTelegramReady)
	}

	// OpenRouter AI client initialization
	aiClient := ai.NewClient(cfg.OpenRouterAPIKey)

	locationCredentials := location.Credentials{
		SecretToken: cfg.LocationSecretToken, ViewerToken: cfg.LocationViewerToken,
		SecretExplicit: cfg.LocationSecretTokenExplicit, ViewerExplicit: cfg.LocationViewerTokenExplicit,
	}
	locationStore, locationErr := location.NewStore(dbConn, locationCredentials)
	var locationService *location.Service
	var locationModule *location.Module
	var locationRetention *worker.Worker
	if locationErr != nil {
		log.Printf("Warning: failed to initialize Location Tracker: %v", locationErr)
	} else {
		locationNotifier := location.NewDiscordNotifier(os.Getenv("DISCORD_WEBHOOK_URL"), 8*time.Second)
		locationService = location.NewService(locationStore, locationNotifier)
		locationModule = location.NewModule(locationStore, locationService)
		if botRouter != nil {
			_, viewerToken := locationStore.Tokens()
			liveURL := "https://spindle-said-hemstitch.ngrok-free.dev/live/" + viewerToken
			botRouter.SetLocationFunc(func(ctx context.Context) (string, *bot.InlineKeyboardMarkup, error) {
				latest, err := locationStore.LatestPoints(ctx)
				if err != nil {
					return "", nil, err
				}
				me := latest[location.DeviceMe]
				friend := latest[location.DeviceFriend]
				return formatTelegramLocationStatus(me, friend, liveURL)
			})
		}
		locationRetention = worker.New("location-retention", 24*time.Hour, func(ctx context.Context) error {
			deleted, err := locationStore.PruneOld(ctx, time.Now().UTC().Add(-7*24*time.Hour))
			if err == nil && deleted > 0 {
				log.Printf("[retention:location] pruned %d points older than 7 days", deleted)
			}
			return err
		})
	}

	// Morning insight: deterministic facts, narrated by OpenRouter → Gemini → template.
	var insightSvc *insight.Service
	if insightStore, err := insight.NewStore(dbConn); err != nil {
		log.Printf("Warning: failed to initialize insight store: %v", err)
	} else {
		var providers []insight.Provider
		if aiClient.HasAPIKey() {
			providers = append(providers, insight.Provider{Name: "openrouter", Gen: aiClient, Timeout: 15 * time.Second})
		}
		if strings.TrimSpace(cfg.GeminiAPIKey) != "" {
			providers = append(providers, insight.Provider{Name: "gemini", Gen: digest.NewGeminiClient(cfg.GeminiAPIKey), Timeout: 30 * time.Second})
		}
		var locStore *location.Store
		if locationErr == nil {
			locStore = locationStore
		}
		var remStore *reminders.Store
		if reminderErr == nil {
			remStore = reminderStore
		}
		insightSvc = insight.NewService(insightStore, insight.NewSources(telemetryRepo, locStore, remStore), providers, notifier)
		if botRouter != nil {
			botRouter.SetInsightFunc(func(ctx context.Context) (string, error) {
				rep, _, err := insightSvc.Generate(ctx, insight.KindManual)
				return rep.TextHTML, err
			})
		}
	}

	return &App{
		cfg:               cfg,
		db:                dbConn,
		port:              port,
		system:            sysMod,
		logs:              logsMod,
		telemetry:         telemetryMod,
		dashboard:         dashMod,
		collector:         collector,
		worker:            bgWorker,
		botPoller:         botPoller,
		botClient:         botClient,
		digest:            digestMod,
		relay:             relay.New(botClient, cfg.TelegramChatID),
		endpoints:         endpointsMod,
		reminders:         reminderMod,
		reminderService:   reminderService,
		locationModule:    locationModule,
		locationService:   locationService,
		locationStore:     locationStore,
		locationRetention: locationRetention,
		aiClient:          aiClient,
		insight:           insightSvc,
	}
}

func setupRouter(app *App) *gin.Engine {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("%s - [%s] \"%s %s\" %d %s\n%s",
			param.ClientIP, param.TimeStamp.Format("2006/01/02 - 15:04:05"), param.Method,
			redactedRequestPath(param.Path), param.StatusCode, param.Latency, param.ErrorMessage)
	}))

	// Register modular apps
	app.dashboard.RegisterRoutes(r)
	app.system.RegisterRoutes(r)
	app.telemetry.RegisterRoutes(r.Group("/api/telemetry"))
	app.logs.RegisterRoutes(r.Group("/api/logs"))
	app.digest.RegisterRoutes(r.Group("/api/digest"))
	app.relay.RegisterRoutes(r)
	if app.endpoints != nil {
		app.endpoints.RegisterRoutes(r.Group("/api"))
	}
	if app.reminders != nil {
		app.reminders.RegisterRoutes(r)
	}
	if app.locationModule != nil {
		app.locationModule.RegisterRoutes(r)
		r.Match([]string{"GET", "HEAD"}, "/api/location/viewer-token", func(c *gin.Context) {
			token := app.locationModule.ViewerToken()
			if token == "" {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			c.JSON(http.StatusOK, gin.H{"token": token})
		})
		r.Match([]string{"GET", "HEAD"}, "/live/:token", app.locationModule.ViewerPage(app.dashboard.HandleLiveMap))
		r.Match([]string{"GET", "HEAD"}, "/live", func(c *gin.Context) {
			token := app.locationModule.ViewerToken()
			if token == "" {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			c.Redirect(http.StatusTemporaryRedirect, "/live/"+token)
		})
		r.Match([]string{"GET", "HEAD"}, "/map", func(c *gin.Context) {
			token := app.locationModule.ViewerToken()
			if token == "" {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			c.Redirect(http.StatusTemporaryRedirect, "/live/"+token)
		})
	}
	// Mount Schedule NL parse endpoint: POST /api/schedule/parse (PIN protected)
	r.POST("/api/schedule/parse", func(c *gin.Context) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
			return
		}
		if strings.TrimSpace(req.Prompt) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prompt cannot be empty"})
			return
		}
		result, err := ai.ParseSchedulePrompt(c.Request.Context(), app.aiClient, req.Prompt, "Asia/Bangkok")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	})

	// Mount Telegram Bridge proofread endpoint: POST /api/relay/proofread (PIN protected)
	r.POST("/api/relay/proofread", func(c *gin.Context) {
		var req struct {
			Text string `json:"text"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
			return
		}
		if strings.TrimSpace(req.Text) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "text cannot be empty"})
			return
		}
		result, err := ai.ProofreadText(c.Request.Context(), app.aiClient, req.Text)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	})

	// Reverse Proxy to local Gatus health dashboard (port 8081)
	gatusTarget, _ := url.Parse("http://127.0.0.1:8081")
	gatusProxy := httputil.NewSingleHostReverseProxy(gatusTarget)
	origDirector := gatusProxy.Director
	gatusProxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = gatusTarget.Host
		if strings.HasPrefix(req.URL.Path, "/status") {
			req.URL.Path = strings.TrimPrefix(req.URL.Path, "/status")
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		}
	}

	r.Any("/status/*path", func(c *gin.Context) {
		gatusProxy.ServeHTTP(c.Writer, c.Request)
	})
	r.GET("/status", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/status/")
	})

	// Forward Gatus SPA bundles and API calls to avoid 404s
	r.Any("/api/v1/*path", func(c *gin.Context) {
		gatusProxy.ServeHTTP(c.Writer, c.Request)
	})
	r.Any("/js/*path", func(c *gin.Context) {
		gatusProxy.ServeHTTP(c.Writer, c.Request)
	})
	r.Any("/css/*path", func(c *gin.Context) {
		gatusProxy.ServeHTTP(c.Writer, c.Request)
	})

	return r
}

func redactedRequestPath(path string) string {
	if strings.HasPrefix(path, "/live/") {
		return "/live/[token]"
	}
	if strings.HasPrefix(path, "/api/location/report?") {
		return "/api/location/report?[redacted]"
	}
	if strings.HasPrefix(path, "/api/location/live?") {
		return "/api/location/live?[redacted]"
	}
	return path
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data.db"
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Fatal: Configuration error: %v", err)
	}

	log.Printf("Loaded config: %s", cfg.String())

	dbConn, err := db.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Fatal: Database initialization failed: %v", err)
	}
	defer dbConn.Close()

	app := setupApp(dbConn, port, cfg)
	_ = app.logs.RecordLog("info", "Server starting up", fmt.Sprintf("port=%s, db=%s", port, dbPath))

	// Initial collection sample on startup
	if _, err := app.collector.CollectOnce(context.Background()); err != nil {
		log.Printf("Warning: initial collection error: %v", err)
	}

	// Start background worker with cancellable context
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	app.worker.Start(workerCtx)
	if app.insight != nil {
		go app.insight.RunScheduler(workerCtx)
	}
	if app.locationRetention != nil {
		app.locationRetention.Start(workerCtx)
	}
	if app.reminderService != nil {
		app.reminderService.Start(workerCtx)
	}
	startWeeklyEventLogPurge(workerCtx, app.logs)

	if app.botPoller != nil {
		log.Printf("[main] 🤖 Starting Telegram Sentinel Bot (Long Polling)...")
		go app.botPoller.Start(workerCtx)
	} else {
		log.Printf("[main] ℹ️ Telegram Sentinel Bot is disabled (TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID not configured)")
	}

	router := setupRouter(app)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("🚀 Pocket Sentinel running on port :%s (DB: %s)", port, dbPath)
		if app.botClient != nil && app.cfg.TelegramChatID != "" {
			go func() {
				time.Sleep(1 * time.Second)
				chatID, _ := strconv.ParseInt(app.cfg.TelegramChatID, 10, 64)
				if chatID != 0 {
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					ramMB := float64(m.Alloc) / 1024 / 1024
					bootMsg := fmt.Sprintf("🚀 <b>Pocket Sentinel พร้อมใช้งาน</b>\n\n"+
						"<b>เวอร์ชัน:</b> 2.1 · Async Agent Bridge\n"+
						"<b>PID:</b> <code>%d</code>\n"+
						"<b>หน่วยความจำ:</b> <code>%.1f MB</code>\n"+
						"<b>เวลา:</b> %s",
						os.Getpid(), ramMB, time.Now().Format("2006-01-02 15:04:05"))
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_, _ = app.botClient.SendMessage(ctx, chatID, bootMsg, nil)
				}
			}()
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	sig := <-stopChan
	log.Printf("Received signal: %v. Initiating graceful shutdown...", sig)

	_ = app.logs.RecordLog("info", "Server shutting down", fmt.Sprintf("signal=%v", sig))

	if app.botClient != nil && app.cfg.TelegramChatID != "" {
		chatID, _ := strconv.ParseInt(app.cfg.TelegramChatID, 10, 64)
		if chatID != 0 {
			downMsg := fmt.Sprintf("⚠️ <b>Pocket Sentinel กำลังหยุดทำงาน</b>\n\n"+
				"<b>PID:</b> <code>%d</code>\n"+
				"<b>Signal:</b> <code>%v</code>\n"+
				"<b>เวลา:</b> %s",
				os.Getpid(), sig, time.Now().Format("2006-01-02 15:04:05"))
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, _ = app.botClient.SendMessage(ctx, chatID, downMsg, nil)
		}
	}

	// 1. Cancel background worker
	cancelWorker()

	// 2. Gracefully stop HTTP server
	shutdownCtx, cancelHTTP := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelHTTP()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Graceful shutdown error: %v", err)
	} else {
		log.Printf("Server shutdown completed successfully.")
	}
}

func startWeeklyEventLogPurge(ctx context.Context, eventLogs *logs.Module) {
	go func() {
		location, err := time.LoadLocation("Asia/Bangkok")
		if err != nil {
			location = time.FixedZone("Asia/Bangkok", 7*60*60)
		}

		for {
			nextRun := nextMondayAtThree(time.Now(), location)
			wait := time.Until(nextRun)
			if wait < 0 {
				wait = 0
			}
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			case <-timer.C:
			}

			purgeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			deleted, purgeErr := eventLogs.PurgeOlderThan(purgeCtx, time.Now().UTC().Add(-7*24*time.Hour))
			cancel()
			if purgeErr != nil {
				log.Printf("[retention:event-logs] purge failed: %v", purgeErr)
			} else {
				log.Printf("[retention:event-logs] purged %d entries older than 7 days", deleted)
			}
		}
	}()
}

func nextMondayAtThree(now time.Time, location *time.Location) time.Time {
	localNow := now.In(location)
	daysUntilMonday := (int(time.Monday) - int(localNow.Weekday()) + 7) % 7
	next := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+daysUntilMonday, 3, 0, 0, 0, location)
	if !next.After(localNow) {
		next = next.AddDate(0, 0, 7)
	}
	return next
}

type botDigestMessenger struct {
	client *bot.Client
	chatID int64
}

func (m *botDigestMessenger) SendMenu(ctx context.Context, text string, buttons [][]digest.Button) error {
	var rows [][]bot.InlineKeyboardButton
	for _, row := range buttons {
		var btnRow []bot.InlineKeyboardButton
		for _, b := range row {
			btnRow = append(btnRow, bot.InlineKeyboardButton{
				Text:         b.Text,
				CallbackData: b.Data,
			})
		}
		rows = append(rows, btnRow)
	}
	markup := &bot.InlineKeyboardMarkup{InlineKeyboard: rows}
	_, err := m.client.SendMessage(ctx, m.chatID, text, markup)
	return err
}
