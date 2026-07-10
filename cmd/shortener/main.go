package main

import (
	"log"
	"net/http"

	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/handlers"
	"github.com/AlexeyKurlevsky/shortener/internal/logger"
	"github.com/AlexeyKurlevsky/shortener/internal/server"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"go.uber.org/zap"
)

func main() {
	cfg := config.NewConfig()

	if err := logger.Initialize(cfg.LogLevel); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	var st storage.Storage
	if cfg.FileStoragePath != "" {
		s, err := storage.NewJSONStorage(cfg.FileStoragePath)
		if err != nil {
			logger.Log.Fatal("Failed to init JSON storage", zap.Error(err))
		}
		st = s
	} else {
		st = storage.NewMemoryStorage()
	}

	h := handlers.NewHandler(st, cfg)
	r := server.NewRouter(h)

	logger.Log.Info("Config",
		zap.String("ServerAddr", cfg.ServerAddr),
		zap.String("BaseURL", cfg.BaseURL),
		zap.String("FileStoragePath", cfg.FileStoragePath),
	)
	if err := http.ListenAndServe(cfg.ServerAddr, r); err != nil {
		logger.Log.Fatal("Server failed: %v", zap.Error(err))
	}
}
