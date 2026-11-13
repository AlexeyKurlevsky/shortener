package grpcserver

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	pb "github.com/AlexeyKurlevsky/shortener/api"
	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/core"
	"github.com/AlexeyKurlevsky/shortener/internal/models"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"github.com/AlexeyKurlevsky/shortener/internal/user"
)

type ShortenerServer struct {
	pb.UnimplementedShortenerServiceServer
	cfg     *config.Config
	storage storage.Storage
}

func NewShortenerServer(cfg *config.Config, stor storage.Storage) *ShortenerServer {
	return &ShortenerServer{cfg: cfg, storage: stor}
}

func (s *ShortenerServer) ShortenURL(
	ctx context.Context,
	req *pb.URLShortenRequest,
) (*pb.URLShortenResponse, error) {
	userID, _ := ctx.Value(user.UserIDContextKey).(string)

	link, err := core.ShortenURL(ctx, req.GetUrl(), userID, s.storage)
	if err != nil {
		var dupErr *models.DuplicateURLError
		if errors.As(err, &dupErr) {
			return nil, status.Errorf(codes.AlreadyExists,
				"URL already shortened: %s", s.cfg.BaseURL+"/"+dupErr.ExistingID)
		}
		var appErr models.AppError
		if errors.As(err, &appErr) {
			return nil, status.Error(httpStatusToGRPC(appErr.Status), appErr.Error())
		}
		return nil, status.Error(codes.Internal, "internal error")
	}

	return pb.URLShortenResponse_builder{
		Result: proto.String(link.GetFullLink(s.cfg.BaseURL)),
	}.Build(), nil
}

func (s *ShortenerServer) ExpandURL(
	ctx context.Context,
	req *pb.URLExpandRequest,
) (*pb.URLExpandResponse, error) {
	id := req.GetId()
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}

	original, err := core.ExpandURL(ctx, id, s.storage)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "URL not found")
		}
		if errors.Is(err, storage.ErrGone) {
			return nil, status.Error(codes.FailedPrecondition, "URL is gone")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}
	return pb.URLExpandResponse_builder{Result: proto.String(original)}.Build(), nil
}

func (s *ShortenerServer) ListUserURLs(
	ctx context.Context,
	_ *pb.ListURLsRequest,
) (*pb.UserURLsResponse, error) {
	userID, _ := ctx.Value(user.UserIDContextKey).(string)

	pairs, err := core.ListUserURLs(ctx, userID, s.storage)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	urlData := make([]*pb.URLData, len(pairs))
	for i, p := range pairs {
		urlData[i] = pb.URLData_builder{
			ShortUrl:    proto.String(s.cfg.BaseURL + "/" + p.ShortURL),
			OriginalUrl: proto.String(p.OriginalURL),
		}.Build()
	}
	return pb.UserURLsResponse_builder{Url: urlData}.Build(), nil
}

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
