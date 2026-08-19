package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/models"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"github.com/AlexeyKurlevsky/shortener/internal/user"
	"github.com/go-chi/chi/v5"
)

// ------------------------------------------------------------
// Тестовые вспомогательные типы и функции
// ------------------------------------------------------------

type dummyPinger struct{}

func (d dummyPinger) Ping(ctx context.Context) error { return nil }

// mockStorage реализует storage.Storage.
type mockStorage struct {
	findIDByURLFunc  func(ctx context.Context, url string) (string, bool)
	existsFunc       func(ctx context.Context, id string) bool
	saveFunc         func(ctx context.Context, id, url, userID string) error
	getFunc          func(ctx context.Context, id string) (string, error)
	loadFunc         func(ctx context.Context) error
	saveToFileFunc   func(ctx context.Context) error
	batchSaveFunc    func(ctx context.Context, items []storage.BatchItem, userID string) error
	getAllByUserFunc func(ctx context.Context, userID string) ([]storage.URLPair, error)
	deleteURLsFunc   func(ctx context.Context, ids []string, userID string) error // новый метод
}

func (m *mockStorage) FindIDByURL(ctx context.Context, url string) (string, bool) {
	if m.findIDByURLFunc != nil {
		return m.findIDByURLFunc(ctx, url)
	}
	return "", false
}

func (m *mockStorage) Exists(ctx context.Context, id string) bool {
	if m.existsFunc != nil {
		return m.existsFunc(ctx, id)
	}
	return false
}

func (m *mockStorage) Save(ctx context.Context, id, url, userID string) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, id, url, userID)
	}
	return nil
}

func (m *mockStorage) Get(ctx context.Context, id string) (string, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, id)
	}
	return "", storage.ErrNotFound
}

func (m *mockStorage) Load(ctx context.Context) error {
	if m.loadFunc != nil {
		return m.loadFunc(ctx)
	}
	return nil
}

func (m *mockStorage) SaveToFile(ctx context.Context) error {
	if m.saveToFileFunc != nil {
		return m.saveToFileFunc(ctx)
	}
	return nil
}

func (m *mockStorage) BatchSave(ctx context.Context, items []storage.BatchItem, userID string) error {
	if m.batchSaveFunc != nil {
		return m.batchSaveFunc(ctx, items, userID)
	}
	return nil
}

func (m *mockStorage) GetAllByUser(ctx context.Context, userID string) ([]storage.URLPair, error) {
	if m.getAllByUserFunc != nil {
		return m.getAllByUserFunc(ctx, userID)
	}
	return nil, nil
}

// DeleteURLs – реализация нового метода
func (m *mockStorage) DeleteURLs(ctx context.Context, ids []string, userID string) error {
	if m.deleteURLsFunc != nil {
		return m.deleteURLsFunc(ctx, ids, userID)
	}
	return nil
}

// setupTest создаёт Handler с заданным mock-хранилищем и фиктивным Pinger.
func setupTest(mock *mockStorage) *Handler {
	cfg := &config.Config{
		ServerAddr: ":8080",
		BaseURL:    "http://localhost:8080",
	}
	return NewHandler(mock, cfg, dummyPinger{})
}

// mockPinger для тестирования PingHandler
type mockPinger struct {
	pingFunc func(ctx context.Context) error
}

func (m mockPinger) Ping(ctx context.Context) error {
	if m.pingFunc != nil {
		return m.pingFunc(ctx)
	}
	return nil
}

const testUserID = "test-user-id"

// setUserContext добавляет userID в контекст запроса с помощью экспортируемой функции user.WithUserID.
func setUserContext(r *http.Request, userID string) *http.Request {
	ctx := user.WithUserID(r.Context(), userID)
	return r.WithContext(ctx)
}

