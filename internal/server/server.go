package server

import (
	"github.com/AlexeyKurlevsky/shortener/internal/handlers"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(h *handlers.Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger, middleware.Recoverer)
	r.Post("/", h.CreateShortURL)
	r.Get("/{id}", h.GetLink)
	return r
}
