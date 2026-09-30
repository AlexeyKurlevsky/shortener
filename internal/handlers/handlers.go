// Package handlers содержит HTTP-обработчики сервиса сокращения ссылок.
//
// Обработчики извлекают идентификатор пользователя из контекста запроса
// (user.UserIDContextKey), публикуют события аудита и формируют ответы
// в формате text/plain или application/json в зависимости от эндпоинта.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/AlexeyKurlevsky/shortener/internal/audit"
	"github.com/AlexeyKurlevsky/shortener/internal/logger"
	"github.com/AlexeyKurlevsky/shortener/internal/models"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"github.com/AlexeyKurlevsky/shortener/internal/user"
)

// Pinger описывает зависимость, доступность которой можно проверить.
// Используется PingHandler для проверки соединения с БД.
// Реализация Ping должна вернуть nil, если зависимость доступна.
type Pinger interface {
	Ping(ctx context.Context) error
}

// CreateShortURL создаёт короткую ссылку из URL, переданного в теле запроса
// как text/plain.
//
// Ожидает, что идентификатор пользователя уже находится в контексте запроса
// по ключу user.UserIDContextKey.
//
// Успешный ответ: text/plain с полной короткой ссылкой и статусом, который
// возвращает shortLink.GetStatusCode() (обычно 201 Created).
// При дубликате возвращает 409 Conflict и text/plain с уже существующей
// короткой ссылкой.
// Ошибка чтения тела — 500 Internal Server Error.
// Ошибки приложения возвращаются согласно models.AppError.
//
// Публикует событие аудита audit.ActionShorten.
func (h *Handler) CreateShortURL(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(user.UserIDContextKey).(string)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	link := string(body)

	shortLink, err := handleShorten(r.Context(), link, h.storage, userID)
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

	h.audit.Publish(r.Context(), audit.Event{
		TS:     time.Now().Unix(),
		Action: audit.ActionShorten,
		UserID: userID,
		URL:    link,
	})

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(shortLink.GetStatusCode())
	fullLink := shortLink.GetFullLink(h.cfg.BaseURL)
	_, _ = w.Write([]byte(fullLink))
}

// GetLink выполняет редирект по короткому идентификатору.
//
// Идентификатор берётся из chi URL-параметра "id".
// Если id пустой или содержит "/", возвращает 400 Bad Request.
// Если ссылка не найдена — 404 Not Found.
// Если ссылка удалена — 410 Gone.
// При прочих ошибках хранилища — 500 Internal Server Error.
//
// При успехе устанавливает заголовок Location и возвращает
// 307 Temporary Redirect.
//
// Публикует событие аудита audit.ActionFollow.
// userID берётся из контекста, если присутствует.
func (h *Handler) GetLink(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	original, err := h.storage.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "URL not found", http.StatusNotFound)
		} else if errors.Is(err, storage.ErrGone) {
			http.Error(w, "URL is gone", http.StatusGone)
		} else {
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}

	var userID string
	if v := r.Context().Value(user.UserIDContextKey); v != nil {
		userID, _ = v.(string)
	}
	h.audit.Publish(r.Context(), audit.Event{
		TS:     time.Now().Unix(),
		Action: audit.ActionFollow,
		UserID: userID,
		URL:    original,
	})

	w.Header().Set("Location", original)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

