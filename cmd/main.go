package main

import (
	"YrestAPI/internal/config"
	"YrestAPI/internal/db"
	"YrestAPI/internal/logger"
	"YrestAPI/internal/model"
	"YrestAPI/internal/router"
	"flag"
	"log"
	"net/http"

	"fmt"
	"os"
)

func startupFatal(code string, err error) {
	msg := err.Error()
	if err == nil {
		msg = "unknown error"
	}
	fmt.Fprintf(os.Stderr, "startup failed (%s): %s\n", code, msg)
	fmt.Fprintf(os.Stderr, "see log/app.log for details\n")
	os.Exit(1)
}

func main() {
	debugFlag := flag.Bool("d", false, "enable debug logging")
	flag.Parse()

	cfg := config.LoadConfig()
	if err := logger.Init("."); err != nil {
		startupFatal("log_init_failed", err)
	}
	logger.SetDebug(*debugFlag)

	// PostgreSQL

	if err := db.InitPostgres(cfg.PostgresDSN); err != nil {
		logger.Error("postgres_init_failed", map[string]any{"error": err.Error()})
		startupFatal("postgres_init_failed", err)
	}
	logger.Info("postgres_connected", nil)

	// Initialize registry
	if err := model.InitRegistry(cfg.ModelsDir); err != nil {
		logger.Error("registry_init_failed", map[string]any{"error": err.Error()})
		startupFatal("registry_init_failed", err)
	}
	model.SetAliasCacheMaxBytes(cfg.AliasCache.MaxBytes)
	logger.Info("models_initialized", nil)
	// Load locales if available
	// This is optional, so we handle errors gracefully
	// If locales are not found, we just disable localization
	if err := model.LoadLocales(cfg.Locale); err != nil {
		logger.Warn("locales_disabled", map[string]any{"error": err.Error()})
		model.HasLocales = false
	} else {
		model.HasLocales = true
	}
	// Initialize routes
	if err := router.InitRoutes(cfg); err != nil {
		logger.Error("router_init_failed", map[string]any{"error": err.Error()})
		startupFatal("router_init_failed", err)
	}
	// Start HTTP server
	logger.Info("server_start", map[string]any{"port": cfg.Port})
	log.Printf("🚀 Starting server on port %s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		logger.Error("server_error", map[string]any{"error": err.Error()})
		startupFatal("server_error", err)
	}
}
