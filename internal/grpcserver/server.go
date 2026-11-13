package grpcserver

import (
	"crypto/tls"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	pb "github.com/AlexeyKurlevsky/shortener/api"
	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"github.com/AlexeyKurlevsky/shortener/internal/tlsconfig"
)

// NewGRPCServer собирает gRPC-сервер с интерсепторами.
//
// Транспорт всегда TLS:
//   - при EnableHTTPS = true  — используется tlsCfg (ACME-менеджер);
//   - при EnableHTTPS = false — генерируется эфемерный самоподписанный
//     сертификат для localhost. Параметры конфигурации сервера
//     (интерсепторы, порт, reflection) при этом не меняются.
func NewGRPCServer(
	cfg *config.Config,
	stor storage.Storage,
	auth TokenAuthenticator,
	log *zap.Logger,
	tlsCfg *tls.Config,
) (*grpc.Server, error) {
	if tlsCfg == nil {
		var err error
		tlsCfg, err = tlsconfig.NewSelfSignedTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("self-signed TLS config: %w", err)
		}
		log.Info("gRPC: using self-signed TLS (EnableHTTPS=false)")
	}

	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(tlsCfg)),
		grpc.ChainUnaryInterceptor(
			RecoveryInterceptor(),
			LoggingInterceptor(log),
			AuthInterceptor(auth),
		),
	}

	srv := grpc.NewServer(opts...)
	pb.RegisterShortenerServiceServer(srv, NewShortenerServer(cfg, stor))
	reflection.Register(srv)

	return srv, nil
}
