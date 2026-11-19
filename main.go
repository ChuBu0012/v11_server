package main

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/gin-gonic/gin"

	"v11-server/modules/alerts"
	"v11-server/modules/bot"
	"v11-server/modules/dashboard"
	"v11-server/modules/digest"
	"v11-server/modules/logs"
	"v11-server/modules/relay"
	"v11-server/modules/reminders"
	"v11-server/modules/system"
	"v11-server/modules/telemetry"
	"v11-server/modules/watchtower"
	"v11-server/shared/ai"
	"v11-server/shared/config"
	"v11-server/shared/db"
	"v11-server/shared/worker"
)

var startTime = time.Now()

var monitorInsightRate = struct {
	sync.Mutex
	last time.Time
}{}

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
	reminders         *reminders.Module
	reminderService   *reminders.Service
	watchtower        *watchtower.Module
	watchtowerService *watchtower.Service
	aiClient          *ai.Client
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
		if digestSvc != nil {
			routerOpts = append(routerOpts, bot.WithDigestFunc(func(ctx context.Context) (string, error) {
				rec, err := digestSvc.GenerateDigest(ctx, true)
				if err != nil {
					return "", err
				}
				if rec == nil {
					return "Digest was skipped (paused in settings).", nil
				}
				return rec.SummaryMD, nil
			}))
		}

		var runnerOpts []bot.AgentRunnerOption
		if binPath := os.Getenv("AGY_BIN_PATH"); binPath != "" {
			runnerOpts = append(runnerOpts, bot.WithCustomBinary(binPath))
		}
		if workDir := os.Getenv("AGY_WORK_DIR"); workDir != "" {
			runnerOpts = append(runnerOpts, bot.WithCustomWorkDir(workDir))
		}
		agentRunner := bot.NewAgentRunner(runnerOpts...)
		routerOpts = append(routerOpts, bot.WithAgentRunner(agentRunner))

		botRouter := bot.NewRouter(botClient, botAuth, torchCtrl, execRunner, statusProv, routerOpts...)
		botPoller = bot.NewPoller(botClient, botRouter)
	}
	if reminderStore != nil && reminderService == nil {
		reminderService = reminders.NewService(reminderStore, nil, 0)
	}
	var reminderMod *reminders.Module
	if reminderStore != nil {
		reminderMod = reminders.NewModule(reminderStore, cfg.ReminderPIN, reminderTelegramReady)
	}

	// OpenRouter AI client initialization
	aiClient := ai.NewClient(cfg.OpenRouterAPIKey)

	// Watchtower module initialization
	watchtowerStore, err := watchtower.NewStore(dbConn)
	if err != nil {
		log.Printf("Warning: failed to initialize watchtower store: %v", err)
	}
	var watchtowerSvc *watchtower.Service
	var watchtowerMod *watchtower.Module
	if watchtowerStore != nil {
		watchtowerSvc = watchtower.NewService(watchtowerStore, notifier, cfg.WatchtowerPIN, log.Default())
		if aiClient != nil {
			watchtowerSvc.SetIncidentSummarizer(func(ctx context.Context, w *watchtower.Watcher, res watchtower.CheckResult) string {
				metadata := ai.IncidentMetadata{
					Timestamp:      res.CheckedAt,
					WatcherName:    w.Name,
					URL:            w.URL,
					ExpectedStatus: w.ExpectedStatus,
					ActualStatus:   res.StatusCode,
					LatencyMs:      res.LatencyMs,
					ErrorSummary:   res.Error,
				}
				sum, err := ai.SummarizeIncident(ctx, aiClient, metadata)
				if err != nil || sum == nil {
					return ""
				}
				return sum.RawSummary
			})
		}
		watchtowerMod = watchtower.NewModule(watchtowerSvc)
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
		relay:             relay.New(botClient, cfg.TelegramChatID, cfg.RelayPIN),
		reminders:         reminderMod,
		reminderService:   reminderService,
		watchtower:        watchtowerMod,
		watchtowerService: watchtowerSvc,
		aiClient:          aiClient,
	}
}

