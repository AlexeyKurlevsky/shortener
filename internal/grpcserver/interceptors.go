package grpcserver

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/AlexeyKurlevsky/shortener/internal/user"
)

// TokenAuthenticator валидирует токен из metadata и возвращает userID.
//
// В main.go это, скорее всего, будет метод вашего user.Service:
//
//	grpcserver.AuthInterceptor(userSvc.ValidateToken)
//
// Если такого метода ещё нет — оберните существующую логику
// из middleware.AuthMiddleware в функцию с этой сигнатурой,
// чтобы HTTP и gRPC использовали одно и то же правило проверки.
type TokenAuthenticator func(ctx context.Context, token string) (userID string, err error)

// RecoveryInterceptor превращает panic в codes.Internal вместо падения
// всего процесса.
func RecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = status.Errorf(codes.Internal, "panic: %v", r)
			}
		}()
		return handler(ctx, req)
	}
}

// LoggingInterceptor логирует метод, длительность и ошибку вызова.
func LoggingInterceptor(log *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		log.Info("gRPC request",
			zap.String("method", info.FullMethod),
			zap.Duration("duration", time.Since(start)),
			zap.Error(err),
		)
		return resp, err
	}
}

// AuthInterceptor берёт токен из metadata "authorization", валидирует его
// и кладёт userID в контекст под ключом user.UserIDContextKey — тем же,
// что использует HTTP-middleware.
//
// Поддерживаются оба формата: "Bearer <token>" и просто "<token>".
func AuthInterceptor(auth TokenAuthenticator) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		vals := md.Get("authorization")
		if len(vals) == 0 {
			return nil, status.Error(codes.Unauthenticated, "authorization token is not provided")
		}

		token := strings.TrimSpace(strings.TrimPrefix(vals[0], "Bearer "))
		if token == "" {
			return nil, status.Error(codes.Unauthenticated, "empty token")
		}

		userID, err := auth(ctx, token)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		ctx = context.WithValue(ctx, user.UserIDContextKey, userID)
		return handler(ctx, req)
	}
}
