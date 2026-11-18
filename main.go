package main

import (
	"context"
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
	"v11-server/modules/system"
	"v11-server/modules/telemetry"
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
	cfg       *config.Config
	db        *sql.DB
	port      string
	system    *system.Module
	logs      *logs.Module
	telemetry *telemetry.Module
	dashboard *dashboard.Module
	collector *telemetry.Collector
	worker    *worker.Worker
	botPoller *bot.Poller
	botClient *bot.Client
	digest    *digest.Module
	relay     *relay.Module
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
	if cfg.TelegramEnabled() {
		var clientOpts []bot.ClientOption
		if apiURL := os.Getenv("TELEGRAM_API_URL"); apiURL != "" {
			clientOpts = append(clientOpts, bot.WithBaseURL(apiURL))
		}
		botClient = bot.NewClient(cfg.TelegramBotToken, clientOpts...)
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

	return &App{
		cfg:       cfg,
		db:        dbConn,
		port:      port,
		system:    sysMod,
		logs:      logsMod,
		telemetry: telemetryMod,
		dashboard: dashMod,
		collector: collector,
		worker:    bgWorker,
		botPoller: botPoller,
		botClient: botClient,
		digest:    digestMod,
		relay:     relay.New(botClient, cfg.TelegramChatID, cfg.RelayPIN),
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

	return r
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
	startWeeklyEventLogPurge(workerCtx, app.logs)

	if app.botPoller != nil {
		log.Printf("[main] 🤖 Starting Telegram Sentinel Bot (Long Polling)...")
		go app.botPoller.Start(workerCtx)
	} else {
		log.Printf("[main] ℹ️ Telegram Sentinel Bot is disabled (TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID not configured)")
	}

	router := setupRouter(app)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
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
					bootMsg := fmt.Sprintf("🚀 <b>Pocket Sentinel ออนไลน์แล้ว!</b>\n"+
						"📍 <b>Version:</b> 2.1 (Async Agent Bridge)\n"+
						"🆔 <b>PID:</b> <code>%d</code>\n"+
						"💾 <b>Process RAM:</b> <code>%.1f MB</code>\n"+
						"🕒 <b>เวลา:</b> %s",
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
			downMsg := fmt.Sprintf("⚠️ <b>Pocket Sentinel กำลังรีสตาร์ท / ปิดระบบ</b>\n"+
				"🆔 <b>PID:</b> <code>%d</code> (Signal: %v)\n"+
				"🕒 <b>เวลา:</b> %s",
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
