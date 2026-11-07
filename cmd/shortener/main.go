package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/AlexeyKurlevsky/shortener/internal/audit"
	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/grpcserver"
	"github.com/AlexeyKurlevsky/shortener/internal/handlers"
	"github.com/AlexeyKurlevsky/shortener/internal/logger"
	"github.com/AlexeyKurlevsky/shortener/internal/server"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"github.com/AlexeyKurlevsky/shortener/internal/tlsconfig"
	"github.com/AlexeyKurlevsky/shortener/internal/user"
)

var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

// printBuildInfo печатает информацию о сборке. Если переменные не заданы
// линкером, подставляет "N/A".
func printBuildInfo() {
	fmt.Printf("Build version: %s\n", orNA(buildVersion))
	fmt.Printf("Build date: %s\n", orNA(buildDate))
	fmt.Printf("Build commit: %s\n", orNA(buildCommit))
}

func orNA(v string) string {
	if v == "" {
		return "N/A"
	}
	return v
}

func main() {
	printBuildInfo()

	// ---- Конфигурация и логгер ----
	cfg, err := config.NewConfig()
	if err != nil {
		logger.Log.Fatal("Incorrect config", zap.Error(err))
	}

	if err = logger.Initialize(cfg.LogLevel); err != nil {
		logger.Log.Fatal("Failed to initialize logger", zap.Error(err))
	}

	// ---- Аудит ----
	// Publisher — центральный узел: собирает события и рассылает их
	// подписчикам (файл, HTTP-эндпоинт).
	auditPublisher := audit.NewPublisher(2048)

	var fileObs *audit.FileObserver
	if cfg.AuditFile != "" {
		fileObs, err = audit.NewFileObserver(cfg.AuditFile)
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

	auditPublisher.Start()

	// ---- Хранилище ----
	// Выбор реализации по приоритету: PostgreSQL → JSON-файл → in-memory.
	// pinger передаётся в HTTP-хендлер /ping; у JSON и in-memory он nil.
	var st storage.Storage
	var pinger handlers.Pinger

	if cfg.DatabaseDSN != "" {
		var pgStore *storage.PostgresStorage
		pgStore, err = storage.NewPostgresStorage(context.Background(), cfg.DatabaseDSN)
		if err != nil {
			logger.Log.Fatal("Failed to init PostgreSQL storage", zap.Error(err))
		}
		defer pgStore.Close()
		logger.Log.Info("Use postgres for storage")
		st = pgStore
		pinger = pgStore
	} else if cfg.FileStoragePath != "" {
		var jsonStore storage.Storage
		jsonStore, err = storage.NewJSONStorage(cfg.FileStoragePath)
		if err != nil {
			logger.Log.Fatal("Failed to init JSON storage", zap.Error(err))
		}
		st = jsonStore
		logger.Log.Info("Use json for storage")
	} else {
		logger.Log.Info("Use inmemory for storage")
		st = storage.NewMemoryStorage()
	}

	// ---- HTTP: хендлеры, сервис пользователей, роутер ----
	h := handlers.NewHandler(st, cfg, pinger, auditPublisher)
	userSvc := user.NewUserService(cfg)
	r := server.NewRouter(h, userSvc)

	logger.Log.Info("Config",
		zap.String("ServerAddr", cfg.ServerAddr),
		zap.String("GRPCAddr", cfg.GRPCAddr),
		zap.String("BaseURL", cfg.BaseURL),
		zap.String("FileStoragePath", cfg.FileStoragePath),
		zap.String("AuditFile", cfg.AuditFile),
		zap.String("AuditURL", cfg.AuditURL),
		zap.Bool("EnableHTTPS", cfg.EnableHTTPS),
		zap.Strings("Domains", cfg.Domains),
	)

	// ---- TLS-конфиг (общий для HTTP и gRPC) ----
	// При EnableHTTPS = true создаём один ACME-менеджер и один *tls.Config
	// с GetCertificate, чтобы и HTTP, и gRPC использовали те же сертификаты.
	var tlsCfg *tls.Config
	srv := &http.Server{Addr: cfg.ServerAddr, Handler: r}

	if cfg.EnableHTTPS {
		manager := tlsconfig.NewManager(cfg.Domains, cfg.ACMEEmail, cfg.ACMECacheDir)
		tlsCfg = tlsconfig.NewTLSConfig(manager)
		srv.Addr = ":443"
		srv.TLSConfig = tlsCfg
		logger.Log.Info("ACME manager initialized",
			zap.Strings("domains", cfg.Domains),
			zap.String("cache", cfg.ACMECacheDir),
		)
	}

	// ---- gRPC-сервер ----
	// Валидация токена переиспользует тот же HMAC-секрет, что и HTTP-cookie.
	grpcSrv := grpcserver.NewGRPCServer(
		cfg,
		st,
		userSvc.ValidateAuthorization,
		logger.Log,
		tlsCfg,
	)

	grpcLis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		logger.Log.Fatal("Failed to listen on gRPC addr",
			zap.String("addr", cfg.GRPCAddr), zap.Error(err))
	}

	// ---- Сигнальный контекст ----
	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	// ---- Запуск обоих серверов ----
	// Буфер на 2: и HTTP-, и gRPC-горутина отправляют сюда свою «финальную»
	// ошибку (ErrServerClosed / ErrServerStopped). Без запаса вторая
	// горутина заблокируется навсегда после select-а.
	errCh := make(chan error, 2)

	go func() {
		if cfg.EnableHTTPS {
			logger.Log.Info("Starting HTTPS server (autocert)",
				zap.String("addr", srv.Addr),
				zap.Strings("domains", cfg.Domains),
			)
			// Пустые пути — сертификаты берутся из srv.TLSConfig.GetCertificate
			errCh <- srv.ListenAndServeTLS("", "")
			return
		}
		logger.Log.Info("Starting HTTP server", zap.String("addr", srv.Addr))
		errCh <- srv.ListenAndServe()
	}()

	go func() {
		logger.Log.Info("Starting gRPC server",
			zap.String("addr", cfg.GRPCAddr),
			zap.Bool("tls", tlsCfg != nil),
		)
		errCh <- grpcSrv.Serve(grpcLis)
	}()

	// Ждём либо ошибку сервера, либо сигнал завершения.
	select {
	case err = <-errCh:
		// http.ErrServerClosed и grpc.ErrServerStopped — штатное завершение
		// после Shutdown/GracefulStop/Stop, а не сбой.
		if err != nil &&
			!errors.Is(err, http.ErrServerClosed) &&
			!errors.Is(err, grpc.ErrServerStopped) {
			logger.Log.Error("Server failed", zap.Error(err))
		}
	case <-ctx.Done():
		logger.Log.Info("Signal received, shutting down", zap.Error(ctx.Err()))
	}
	stop()

	// ---- Graceful shutdown ----
	// Всё завершение выполняется в отдельной горутине, а main ограничивает
	// его по времени. Даже при полном ступоре внутри процесс выйдет.
	done := make(chan struct{})
	go func() {
		defer close(done)

		shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutCancel()

		// HTTP: Shutdown дожидается активных запросов.
		if err := srv.Shutdown(shutCtx); err != nil {
			logger.Log.Warn("HTTP shutdown failed, forcing close", zap.Error(err))
			_ = srv.Close()
		}
		logger.Log.Info("HTTP server stopped")

		// gRPC: GracefulStop не принимает контекст — ограничиваем вручную.
		grpcStopped := make(chan struct{})
		go func() {
			grpcSrv.GracefulStop()
			close(grpcStopped)
		}()
		select {
		case <-grpcStopped:
			logger.Log.Info("gRPC server stopped gracefully")
		case <-shutCtx.Done():
			logger.Log.Warn("gRPC graceful stop timeout, forcing stop")
			grpcSrv.Stop()
			<-grpcStopped
		}

		// Останавливаем delete-воркер и аудит уже после серверов,
		// чтобы не потерять последние задачи/события.
		h.Close()
		logger.Log.Info("Delete worker stopped")

		auditPublisher.Close()
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
	case <-time.After(45 * time.Second):
		logger.Log.Warn("Shutdown timeout exceeded, exiting anyway")
	}
}
