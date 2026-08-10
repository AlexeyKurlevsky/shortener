package user

import (
	"context"
	"net/http"

	"github.com/AlexeyKurlevsky/shortener/internal/auth"
	"github.com/AlexeyKurlevsky/shortener/internal/config"
)

type UserService interface {
	GetUserIDFromRequest(r *http.Request) (string, error)
	SetUserCookie(w http.ResponseWriter, userID string)
	EnsureUser(w http.ResponseWriter, r *http.Request) context.Context
}

type userService struct {
	secret []byte
}

func NewUserService(cfg *config.Config) UserService {
	return &userService{secret: cfg.SecretKeyByte}
}

func (s *userService) GetUserIDFromRequest(r *http.Request) (string, error) {
	return auth.GetUserIDFromCookie(r, s.secret)
}

func (s *userService) SetUserCookie(w http.ResponseWriter, userID string) {
	cookie := auth.CreateSignedCookie(userID, s.secret)
	http.SetCookie(w, &cookie)
}

func (s *userService) EnsureUser(w http.ResponseWriter, r *http.Request) context.Context {
	userID, err := s.GetUserIDFromRequest(r)
	if err != nil {
		userID = auth.GenerateUserID()
		s.SetUserCookie(w, userID)
	}
	return context.WithValue(r.Context(), userIDContextKey, userID)
}

type contextKey string

const userIDContextKey contextKey = "userID"

func GetUserIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(userIDContextKey).(string); ok {
		return id
	}
	return ""
}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey, userID)
}
