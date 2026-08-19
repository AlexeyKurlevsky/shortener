package server

import (
	"github.com/AlexeyKurlevsky/shortener/internal/handlers"
	mymiddleware "github.com/AlexeyKurlevsky/shortener/internal/middleware"
	"github.com/AlexeyKurlevsky/shortener/internal/user"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(h *handlers.Handler, userSvc user.UserService) *chi.Mux {
	r := chi.NewRouter()
	r.Use(mymiddleware.RequestLogger, middleware.Recoverer, mymiddleware.GzipMiddleware)
	r.Use(mymiddleware.AuthMiddleware(userSvc))
	r.Post("/", h.CreateShortURL)
	r.Post("/api/shorten", h.CreateShortURLJson)
	r.Get("/{id}", h.GetLink)
	r.Get("/ping", h.PingHandler)
	r.Post("/api/shorten/batch", h.BatchCreateShortURL)
	r.Get("/api/user/urls", h.GetUserURLs)
	return r
}