// ------------------------------------------------------------
// Тесты
// ------------------------------------------------------------

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"valid http", "http://example.com", true},
		{"valid https", "https://example.com/path", true},
		{"no scheme", "example.com", false},
		{"invalid scheme", "ftp://example.com", false},
		{"empty", "", false},
		{"just host no scheme", "example", false},
		{"http without host", "http://", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidURL(tt.url); got != tt.want {
				t.Errorf("IsValidURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestCreateShortURL(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		mockFind       func(ctx context.Context, url string) (string, bool)
		mockExists     func(ctx context.Context, id string) bool
		mockSave       func(ctx context.Context, id, url, userID string) error
		wantStatus     int
		wantBodyPrefix string
	}{
		{
			name:           "success new URL",
			body:           "https://example.com",
			mockFind:       func(ctx context.Context, url string) (string, bool) { return "", false },
			mockExists:     func(ctx context.Context, id string) bool { return false },
			mockSave:       func(ctx context.Context, id, url, userID string) error { return nil },
			wantStatus:     http.StatusCreated,
			wantBodyPrefix: "http://localhost:8080/",
		},
		{
			name:           "existing URL – returns 409 Conflict",
			body:           "https://example.com",
			mockFind:       func(ctx context.Context, url string) (string, bool) { return "abc123", true },
			wantStatus:     http.StatusConflict,
			wantBodyPrefix: "http://localhost:8080/abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStorage{
				findIDByURLFunc: tt.mockFind,
				existsFunc:      tt.mockExists,
				saveFunc:        tt.mockSave,
			}
			h := setupTest(mock)

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			req = setUserContext(req, testUserID)

			w := httptest.NewRecorder()
			h.CreateShortURL(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}

			if tt.wantBodyPrefix != "" {
				bodyBytes, err := io.ReadAll(res.Body)
				if err != nil {
					t.Fatalf("failed to read response body: %v", err)
				}
				body := string(bodyBytes)
				if !strings.HasPrefix(body, tt.wantBodyPrefix) {
					t.Errorf("body = %q, want prefix %q", body, tt.wantBodyPrefix)
				}
			}
		})
	}
}

