package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/logger"
	"github.com/AlexeyKurlevsky/shortener/internal/models"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	storage storage.Storage
	cfg     *config.Config
	db      Pinger
}

func NewHandler(storage storage.Storage, cfg *config.Config, db Pinger) *Handler {
	return &Handler{storage: storage, cfg: cfg, db: db}
}

func (h *Handler) CreateShortURL(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	link := string(body)

	shortURL, err := handleShorten(r.Context(), link, h.storage)
	if err != nil {
		var dupErr *DuplicateURLError
		if errors.As(err, &dupErr) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusConflict)
			fullLink := h.cfg.BaseURL + "/" + dupErr.ExistingID
			_, _ = w.Write([]byte(fullLink))
			return
		}
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
	original, err := h.storage.Get(r.Context(), id)
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
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		logger.Log.Debug("cannot decode request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shortURL, err := handleShorten(r.Context(), req.Url, h.storage)
	if err != nil {
		var dupErr *DuplicateURLError
		if errors.As(err, &dupErr) {
			resp := models.ShortUrlResponse{
				Result: h.cfg.BaseURL + "/" + dupErr.ExistingID,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				logger.Log.Error("failed to encode response", zap.Error(err))
			}
			return
		}
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

func (h *Handler) PingHandler(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
		return
	}
	ctx := r.Context()
	if err := h.db.Ping(ctx); err != nil {
		http.Error(w, "Database connection failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *Handler) BatchCreateShortURL(w http.ResponseWriter, r *http.Request) {
	// 1. Читаем тело
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// 2. Декодируем JSON
	var reqItems []models.BatchRequestItem
	if err := json.Unmarshal(body, &reqItems); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if len(reqItems) == 0 {
		http.Error(w, "Empty batch", http.StatusBadRequest)
		return
	}

	// 3. Подготовка данных (валидация, поиск существующих, генерация новых ID)
	urlMap, newItems, err := prepareBatchItems(r.Context(), reqItems, h.storage)
	if err != nil {
		var appErr models.AppError
		if errors.As(err, &appErr) {
			http.Error(w, appErr.Error(), appErr.Status)
		} else {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	// 4. Сохраняем новые пары атомарно
	if len(newItems) > 0 {
		if err := h.storage.BatchSave(r.Context(), newItems); err != nil {
			logger.Log.Error("failed to save batch", zap.Error(err))
			http.Error(w, "Failed to save batch", http.StatusInternalServerError)
			return
		}
	}

	// 5. Формируем и отправляем ответ
	respItems := buildBatchResponse(reqItems, urlMap, h.cfg.BaseURL)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(respItems); err != nil {
		logger.Log.Error("failed to encode response", zap.Error(err))
	}
}
