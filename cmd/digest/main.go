package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"v11-server/modules/alerts"
	"v11-server/modules/digest"
	"v11-server/shared/db"

	_ "modernc.org/sqlite"
)

func init() {
	setupAndroidDNS()
}

func setupAndroidDNS() {
	if _, err := os.Stat("/etc/resolv.conf"); err == nil {
		return
	}
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
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp4", dnsServer)
		},
	}
}

func loadDotEnv() {
	paths := []string{".env", "/data/data/com.termux/files/home/go-app/.env"}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					val := strings.TrimSpace(parts[1])
					if os.Getenv(key) == "" {
						os.Setenv(key, val)
					}
				}
			}
			break
		}
	}
}

func main() {
	forceFlag := flag.Bool("force", false, "Force generate digest even if paused in settings")
	checkFlag := flag.Bool("check", false, "Check status and latest digest without running")
	flag.Parse()

	loadDotEnv()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data.db"
	}

	dbConn, err := db.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Fatal: Database initialization failed: %v", err)
	}
	defer dbConn.Close()

	store, err := digest.NewStore(dbConn)
	if err != nil {
		log.Fatalf("Fatal: Digest store init failed: %v", err)
	}

	ctx := context.Background()

	if *checkFlag {
		st, err := store.GetSettings(ctx)
		if err != nil {
			log.Fatalf("GetSettings failed: %v", err)
		}
		latest, err := store.GetLatestRun(ctx)
		if err != nil {
			log.Fatalf("GetLatestRun failed: %v", err)
		}
		fmt.Printf("=== Pocket Sentinel AI Digest Status ===\n")
		fmt.Printf("Enabled: %v (Schedule: %s)\n", st.Enabled, st.Schedule)
		if latest != nil {
			fmt.Printf("Last Run: #%d at %s | Status: %s | Items: %d\n",
				latest.ID, latest.StartedAt.Format("2006-01-02 15:04:05"), latest.Status, latest.ItemsCount)
			if latest.SummaryMD != "" {
				fmt.Printf("\n--- Latest Summary ---\n%s\n", latest.SummaryMD)
			}
		} else {
			fmt.Printf("No runs recorded yet.\n")
		}
		return
	}

	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		log.Fatalf("Fatal: GEMINI_API_KEY is not set in environment or .env")
	}

	teleToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	teleChat := os.Getenv("TELEGRAM_CHAT_ID")
	notifier := alerts.NewTelegramNotifier(teleToken, teleChat, 15*time.Second)

	geminiClient := digest.NewGeminiClient(geminiKey)
	svc := digest.NewService(store, geminiClient, notifier)

	runRec, err := svc.GenerateDigest(ctx, *forceFlag)
	if err != nil {
		log.Fatalf("Digest run failed: %v", err)
	}

	if runRec == nil {
		fmt.Println("ℹ️ Digest run skipped (paused in settings). Use -force to override.")
		return
	}

	fmt.Printf("✨ AI Digest #%d generated successfully!\nStatus: %s\nSummary:\n\n%s\n",
		runRec.ID, runRec.Status, runRec.SummaryMD)
}