func TestCreateShortURLJson(t *testing.T) {
	tests := []struct {
		name           string
		body           interface{}
		mockFind       func(ctx context.Context, url string) (string, bool)
		mockExists     func(ctx context.Context, id string) bool
		mockSave       func(ctx context.Context, id, url, userID string) error
		wantStatus     int
		wantBodyResult string
	}{
		{
			name:           "success new URL",
			body:           models.CreateUrlRequest{Url: "https://example.com"},
			mockFind:       func(ctx context.Context, url string) (string, bool) { return "", false },
			mockExists:     func(ctx context.Context, id string) bool { return false },
			mockSave:       func(ctx context.Context, id, url, userID string) error { return nil },
			wantStatus:     http.StatusCreated,
			wantBodyResult: "http://localhost:8080/",
		},
		{
			name:           "existing URL – returns 409 Conflict",
			body:           models.CreateUrlRequest{Url: "https://example.com"},
			mockFind:       func(ctx context.Context, url string) (string, bool) { return "abc123", true },
			wantStatus:     http.StatusConflict,
			wantBodyResult: "http://localhost:8080/abc123",
		},
		{
			name:       "invalid URL",
			body:       models.CreateUrlRequest{Url: "not-a-url"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "malformed JSON",
			body:       "invalid json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "save fails",
			body:       models.CreateUrlRequest{Url: "https://example.com"},
			mockFind:   func(ctx context.Context, url string) (string, bool) { return "", false },
			mockExists: func(ctx context.Context, id string) bool { return false },
			mockSave:   func(ctx context.Context, id, url, userID string) error { return errors.New("storage error") },
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStorage{
				findIDByURLFunc: tt.mockFind,
				existsFunc:      tt.mockExists,
				saveFunc:        tt.mockSave,
			}
			h := setupTest(mock)

			var bodyBytes []byte
			switch v := tt.body.(type) {
			case models.CreateUrlRequest:
				var err error
				bodyBytes, err = json.Marshal(v)
				if err != nil {
					t.Fatalf("failed to marshal request: %v", err)
				}
			case string:
				bodyBytes = []byte(v)
			default:
				t.Fatalf("unsupported body type")
			}

			req := httptest.NewRequest(http.MethodPost, "/api/shorten", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req = setUserContext(req, testUserID)

			w := httptest.NewRecorder()
			h.CreateShortURLJson(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}

			if tt.wantBodyResult != "" {
				var resp models.ShortUrlResponse
				if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if tt.wantBodyResult == "http://localhost:8080/" {
					if !strings.HasPrefix(resp.Result, tt.wantBodyResult) {
						t.Errorf("result = %q, want prefix %q", resp.Result, tt.wantBodyResult)
					}
				} else {
					if resp.Result != tt.wantBodyResult {
						t.Errorf("result = %q, want %q", resp.Result, tt.wantBodyResult)
					}
				}
			}
		})
	}
}

func TestPingHandler(t *testing.T) {
	tests := []struct {
		name       string
		pingError  error
		wantStatus int
	}{
		{
			name:       "successful ping",
			pingError:  nil,
			wantStatus: http.StatusOK,
		},
		{
			name:       "failed ping",
			pingError:  errors.New("connection refused"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := mockPinger{
				pingFunc: func(ctx context.Context) error {
					return tt.pingError
				},
			}
			cfg := &config.Config{ServerAddr: ":8080", BaseURL: "http://localhost:8080"}
			h := NewHandler(nil, cfg, mock)

			req := httptest.NewRequest(http.MethodGet, "/ping", nil)
			w := httptest.NewRecorder()
			h.PingHandler(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				body, _ := io.ReadAll(res.Body)
				if string(body) != "OK" {
					t.Errorf("body = %q, want %q", body, "OK")
				}
			}
		})
	}
}

// TestGetLink проверяет получение оригинального URL и статусы 404, 410, 302.
func TestGetLink(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		mockGet      func(ctx context.Context, id string) (string, error)
		wantStatus   int
		wantLocation string
	}{
		{
			name:         "successful redirect",
			id:           "abc123",
			mockGet:      func(ctx context.Context, id string) (string, error) { return "https://example.com", nil },
			wantStatus:   http.StatusTemporaryRedirect,
			wantLocation: "https://example.com",
		},
		{
			name:       "not found",
			id:         "notexist",
			mockGet:    func(ctx context.Context, id string) (string, error) { return "", storage.ErrNotFound },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "gone (deleted)",
			id:         "deleted",
			mockGet:    func(ctx context.Context, id string) (string, error) { return "", storage.ErrGone },
			wantStatus: http.StatusGone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStorage{
				getFunc: tt.mockGet,
			}
			h := setupTest(mock)

			req := httptest.NewRequest(http.MethodGet, "/{id}", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.id)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			h.GetLink(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusTemporaryRedirect {
				if loc := res.Header.Get("Location"); loc != tt.wantLocation {
					t.Errorf("Location = %q, want %q", loc, tt.wantLocation)
				}
			}
		})
	}
}

// TestDeleteUserURLs проверяет хендлер массового удаления.
func TestDeleteUserURLs(t *testing.T) {
	tests := []struct {
		name       string
		body       interface{}
		mockDelete func(ctx context.Context, ids []string, userID string) error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid request",
			body:       []string{"abc123", "def456"},
			mockDelete: func(ctx context.Context, ids []string, userID string) error { return nil },
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "empty list",
			body:       []string{},
			mockDelete: nil, // не должен вызываться
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON",
			body:       "not a json",
			mockDelete: nil,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "storage error",
			body:       []string{"abc123"},
			mockDelete: func(ctx context.Context, ids []string, userID string) error { return errors.New("db error") },
			wantStatus: http.StatusAccepted, // асинхронно, ошибка игнорируется (лог)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStorage{
				deleteURLsFunc: tt.mockDelete,
			}
			h := setupTest(mock)

			var bodyBytes []byte
			switch v := tt.body.(type) {
			case []string:
				var err error
				bodyBytes, err = json.Marshal(v)
				if err != nil {
					t.Fatalf("failed to marshal: %v", err)
				}
			case string:
				bodyBytes = []byte(v)
			default:
				t.Fatalf("unsupported body type")
			}

			req := httptest.NewRequest(http.MethodDelete, "/api/user/urls", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req = setUserContext(req, testUserID)

			w := httptest.NewRecorder()
			h.DeleteUserURLs(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}
		})
	}
}
