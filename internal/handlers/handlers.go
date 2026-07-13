package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"

	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/logger"
	"github.com/AlexeyKurlevsky/shortener/internal/models"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type Handler struct {
	storage storage.Storage
	cfg     *config.Config
}

func NewHandler(storage storage.Storage, cfg *config.Config) *Handler {
	return &Handler{storage: storage, cfg: cfg}
}

func generateID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 8
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func IsValidURL(str string) bool {
	u, err := url.ParseRequestURI(str)
	if err != nil {
		return false
	}
	if u.Scheme == "" || u.Host == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return true
}

func normalizeURL(raw string) string {
	// 1. Удаляем пробелы вокруг (аналог strip)
	trimmed := strings.TrimSpace(raw)

	// 2. Убираем завершающий слэш, если он есть (кроме корневого "/")
	return strings.TrimSuffix(trimmed, "/")
}

func handleShorten(url string, storage storage.Storage) (models.ShortenLink, error) {
	var result models.ShortenLink

	// Проверяем валидность url
	if !IsValidURL(url) {
		return result, newInvalidURLError()
	}
	url = normalizeURL(url)

	// Ищем url в уже созданных ссылках
	if shortURL, ok := storage.FindIDByURL(url); ok {
		result.ShortUrl = shortURL
		result.OriginalUrl = url
		result.IsNew = false
		return result, nil
	}

	// Если не нашли, то генерируем новую
	var shortURL string
	for {
		shortURL = generateID()
		if !storage.Exists(shortURL) {
			break
		}
	}
	if err := storage.Save(shortURL, url); err != nil {
		return result, newStorageSaveError()
	}

	result.OriginalUrl = url
	result.ShortUrl = shortURL
	result.IsNew = true

	return result, nil
}

func (h *Handler) CreateShortURL(w http.ResponseWriter, r *http.Request) {
	var shortURL models.ShortenLink
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	link := string(body)

	shortURL, err = handleShorten(link, h.storage)
	if err != nil {
		var appErr models.AppError
		if errors.As(err, &appErr) {
			http.Error(w, appErr.Error(), appErr.Status)
		} else {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(shortURL.GetStatusCode())
	fullLink := shortURL.GetFullLink(h.cfg.BaseURL)
	_, _ = w.Write([]byte(fullLink))
}

func (h *Handler) GetLink(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	original, err := h.storage.Get(id)
	if err != nil {
		if err == storage.ErrNotFound {
			http.Error(w, "URL not found", http.StatusNotFound)
		} else {
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Location", original)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func (h *Handler) CreateShortURLJson(w http.ResponseWriter, r *http.Request) {
	var req models.CreateUrlRequest
	var shortURL models.ShortenLink

	dec := json.NewDecoder(r.Body)

	if err := dec.Decode(&req); err != nil {
		logger.Log.Debug("cannot decode request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shortURL, err := handleShorten(req.Url, h.storage)
	if err != nil {
		var appErr models.AppError
		if errors.As(err, &appErr) {
			http.Error(w, appErr.Error(), appErr.Status)
		} else {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	resp := models.ShortUrlResponse{
		Result: shortURL.GetFullLink(h.cfg.BaseURL),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(shortURL.GetStatusCode())

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Log.Error("failed to encode response", zap.Error(err))
	}
}
