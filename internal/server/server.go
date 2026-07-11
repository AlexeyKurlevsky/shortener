package server

import (
	"github.com/AlexeyKurlevsky/shortener/internal/handlers"
	"github.com/AlexeyKurlevsky/shortener/internal/logger"
	mymiddleware "github.com/AlexeyKurlevsky/shortener/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(h *handlers.Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(logger.RequestLogger, middleware.Recoverer, mymiddleware.GzipMiddleware)
	r.Post("/", h.CreateShortURL)
	r.Post("/api/shorten", h.CreateShortURLJson)
	r.Get("/{id}", h.GetLink)
	return r
}