func setupRouter(app *App) *gin.Engine {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	// Register modular apps
	app.dashboard.RegisterRoutes(r)
	app.system.RegisterRoutes(r)
	app.telemetry.RegisterRoutes(r.Group("/api/telemetry"))
	app.logs.RegisterRoutes(r.Group("/api/logs"))
	app.digest.RegisterRoutes(r.Group("/api/digest"))
	app.relay.RegisterRoutes(r)
	if app.reminders != nil {
		app.reminders.RegisterRoutes(r)
	}
	if app.watchtower != nil {
		app.watchtower.RegisterRoutes(r)
	}
	r.POST("/api/monitor/insight", requirePINAuth(app.cfg.RelayPIN, "X-Relay-PIN"), func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
		var evidence ai.MonitorEvidence
		if err := c.ShouldBindJSON(&evidence); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูล Monitor ไม่ถูกต้อง หรือมีขนาดใหญ่เกินไป"})
			return
		}
		if err := validateMonitorEvidence(evidence); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		monitorInsightRate.Lock()
		if time.Since(monitorInsightRate.last) < 20*time.Second {
			monitorInsightRate.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "รอสักครู่ก่อนขอ AI insight อีกครั้ง"})
			return
		}
		monitorInsightRate.last = time.Now()
		monitorInsightRate.Unlock()

		c.JSON(http.StatusOK, ai.SummarizeMonitorInsight(c.Request.Context(), app.aiClient, evidence))
	})

	// Mount Schedule NL parse endpoint: POST /api/schedule/parse (PIN protected)
	r.POST("/api/schedule/parse", requirePINAuth(app.cfg.ReminderPIN, "X-Reminder-PIN"), func(c *gin.Context) {
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
	r.POST("/api/relay/proofread", requirePINAuth(app.cfg.RelayPIN, "X-Relay-PIN"), func(c *gin.Context) {
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

	return r
}

func validateMonitorEvidence(e ai.MonitorEvidence) error {
	switch e.Status {
	case "รอข้อมูล telemetry", "ต้องตรวจสอบการเก็บข้อมูล", "พบเงื่อนไขที่ต้องติดตาม", "ระบบทำงานปกติ":
	default:
		return fmt.Errorf("สถานะ Monitor ไม่ถูกต้อง")
	}
	if e.HostRAMUsedMB < 0 || e.HostRAMUsedMB > 1024*1024 || e.HostRAMTotalMB < 0 || e.HostRAMTotalMB > 1024*1024 ||
		e.GoRAMMB < 0 || e.GoRAMMB > 1024*1024 || e.DiskFreeGB < 0 || e.DiskFreeGB > 1024*1024 ||
		e.DiskTotalGB < 0 || e.DiskTotalGB > 1024*1024 || (e.BatteryAvailable && (e.BatteryPct < 0 || e.BatteryPct > 100)) || e.UptimeSeconds < 0 {
		return fmt.Errorf("ค่าตัวเลข Monitor อยู่นอกช่วงที่รองรับ")
	}
	if len(e.Conditions) > 10 || len(e.Trend) > 24 || e.Watchtower.Total < 0 || e.Watchtower.Total > 1000 ||
		e.Watchtower.Up < 0 || e.Watchtower.Down < 0 || e.Watchtower.Paused < 0 ||
		e.Watchtower.Up+e.Watchtower.Down+e.Watchtower.Paused > e.Watchtower.Total {
		return fmt.Errorf("จำนวนรายการ Monitor อยู่นอกช่วงที่รองรับ")
	}
	for _, condition := range e.Conditions {
		switch condition {
		case "low_battery", "low_disk", "service_failure":
		default:
			return fmt.Errorf("รหัสเงื่อนไข Monitor ไม่ถูกต้อง")
		}
	}
	for _, point := range e.Trend {
		if point.GoRAMMB < 0 || point.GoRAMMB > 1024*1024 ||
			point.DiskFreeGB < 0 || point.DiskFreeGB > 1024*1024 || (point.BatteryAvailable && (point.BatteryPct < 0 || point.BatteryPct > 100)) {
			return fmt.Errorf("จุดข้อมูลแนวโน้มไม่ถูกต้อง")
		}
	}
	return nil
}

func requirePINAuth(configuredPIN, headerName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if configuredPIN == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "PIN ไม่ถูกต้อง หรือยังไม่ได้ตั้งค่า PIN"})
			return
		}

		provided := c.GetHeader(headerName)
		if provided == "" {
			provided = c.GetHeader("X-PIN")
		}
		if provided == "" {
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				provided = strings.TrimSpace(authHeader[7:])
			}
		}
		if provided == "" {
			provided = c.Query("pin")
		}

		if len(provided) != len(configuredPIN) ||
			subtle.ConstantTimeCompare([]byte(provided), []byte(configuredPIN)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "PIN ไม่ถูกต้อง หรือไม่ได้รับอนุญาต"})
			return
		}

		c.Next()
	}
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
	if app.reminderService != nil {
		app.reminderService.Start(workerCtx)
	}
	if app.watchtowerService != nil {
		app.watchtowerService.Start(workerCtx)
		if app.watchtowerService.Store() != nil {
			if purged, err := app.watchtowerService.Store().PurgeOldChecks(context.Background(), watchtower.RetentionDays); err == nil && purged > 0 {
				log.Printf("[retention:watchtower] purged %d checks older than %d days on startup", purged, watchtower.RetentionDays)
			}
		}
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
	if app.watchtowerService != nil {
		app.watchtowerService.Stop()
	}

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
