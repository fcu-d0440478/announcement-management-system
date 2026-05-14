package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"announcement-management-system/backend/internal/config"
	"announcement-management-system/backend/internal/db"
	"announcement-management-system/backend/internal/handlers"
)

func main() {
	cfg := config.Load()
	store, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer store.DB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := store.Migrate(ctx); err != nil {
		cancel()
		log.Fatalf("migrate database: %v", err)
	}
	cancel()

	go runScheduler(store)

	server := &handlers.Server{
		Store:      store,
		JWTSecret:  cfg.JWTSecret,
		CORSOrigin: cfg.CORSOrigin,
	}
	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("api listening on :%s", cfg.Port)
	log.Fatal(httpServer.ListenAndServe())
}

func runScheduler(store *db.Store) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		affected, err := store.PromoteScheduled(ctx)
		cancel()
		if err != nil {
			log.Printf("scheduler error: %v", err)
		} else if affected > 0 {
			log.Printf("scheduler published %d announcements", affected)
		}
		<-ticker.C
	}
}

