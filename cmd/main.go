package main

import (
	"YrestAPI/internal/config"
	"YrestAPI/internal/db"
	"YrestAPI/internal/model"
	"YrestAPI/internal/router"
	"log"
	"net/http"

	"fmt"
)

func main() {
	cfg := config.LoadConfig()

	// PostgreSQL
	if err := db.InitPostgres(cfg.PostgresDSN); err != nil {
		log.Fatalf("❌ PostgreSQL init failed: %v", err)
	}
	log.Println("✅ Connected to PostgreSQL")

	// Redis
    db.InitRedis(cfg.RedisAddr)

	if err := db.PingRedis(); err != nil {
		log.Fatalf("❌ Redis init failed: %v", err)
	}

	log.Println("✅ Connected to Redis")
  // Initialize registry 
	if err := model.InitRegistry(cfg.ModelsDir); err != nil {
		log.Fatalf("❌ InitRegistry failed: %v", err)
	}
	fmt.Println("✅ All Models and Presets initialized")  
	// Load locales if available
	// This is optional, so we handle errors gracefully
	// If locales are not found, we just disable localization	
	if err := model.LoadLocales(cfg.Locale); err != nil {		
		log.Printf("⚠️ LoadLocales: %v (localization disabled)", err)
		model.HasLocales = false
	} else {
		model.HasLocales = true
	}
  // Initialize routes
  router.InitRoutes()
  // Start HTTP server
  log.Printf("🚀 Starting server on port %s", cfg.Port)
  log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}
