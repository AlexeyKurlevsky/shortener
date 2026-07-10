package main

import (
	"log"
	"net/http"

	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/handlers"
	"github.com/AlexeyKurlevsky/shortener/internal/server"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
)

func main() {
	cfg := config.NewConfig()

	var st storage.Storage
	if cfg.FileStoragePath != "" {
		s, err := storage.NewJSONStorage(cfg.FileStoragePath)
		if err != nil {
			log.Fatalf("Failed to init JSON storage: %v", err)
		}
		st = s
	} else {
		st = storage.NewMemoryStorage()
	}

	h := handlers.NewHandler(st, cfg)
	r := server.NewRouter(h)

	log.Printf("Server starting on %s", cfg.ServerAddr)
	if err := http.ListenAndServe(cfg.ServerAddr, r); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
