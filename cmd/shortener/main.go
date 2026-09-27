package main

import (
	"context"
	"log"
	"net/http"

	"github.com/AlexeyKurlevsky/shortener/internal/audit"
	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/handlers"
	"github.com/AlexeyKurlevsky/shortener/internal/logger"
	"github.com/AlexeyKurlevsky/shortener/internal/server"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"github.com/AlexeyKurlevsky/shortener/internal/user"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatalf("Incorrect config: %v", err)
	}

	if err := logger.Initialize(cfg.LogLevel); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	// ---- Аудит ----
	// Publisher запускаем с context.Background():
	auditPublisher := audit.NewPublisher(2048)

	if cfg.AuditFile != "" {
		fileObs, err := audit.NewFileObserver(cfg.AuditFile)
		if err != nil {
			logger.Log.Fatal("Failed to init audit file", zap.Error(err))
		}
		auditPublisher.Subscribe(fileObs)
		logger.Log.Info("Audit: file observer enabled", zap.String("path", cfg.AuditFile))
	}

	if cfg.AuditURL != "" {
		auditPublisher.Subscribe(audit.NewHTTPObserver(cfg.AuditURL))
		logger.Log.Info("Audit: http observer enabled", zap.String("url", cfg.AuditURL))
	}

	auditPublisher.Start(context.Background())

	// ---- Storage ----
	var st storage.Storage
	var pinger handlers.Pinger

	if cfg.DatabaseDSN != "" {
		pgStore, err := storage.NewPostgresStorage(cfg.DatabaseDSN)
		if err != nil {
			logger.Log.Fatal("Failed to init PostgreSQL storage", zap.Error(err))
		}
		defer pgStore.Close()
		logger.Log.Info("Use postgres for storage")
		st = pgStore
		pinger = pgStore
	} else if cfg.FileStoragePath != "" {
		s, err := storage.NewJSONStorage(cfg.FileStoragePath)
		if err != nil {
			logger.Log.Fatal("Failed to init JSON storage", zap.Error(err))
		}
		st = s
		logger.Log.Info("Use json for storage")
	} else {
		logger.Log.Info("Use inmemory for storage")
		st = storage.NewMemoryStorage()
	}

	h := handlers.NewHandler(st, cfg, pinger, auditPublisher)
	userSvc := user.NewUserService(cfg)
	r := server.NewRouter(h, userSvc)

	logger.Log.Info("Config",
		zap.String("ServerAddr", cfg.ServerAddr),
		zap.String("BaseURL", cfg.BaseURL),
		zap.String("FileStoragePath", cfg.FileStoragePath),
		zap.String("AuditFile", cfg.AuditFile),
		zap.String("AuditURL", cfg.AuditURL),
	)

	if err := http.ListenAndServe(cfg.ServerAddr, r); err != nil {
		logger.Log.Fatal("Server failed: %v", zap.Error(err))
	}
}
