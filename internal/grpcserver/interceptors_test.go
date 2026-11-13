package grpcserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/AlexeyKurlevsky/shortener/internal/user"
)

// info — минимальный grpc.UnaryServerInfo для тестов.
func info(method string) *grpc.UnaryServerInfo {
	return &grpc.UnaryServerInfo{FullMethod: method}
}

// okHandler возвращает resp и nil-ошибку.
func okHandler(resp interface{}) grpc.UnaryHandler {
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		return resp, nil
	}
}

// errHandler возвращает заданную ошибку.
func errHandler(err error) grpc.UnaryHandler {
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, err
	}
}

// ---- RecoveryInterceptor ----

func TestRecoveryInterceptor_PanicRecovered(t *testing.T) {
	panickingHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		panic("boom")
	}

	resp, err := RecoveryInterceptor()(context.Background(), nil, info("/x/Y"), panickingHandler)

	if resp != nil {
		t.Fatalf("expected nil resp, got %v", resp)
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.Internal {
		t.Fatalf("expected Internal, got %v", st.Code())
	}
	if !strings.Contains(st.Message(), "boom") {
		t.Fatalf("expected message to contain panic text, got %q", st.Message())
	}
}

func TestRecoveryInterceptor_PassThrough(t *testing.T) {
	resp, err := RecoveryInterceptor()(context.Background(), "req", info("/x/Y"), okHandler("resp"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp != "resp" {
		t.Fatalf("got %v, want resp", resp)
	}
}

// ---- LoggingInterceptor ----

func TestLoggingInterceptor_LogsAndPasses(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log := zap.New(core)

	resp, err := LoggingInterceptor(log)(
		context.Background(), "req", info("/svc/Method"), okHandler("resp"),
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp != "resp" {
		t.Fatalf("got %v, want resp", resp)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if got := entries[0].ContextMap()["method"]; got != "/svc/Method" {
		t.Fatalf("expected method field %q, got %v", "/svc/Method", got)
	}
}

func TestLoggingInterceptor_LogsError(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log := zap.New(core)

	wantErr := errors.New("boom")
	_, err := LoggingInterceptor(log)(
		context.Background(), nil, info("/svc/Method"), errHandler(wantErr),
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want %v", err, wantErr)
	}
	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", logs.Len())
	}
}

// ---- AuthInterceptor ----

func TestAuthInterceptor_MissingMetadata(t *testing.T) {
	// context.Background() без метаданных.
	auth := func(ctx context.Context, token string) (string, error) { return "user", nil }

	_, err := AuthInterceptor(auth)(context.Background(), nil, info("/svc/Method"), okHandler(nil))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestAuthInterceptor_MissingAuthorization(t *testing.T) {
	auth := func(ctx context.Context, token string) (string, error) { return "user", nil }
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{}))

	_, err := AuthInterceptor(auth)(ctx, nil, info("/svc/Method"), okHandler(nil))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestAuthInterceptor_EmptyToken(t *testing.T) {
	auth := func(ctx context.Context, token string) (string, error) { return "user", nil }
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer "))

	_, err := AuthInterceptor(auth)(ctx, nil, info("/svc/Method"), okHandler(nil))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestAuthInterceptor_ValidBearer(t *testing.T) {
	var gotToken string
	auth := func(ctx context.Context, token string) (string, error) {
		gotToken = token
		return "user-123", nil
	}
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer abc.def"))

	var gotUserID string
	h := func(ctx context.Context, req interface{}) (interface{}, error) {
		gotUserID, _ = ctx.Value(user.UserIDContextKey).(string)
		return "ok", nil
	}

	resp, err := AuthInterceptor(auth)(ctx, nil, info("/svc/Method"), h)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("got %v, want ok", resp)
	}
	if gotToken != "abc.def" {
		t.Fatalf("expected Bearer prefix stripped, got %q", gotToken)
	}
	if gotUserID != "user-123" {
		t.Fatalf("expected userID in ctx, got %q", gotUserID)
	}
}

func TestAuthInterceptor_ValidWithoutBearer(t *testing.T) {
	auth := func(ctx context.Context, token string) (string, error) { return "user-123", nil }
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "abc.def"))

	var gotUserID string
	h := func(ctx context.Context, req interface{}) (interface{}, error) {
		gotUserID, _ = ctx.Value(user.UserIDContextKey).(string)
		return nil, nil
	}

	if _, err := AuthInterceptor(auth)(ctx, nil, info("/svc/Method"), h); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotUserID != "user-123" {
		t.Fatalf("expected userID in ctx, got %q", gotUserID)
	}
}

func TestAuthInterceptor_InvalidToken(t *testing.T) {
	auth := func(ctx context.Context, token string) (string, error) {
		return "", errors.New("bad token")
	}
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer bad"))

	_, err := AuthInterceptor(auth)(ctx, nil, info("/svc/Method"), okHandler(nil))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
	}
}
