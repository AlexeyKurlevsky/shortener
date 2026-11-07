package grpcserver

import (
	"crypto/tls"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	pb "github.com/AlexeyKurlevsky/shortener/api"
	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
)

// NewGRPCServer собирает gRPC-сервер с интерсепторами и TLS (если tlsCfg != nil).
//
// tlsCfg передаётся тот же, что и для HTTP-сервера (ACME-менеджер),
// чтобы «параметры конфигурации gRPC-сервера совпадали с HTTP»:
//   - при EnableHTTPS = true  → TLS с SNI-сертификатами из ACME;
//   - при EnableHTTPS = false → обычный plaintext-порт.
func NewGRPCServer(
	cfg *config.Config,
	stor storage.Storage,
	auth TokenAuthenticator,
	log *zap.Logger,
	tlsCfg *tls.Config,
) *grpc.Server {
	var opts []grpc.ServerOption

	if tlsCfg != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}

	opts = append(opts,
		grpc.ChainUnaryInterceptor(
			RecoveryInterceptor(),
			LoggingInterceptor(log),
			AuthInterceptor(auth),
		),
	)

	srv := grpc.NewServer(opts...)
	pb.RegisterShortenerServiceServer(srv, NewShortenerServer(cfg, stor))

	// reflection полезен для grpcurl/отладки. В проде при желании уберите.
	reflection.Register(srv)

	return srv
}
