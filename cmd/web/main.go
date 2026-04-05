package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/bjornjee/agent-dashboard/internal/config"
	"github.com/bjornjee/agent-dashboard/internal/db"
	"github.com/bjornjee/agent-dashboard/internal/web"
)

func main() {
	port := flag.Int("port", 8390, "HTTP server port")
	bind := flag.String("bind", "127.0.0.1", "Bind address (use 0.0.0.0 for LAN access)")
	allowedEmail := flag.String("allowed-email", "", "Google email allowed to access (or DASHBOARD_ALLOWED_EMAIL)")
	flag.Parse()

	cfg := config.DefaultConfig()

	// Open usage database
	dbPath := cfg.Profile.StateDir + "/usage.db"
	database, err := db.OpenDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: usage DB not available: %v\n", err)
	}
	if database != nil {
		defer database.Close()
	}

	// Build auth options from flags and env vars
	opts := web.ServerOptions{
		GoogleClientID:     envOrDefault("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: envOrDefault("GOOGLE_CLIENT_SECRET", ""),
		AllowedEmail:       envOrDefault("DASHBOARD_ALLOWED_EMAIL", *allowedEmail),
		SessionSecret:      envOrDefault("DASHBOARD_SESSION_SECRET", ""),
	}

	// Require auth when binding to non-localhost
	if *bind != "127.0.0.1" && *bind != "localhost" {
		if opts.GoogleClientID == "" || opts.GoogleClientSecret == "" {
			log.Fatal("Google OAuth credentials required when binding to " + *bind +
				"\nSet GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET environment variables")
		}
		if opts.AllowedEmail == "" {
			log.Fatal("Allowed email required when binding to " + *bind +
				"\nSet DASHBOARD_ALLOWED_EMAIL or use --allowed-email flag")
		}
		if opts.SessionSecret == "" {
			log.Fatal("Session secret required when binding to " + *bind +
				"\nSet DASHBOARD_SESSION_SECRET (use: openssl rand -hex 32)")
		}
	}

	srv := web.NewServer(cfg, database, opts)
	addr := fmt.Sprintf("%s:%d", *bind, *port)
	log.Printf("Agent Dashboard web UI: http://%s", addr)
	if opts.GoogleClientID != "" {
		log.Printf("Google OAuth enabled for: %s", opts.AllowedEmail)
	} else {
		log.Printf("No auth (localhost only)")
	}
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
