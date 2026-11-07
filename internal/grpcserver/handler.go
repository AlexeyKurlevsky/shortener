package grpcserver

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "github.com/AlexeyKurlevsky/shortener/api"
	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/core"
	"github.com/AlexeyKurlevsky/shortener/internal/models"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"github.com/AlexeyKurlevsky/shortener/internal/user"
)

// ShortenerServer — тонкий фасад над core.*: те же операции, что
// и в HTTP-хендлерах, только транспорт другой.
type ShortenerServer struct {
	pb.UnimplementedShortenerServiceServer
	cfg     *config.Config
	storage storage.Storage
}

func NewShortenerServer(cfg *config.Config, stor storage.Storage) *ShortenerServer {
	return &ShortenerServer{cfg: cfg, storage: stor}
}

// ShortenURL — gRPC-аналог POST /api/shorten.
func (s *ShortenerServer) ShortenURL(
	ctx context.Context,
	req *pb.URLShortenRequest,
) (*pb.URLShortenResponse, error) {
	userID, _ := ctx.Value(user.UserIDContextKey).(string)

	link, err := core.ShortenURL(ctx, req.Url, userID, s.storage)
	if err != nil {
		var dupErr *models.DuplicateURLError
		if errors.As(err, &dupErr) {
			// gRPC не умеет возвращать тело вместе с ошибкой, поэтому
			// существующую короткую ссылку кладём в сообщение статуса.
			// Клиент может извлечь её из st.Message().
			return nil, status.Errorf(codes.AlreadyExists,
				"URL already shortened: %s", s.cfg.BaseURL+"/"+dupErr.ExistingID)
		}
		var appErr models.AppError
		if errors.As(err, &appErr) {
			return nil, status.Error(httpStatusToGRPC(appErr.Status), appErr.Error())
		}
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &pb.URLShortenResponse{
		Result: link.GetFullLink(s.cfg.BaseURL),
	}, nil
}

// ExpandURL — gRPC-аналог GET /<id> (только без редиректа:
// gRPC возвращает оригинальный URL, а не Location).
func (s *ShortenerServer) ExpandURL(
	ctx context.Context,
	req *pb.URLExpandRequest,
) (*pb.URLExpandResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}

	original, err := core.ExpandURL(ctx, req.Id, s.storage)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "URL not found")
		}
		if errors.Is(err, storage.ErrGone) {
			return nil, status.Error(codes.FailedPrecondition, "URL is gone")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &pb.URLExpandResponse{Result: original}, nil
}

// ListUserURLs — gRPC-аналог GET /api/user/urls.
func (s *ShortenerServer) ListUserURLs(
	ctx context.Context,
	_ *emptypb.Empty,
) (*pb.UserURLsResponse, error) {
	userID, _ := ctx.Value(user.UserIDContextKey).(string)

	pairs, err := core.ListUserURLs(ctx, userID, s.storage)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	urlData := make([]*pb.URLData, len(pairs))
	for i, p := range pairs {
		urlData[i] = &pb.URLData{
			ShortUrl:    s.cfg.BaseURL + "/" + p.ShortURL,
			OriginalUrl: p.OriginalURL,
		}
	}
	return &pb.UserURLsResponse{Url: urlData}, nil
}

// httpStatusToGRPC переводит HTTP-статус из models.AppError в gRPC-код.
func httpStatusToGRPC(httpStatus int) codes.Code {
	switch httpStatus {
	case http.StatusBadRequest:
		return codes.InvalidArgument
	case http.StatusNotFound:
		return codes.NotFound
	case http.StatusConflict:
		return codes.AlreadyExists
	case http.StatusGone:
		return codes.FailedPrecondition
	case http.StatusServiceUnavailable:
		return codes.Unavailable
	default:
		return codes.Internal
	}
}
