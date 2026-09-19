package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/psiloconvalley/404not403/internal/sensor/client"
	"github.com/psiloconvalley/404not403/internal/sensor/keychain"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	configDir := keychain.DefaultConfigDir()

	switch os.Args[1] {
	case "enroll":
		enrollCmd := flag.NewFlagSet("enroll", flag.ExitOnError)
		serverURL := enrollCmd.String("server", "https://404not403.com", "Server base URL")
		token := enrollCmd.String("token", "", "Organization enrollment API key")
		enrollCmd.Parse(os.Args[2:])

		if *token == "" {
			log.Fatal("❌ Error: --token is required for enrollment")
		}

		fmt.Println("🛰️  Enrolling 404-Sensor with server:", *serverURL)
		cfg, err := client.Enroll(*serverURL, *token)
		if err != nil {
			log.Fatalf("❌ Enrollment failed: %v", err)
		}

		if err := keychain.SaveConfig(configDir, cfg); err != nil {
			log.Fatalf("❌ Failed to save config: %v", err)
		}

		fmt.Printf("✅ Sensor enrolled successfully!\n")
		fmt.Printf("   Sensor ID:    %s\n", cfg.SensorID)
		fmt.Printf("   Org ID:       %s\n", cfg.OrgID)
		fmt.Printf("   Config Item:  %s\n", cfg.ConfigItemID)
		fmt.Printf("   Config saved: %s/config.json\n", configDir)

	case "run":
		cfg, err := keychain.LoadConfig(configDir)
		if err != nil {
			log.Fatalf("❌ Sensor is not enrolled. Run '404-sensor enroll --token <key>' first.")
		}

		c, err := client.New(cfg)
		if err != nil {
			log.Fatalf("❌ Failed to initialize sensor: %v", err)
		}

		log.Printf("🚀 404-Sensor starting daemon for Sensor ID: %s", cfg.SensorID)

		// 1. Send initial baseline snapshot
		log.Printf("📡 Dispatching initial baseline snapshot...")
		if err := c.SendBaseline(); err != nil {
			log.Printf("⚠️ Baseline checkin warning: %v", err)
		} else {
			log.Printf("✅ Baseline checkin acknowledged by server.")
		}

		// 2. Setup ticker loops
		deltaTicker := time.NewTicker(60 * time.Second)
		heartbeatTicker := time.NewTicker(time.Duration(cfg.CheckinIntervalSeconds) * time.Second)
		dailyTicker := time.NewTicker(24 * time.Hour)
		defer deltaTicker.Stop()
		defer heartbeatTicker.Stop()
		defer dailyTicker.Stop()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		for {
			select {
			case <-deltaTicker.C:
				if err := c.CheckAndSendDeltas(); err != nil {
					log.Printf("⚠️ Delta check error: %v", err)
				}
			case <-heartbeatTicker.C:
				log.Printf("💓 Sending heartbeat...")
				if err := c.SendHeartbeat(); err != nil {
					log.Printf("⚠️ Heartbeat error: %v", err)
				}
			case <-dailyTicker.C:
				log.Printf("📡 Sending scheduled daily baseline...")
				if err := c.SendBaseline(); err != nil {
					log.Printf("⚠️ Daily baseline error: %v", err)
				}
			case sig := <-sigChan:
				log.Printf("🛑 Sensor stopping gracefully on signal: %v", sig)
				os.Exit(0)
			}
		}

	case "status":
		cfg, err := keychain.LoadConfig(configDir)
		if err != nil {
			fmt.Println("❌ Sensor not enrolled.")
			os.Exit(1)
		}
		fmt.Printf("404-Sensor Status:\n")
		fmt.Printf("  Status:       Enrolled\n")
		fmt.Printf("  Sensor ID:    %s\n", cfg.SensorID)
		fmt.Printf("  Org ID:       %s\n", cfg.OrgID)
		fmt.Printf("  Server:       %s\n", cfg.ServerURL)
		fmt.Printf("  Public Key:   %s...\n", cfg.PublicKeyHex[:16])

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("404-Sensor — Zero-Context-Gap Endpoint Sensor")
	fmt.Println("\nUsage:")
	fmt.Println("  404-sensor enroll --server <url> --token <org_api_key>")
	fmt.Println("  404-sensor run")
	fmt.Println("  404-sensor status")
}