// CreateShortURLJson создаёт короткую ссылку из JSON-запроса
// models.CreateURLRequest.
//
// Ожидает userID в контексте по ключу user.UserIDContextKey.
// При некорректном JSON возвращает 400 Bad Request.
// При дубликате возвращает 409 Conflict и JSON models.ShortURLResponse
// с уже существующей короткой ссылкой.
// При успехе возвращает JSON models.ShortURLResponse и статус, который
// возвращает shortLink.GetStatusCode() (обычно 201 Created).
// Ошибки приложения обрабатываются через models.AppError.
//
// Публикует событие аудита audit.ActionShorten.
func (h *Handler) CreateShortURLJson(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(user.UserIDContextKey).(string)
	var req models.CreateURLRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		logger.Log.Debug("cannot decode request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shortLink, err := handleShorten(r.Context(), req.URL, h.storage, userID)
	if err != nil {
		var dupErr *DuplicateURLError
		if errors.As(err, &dupErr) {
			resp := models.ShortURLResponse{
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

	h.audit.Publish(r.Context(), audit.Event{
		TS:     time.Now().Unix(),
		Action: audit.ActionShorten,
		UserID: userID,
		URL:    req.URL,
	})

	resp := models.ShortURLResponse{
		Result: shortLink.GetFullLink(h.cfg.BaseURL),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(shortLink.GetStatusCode())
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Log.Error("failed to encode response", zap.Error(err))
	}
}

// PingHandler проверяет доступность БД.
//
// Если h.db равен nil, сразу возвращает 200 OK с телом "OK".
// Иначе вызывает h.db.Ping(ctx).
// При ошибке возвращает 500 Internal Server Error.
// При успехе — 200 OK с телом "OK".
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

// BatchCreateShortURL пакетно создаёт короткие ссылки из JSON-массива
// models.BatchRequestItem.
//
// Ожидает userID в контексте по ключу user.UserIDContextKey.
// При ошибке чтения тела — 500 Internal Server Error.
// При некорректном JSON или пустом массиве — 400 Bad Request.
// Новые элементы сохраняются через h.storage.BatchSave.
// При ошибке сохранения — 500 Internal Server Error.
// При успехе возвращает 201 Created и JSON-массив, построенный
// buildBatchResponse.
//
// Для каждого элемента публикует событие аудита audit.ActionShorten.
func (h *Handler) BatchCreateShortURL(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(user.UserIDContextKey).(string)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	var reqItems []models.BatchRequestItem
	if err := json.Unmarshal(body, &reqItems); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if len(reqItems) == 0 {
		http.Error(w, "Empty batch", http.StatusBadRequest)
		return
	}

	urlMap, newItems, err := prepareBatchItems(r.Context(), reqItems, h.storage, userID)
	if err != nil {
		var appErr models.AppError
		if errors.As(err, &appErr) {
			http.Error(w, appErr.Error(), appErr.Status)
		} else {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	if len(newItems) > 0 {
		if err := h.storage.BatchSave(r.Context(), newItems, userID); err != nil {
			logger.Log.Error("failed to save batch", zap.Error(err))
			http.Error(w, "Failed to save batch", http.StatusInternalServerError)
			return
		}
	}

	for _, item := range reqItems {
		h.audit.Publish(r.Context(), audit.Event{
			TS:     time.Now().Unix(),
			Action: audit.ActionShorten,
			UserID: userID,
			URL:    item.OriginalURL,
		})
	}

	respItems := buildBatchResponse(reqItems, urlMap, h.cfg.BaseURL)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(respItems); err != nil {
		logger.Log.Error("failed to encode response", zap.Error(err))
	}
}

// GetUserURLs возвращает все короткие ссылки текущего пользователя.
//
// Ожидает userID в контексте по ключу user.UserIDContextKey.
// Если у пользователя нет ссылок, возвращает 204 No Content.
// При успехе возвращает 200 OK и JSON-массив models.URLPair,
// где ShortURL — полная короткая ссылка на базе h.cfg.BaseURL.
// При ошибке хранилища — 500 Internal Server Error.
func (h *Handler) GetUserURLs(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(user.UserIDContextKey).(string)
	pairs, err := h.storage.GetAllByUser(r.Context(), userID)
	if err != nil {
		logger.Log.Error("failed to get user URLs", zap.Error(err))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if len(pairs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := make([]models.URLPair, len(pairs))
	for i, p := range pairs {
		resp[i] = models.URLPair{
			ShortURL:    h.cfg.BaseURL + "/" + p.ShortURL,
			OriginalURL: p.OriginalURL,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Log.Error("failed to encode response", zap.Error(err))
	}
}

// DeleteUserURLs ставит в очередь удаление коротких ссылок текущего
// пользователя.
//
// Ожидает userID в контексте по ключу user.UserIDContextKey.
// Тело запроса — JSON-массив строковых идентификаторов.
// При ошибке чтения тела, некорректном JSON или пустом списке —
// 400 Bad Request.
//
// Задачи отправляются в h.deleteChan без блокировки:
//   - если контекст сервера завершён — 503 Service Unavailable;
//   - если канал переполнен — 503 Service Unavailable.
//
// При успешной постановке в очередь возвращает 202 Accepted.
// Удаление выполняется асинхронно.
func (h *Handler) DeleteUserURLs(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(user.UserIDContextKey).(string)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var ids []string
	if err := json.Unmarshal(body, &ids); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if len(ids) == 0 {
		http.Error(w, "Empty list", http.StatusBadRequest)
		return
	}

	for _, id := range ids {
		select {
		case h.deleteChan <- deleteTask{UserID: userID, ID: id}:
			// успешно отправлено
		case <-h.ctx.Done():
			// сервер завершается – больше не принимаем задачи
			http.Error(w, "Server is shutting down", http.StatusServiceUnavailable)
			return
		default:
			// канал переполнен – возвращаем ошибку
			http.Error(w, "Server overloaded, unable to queue deletion", http.StatusServiceUnavailable)
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
}
