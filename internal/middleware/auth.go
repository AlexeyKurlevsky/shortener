package middleware

import (
	"net/http"

	"github.com/AlexeyKurlevsky/shortener/internal/user"
)

func AuthMiddleware(us user.UserService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := us.EnsureUser(w, r)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
