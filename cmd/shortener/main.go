package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
		logger.Log.Fatal("Incorrect config", zap.Error(err))
	}

	if err := logger.Initialize(cfg.LogLevel); err != nil {
		logger.Log.Fatal("Failed to initialize logger", zap.Error(err))
	}

	// ---- Аудит ----
	auditPublisher := audit.NewPublisher(2048)

	var fileObs *audit.FileObserver
	if cfg.AuditFile != "" {
		fo, err := audit.NewFileObserver(cfg.AuditFile)
		if err != nil {
			logger.Log.Fatal("Failed to init audit file", zap.Error(err))
		}
		fileObs = fo
		auditPublisher.Subscribe(fileObs)
		logger.Log.Info("Audit: file observer enabled", zap.String("path", cfg.AuditFile))
	}

	if cfg.AuditURL != "" {
		auditPublisher.Subscribe(audit.NewHTTPObserver(cfg.AuditURL))
		logger.Log.Info("Audit: http observer enabled", zap.String("url", cfg.AuditURL))
	}

	auditPublisher.Start()

	// ---- Storage ----
	var st storage.Storage
	var pinger handlers.Pinger

	if cfg.DatabaseDSN != "" {
		pgStore, err := storage.NewPostgresStorage(context.Background(), cfg.DatabaseDSN)
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

	// ---- Handlers + Router ----
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

	// ---- Сервер ----
	srv := &http.Server{Addr: cfg.ServerAddr, Handler: r}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Log.Fatal("Server failed", zap.Error(err))
		}
	case sig := <-stop:
		logger.Log.Info("Signal received, shutting down",
			zap.String("signal", sig.String()))
	}

	// Всё завершение выполняется в отдельной горутине, а main
	// ограничивает его по времени. Даже при полном ступоре внутри
	// процесс выйдет через 5 секунд.
	done := make(chan struct{})
	go func() {
		defer close(done)

		shutCtx, shutCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutCancel()

		if err := srv.Shutdown(shutCtx); err != nil {
			logger.Log.Warn("HTTP shutdown failed, forcing close", zap.Error(err))
			_ = srv.Close()
		}
		logger.Log.Info("HTTP server stopped")

		auditPublisher.Close() // дожидается обработки остатка очереди
		logger.Log.Info("Audit publisher closed")

		if fileObs != nil {
			if err := fileObs.Close(); err != nil {
				logger.Log.Error("Audit file close failed", zap.Error(err))
			}
		}
		logger.Log.Info("Bye")
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		logger.Log.Warn("Shutdown timeout exceeded, exiting anyway")
	}
}
