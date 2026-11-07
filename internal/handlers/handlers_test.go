package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/AlexeyKurlevsky/shortener/internal/audit"
	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/core"
	"github.com/AlexeyKurlevsky/shortener/internal/models"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"github.com/AlexeyKurlevsky/shortener/internal/user"

	storage_mocks "github.com/AlexeyKurlevsky/shortener/internal/storage/mocks"
)

// ------------------------------------------------------------
// Заглушки аудита
// ------------------------------------------------------------

// nopPublisher — для тестов, где события аудита не важны.
type nopPublisher struct{}

func (nopPublisher) Publish(audit.Event) {}

type spyPublisher struct {
	mu     sync.Mutex
	events []audit.Event
}

func (s *spyPublisher) Publish(e audit.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
}

func (s *spyPublisher) snapshot() []audit.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]audit.Event, len(s.events))
	copy(out, s.events)
	return out
}

// ------------------------------------------------------------
// Тестовые вспомогательные типы и функции
// ------------------------------------------------------------

type dummyPinger struct{}

func (d dummyPinger) Ping(ctx context.Context) error { return nil }

// setupTest создаёт Handler с mock-хранилищем и заглушкой аудита.
func setupTest(mock *storage_mocks.MockStorage) *Handler {
	return setupTestWithAudit(mock, nopPublisher{})
}

// setupTestWithAudit — вариант с явным паблишером (для проверок аудита).
func setupTestWithAudit(mock *storage_mocks.MockStorage, pub AuditPublisher) *Handler {
	cfg := &config.Config{
		ServerAddr: ":8080",
		BaseURL:    "http://localhost:8080",
	}
	return NewHandler(mock, cfg, dummyPinger{}, pub)
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
			if got := core.IsValidURL(tt.url); got != tt.want {
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
			mock := &storage_mocks.MockStorage{
				FindIDByURLFunc: tt.mockFind,
				ExistsFunc:      tt.mockExists,
				SaveFunc:        tt.mockSave,
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
			body:           models.CreateURLRequest{URL: "https://example.com"},
			mockFind:       func(ctx context.Context, url string) (string, bool) { return "", false },
			mockExists:     func(ctx context.Context, id string) bool { return false },
			mockSave:       func(ctx context.Context, id, url, userID string) error { return nil },
			wantStatus:     http.StatusCreated,
			wantBodyResult: "http://localhost:8080/",
		},
		{
			name:           "existing URL – returns 409 Conflict",
			body:           models.CreateURLRequest{URL: "https://example.com"},
			mockFind:       func(ctx context.Context, url string) (string, bool) { return "abc123", true },
			wantStatus:     http.StatusConflict,
			wantBodyResult: "http://localhost:8080/abc123",
		},
		{
			name:       "invalid URL",
			body:       models.CreateURLRequest{URL: "not-a-url"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "malformed JSON",
			body:       "invalid json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "save fails",
			body:       models.CreateURLRequest{URL: "https://example.com"},
			mockFind:   func(ctx context.Context, url string) (string, bool) { return "", false },
			mockExists: func(ctx context.Context, id string) bool { return false },
			mockSave:   func(ctx context.Context, id, url, userID string) error { return errors.New("storage error") },
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &storage_mocks.MockStorage{
				FindIDByURLFunc: tt.mockFind,
				ExistsFunc:      tt.mockExists,
				SaveFunc:        tt.mockSave,
			}
			h := setupTest(mock)

			var bodyBytes []byte
			switch v := tt.body.(type) {
			case models.CreateURLRequest:
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
				var resp models.ShortURLResponse
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
			h := NewHandler(nil, cfg, mock, nopPublisher{})

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
			mock := &storage_mocks.MockStorage{GetFunc: tt.mockGet}
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
			mockDelete: nil,
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
			wantStatus: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &storage_mocks.MockStorage{DeleteURLsFunc: tt.mockDelete}
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

// ------------------------------------------------------------
// Тесты аудита
// ------------------------------------------------------------

func TestAuditOnShorten(t *testing.T) {
	mock := &storage_mocks.MockStorage{
		FindIDByURLFunc: func(ctx context.Context, url string) (string, bool) { return "", false },
		ExistsFunc:      func(ctx context.Context, id string) bool { return false },
		SaveFunc:        func(ctx context.Context, id, url, userID string) error { return nil },
	}
	spy := &spyPublisher{}
	h := setupTestWithAudit(mock, spy)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://example.com"))
	req = setUserContext(req, testUserID)
	w := httptest.NewRecorder()
	h.CreateShortURL(w, req)

	events := spy.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	e := events[0]
	if e.Action != audit.ActionShorten {
		t.Errorf("action = %q, want %q", e.Action, audit.ActionShorten)
	}
	if e.UserID != testUserID {
		t.Errorf("user_id = %q, want %q", e.UserID, testUserID)
	}
	if e.URL != "https://example.com" {
		t.Errorf("url = %q, want %q", e.URL, "https://example.com")
	}
	if e.TS == 0 {
		t.Errorf("ts is zero")
	}
}

func TestAuditOnFollow(t *testing.T) {
	mock := &storage_mocks.MockStorage{
		GetFunc: func(ctx context.Context, id string) (string, error) {
			return "https://example.com/target", nil
		},
	}
	spy := &spyPublisher{}
	h := setupTestWithAudit(mock, spy)

	req := httptest.NewRequest(http.MethodGet, "/{id}", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetLink(w, req)

	events := spy.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	e := events[0]
	if e.Action != audit.ActionFollow {
		t.Errorf("action = %q, want %q", e.Action, audit.ActionFollow)
	}
	if e.URL != "https://example.com/target" {
		t.Errorf("url = %q, want %q", e.URL, "https://example.com/target")
	}
	// userID для follow может быть пустым — это допустимо.
}

// Проверяем, что при ошибке (409 Conflict / 404) событие НЕ публикуется.
func TestAuditNotPublishedOnFailure(t *testing.T) {
	t.Run("shorten conflict", func(t *testing.T) {
		mock := &storage_mocks.MockStorage{
			FindIDByURLFunc: func(ctx context.Context, url string) (string, bool) {
				return "abc123", true
			},
		}
		spy := &spyPublisher{}
		h := setupTestWithAudit(mock, spy)

		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://example.com"))
		req = setUserContext(req, testUserID)
		w := httptest.NewRecorder()
		h.CreateShortURL(w, req)

		if got := len(spy.snapshot()); got != 0 {
			t.Errorf("expected 0 audit events on conflict, got %d", got)
		}
	})

	t.Run("follow not found", func(t *testing.T) {
		mock := &storage_mocks.MockStorage{
			GetFunc: func(ctx context.Context, id string) (string, error) {
				return "", storage.ErrNotFound
			},
		}
		spy := &spyPublisher{}
		h := setupTestWithAudit(mock, spy)

		req := httptest.NewRequest(http.MethodGet, "/{id}", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "missing")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		h.GetLink(w, req)

		if got := len(spy.snapshot()); got != 0 {
			t.Errorf("expected 0 audit events on 404, got %d", got)
		}
	})
}

func TestGetStats(t *testing.T) {
	tests := []struct {
		name       string
		countURLs  func(ctx context.Context) (int, error)
		countUsers func(ctx context.Context) (int, error)
		wantStatus int
		wantURLs   int
		wantUsers  int
	}{
		{
			name:       "success",
			countURLs:  func(ctx context.Context) (int, error) { return 42, nil },
			countUsers: func(ctx context.Context) (int, error) { return 7, nil },
			wantStatus: http.StatusOK,
			wantURLs:   42,
			wantUsers:  7,
		},
		{
			name:       "empty service",
			countURLs:  func(ctx context.Context) (int, error) { return 0, nil },
			countUsers: func(ctx context.Context) (int, error) { return 0, nil },
			wantStatus: http.StatusOK,
			wantURLs:   0,
			wantUsers:  0,
		},
		{
			name:       "countURLs error",
			countURLs:  func(ctx context.Context) (int, error) { return 0, errors.New("db error") },
			countUsers: func(ctx context.Context) (int, error) { return 0, nil },
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "countUsers error",
			countURLs:  func(ctx context.Context) (int, error) { return 10, nil },
			countUsers: func(ctx context.Context) (int, error) { return 0, errors.New("db error") },
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &storage_mocks.MockStorage{
				CountURLsFunc:  tt.countURLs,
				CountUsersFunc: tt.countUsers,
			}
			h := setupTest(mock)

			req := httptest.NewRequest(http.MethodGet, "/api/internal/stats", nil)
			w := httptest.NewRecorder()
			h.GetStats(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}

			if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			var resp models.StatsResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.URLs != tt.wantURLs {
				t.Errorf("urls = %d, want %d", resp.URLs, tt.wantURLs)
			}
			if resp.Users != tt.wantUsers {
				t.Errorf("users = %d, want %d", resp.Users, tt.wantUsers)
			}
		})
	}
}

// ------------------------------------------------------------
// Примеры (Examples)
// ------------------------------------------------------------

// Example демонстрирует базовый сценарий работы пакета:
// сокращение URL и последующий редирект по короткому идентификатору.
func Example() {
	mock := &storage_mocks.MockStorage{
		FindIDByURLFunc: func(ctx context.Context, url string) (string, bool) { return "", false },
		ExistsFunc:      func(ctx context.Context, id string) bool { return false },
		SaveFunc:        func(ctx context.Context, id, url, userID string) error { return nil },
	}
	h := setupTest(mock)

	// 1. Создаём короткую ссылку.
	createReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://example.com"))
	createReq = setUserContext(createReq, testUserID)
	createRec := httptest.NewRecorder()
	h.CreateShortURL(createRec, createReq)

	fmt.Println("create status:", createRec.Code)
	fmt.Println("prefix ok:", strings.HasPrefix(createRec.Body.String(), "http://localhost:8080/"))
	// Output:
	// create status: 201
	// prefix ok: true
}

// ExampleIsValidURL показывает, какие строки считаются корректными URL.
func ExampleIsValidURL() {
	fmt.Println(core.IsValidURL("https://example.com"))
	fmt.Println(core.IsValidURL("http://localhost:8080/path?q=1"))
	fmt.Println(core.IsValidURL("ftp://example.com"))
	fmt.Println(core.IsValidURL("not-a-url"))
	// Output:
	// true
	// true
	// false
	// false
}

// ExampleHandler_CreateShortURL показывает сокращение URL, пришедшего
// в теле запроса как text/plain.
func ExampleHandler_CreateShortURL() {
	mock := &storage_mocks.MockStorage{
		FindIDByURLFunc: func(ctx context.Context, url string) (string, bool) { return "", false },
		ExistsFunc:      func(ctx context.Context, id string) bool { return false },
		SaveFunc:        func(ctx context.Context, id, url, userID string) error { return nil },
	}
	h := setupTest(mock)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://example.com"))
	req = setUserContext(req, testUserID)
	rec := httptest.NewRecorder()

	h.CreateShortURL(rec, req)

	fmt.Println("status:", rec.Code)
	fmt.Println("content-type:", rec.Header().Get("Content-Type"))
	fmt.Println("prefix ok:", strings.HasPrefix(rec.Body.String(), "http://localhost:8080/"))
	// Output:
	// status: 201
	// content-type: text/plain
	// prefix ok: true
}

// ExampleHandler_CreateShortURL_conflict показывает обработку дубликата:
// если URL уже сохранён, возвращается 409 Conflict и существующая ссылка.
func ExampleHandler_CreateShortURL_conflict() {
	mock := &storage_mocks.MockStorage{
		FindIDByURLFunc: func(ctx context.Context, url string) (string, bool) {
			return "abc123", true
		},
	}
	h := setupTest(mock)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://example.com"))
	req = setUserContext(req, testUserID)
	rec := httptest.NewRecorder()

	h.CreateShortURL(rec, req)

	fmt.Println("status:", rec.Code)
	fmt.Println("body:", rec.Body.String())
	// Output:
	// status: 409
	// body: http://localhost:8080/abc123
}

// ExampleHandler_CreateShortURLJson показывает сокращение URL из JSON-запроса.
func ExampleHandler_CreateShortURLJson() {
	mock := &storage_mocks.MockStorage{
		FindIDByURLFunc: func(ctx context.Context, url string) (string, bool) { return "", false },
		ExistsFunc:      func(ctx context.Context, id string) bool { return false },
		SaveFunc:        func(ctx context.Context, id, url, userID string) error { return nil },
	}
	h := setupTest(mock)

	body := strings.NewReader(`{"url":"https://example.com/api/shorten"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", body)
	req.Header.Set("Content-Type", "application/json")
	req = setUserContext(req, testUserID)
	rec := httptest.NewRecorder()

	h.CreateShortURLJson(rec, req)

	var resp models.ShortURLResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	fmt.Println("status:", rec.Code)
	fmt.Println("content-type:", rec.Header().Get("Content-Type"))
	fmt.Println("prefix ok:", strings.HasPrefix(resp.Result, "http://localhost:8080/"))
	// Output:
	// status: 201
	// content-type: application/json
	// prefix ok: true
}

// ExampleHandler_GetLink показывает редирект по короткому идентификатору.
func ExampleHandler_GetLink() {
	mock := &storage_mocks.MockStorage{
		GetFunc: func(ctx context.Context, id string) (string, error) {
			return "https://example.com/target", nil
		},
	}
	h := setupTest(mock)

	req := httptest.NewRequest(http.MethodGet, "/abc123", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.GetLink(rec, req)

	fmt.Println("status:", rec.Code)
	fmt.Println("location:", rec.Header().Get("Location"))
	// Output:
	// status: 307
	// location: https://example.com/target
}

// ExampleHandler_GetLink_notFound показывает ответ 404 для неизвестного id.
func ExampleHandler_GetLink_notFound() {
	mock := &storage_mocks.MockStorage{
		GetFunc: func(ctx context.Context, id string) (string, error) {
			return "", storage.ErrNotFound
		},
	}
	h := setupTest(mock)

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "missing")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.GetLink(rec, req)

	fmt.Println("status:", rec.Code)
	// Output:
	// status: 404
}

// ExampleHandler_PingHandler показывает успешную проверку БД.
func ExampleHandler_PingHandler() {
	mock := &storage_mocks.MockStorage{}
	h := setupTest(mock)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	h.PingHandler(rec, req)

	fmt.Println("status:", rec.Code)
	fmt.Println("body:", rec.Body.String())
	// Output:
	// status: 200
	// body: OK
}

// ExampleHandler_PingHandler_failure показывает ответ 500 при недоступной БД.
func ExampleHandler_PingHandler_failure() {
	mock := &storage_mocks.MockStorage{}
	cfg := &config.Config{ServerAddr: ":8080", BaseURL: "http://localhost:8080"}
	badPinger := mockPinger{
		pingFunc: func(ctx context.Context) error { return errors.New("connection refused") },
	}
	h := NewHandler(mock, cfg, badPinger, nopPublisher{})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	h.PingHandler(rec, req)

	fmt.Println("status:", rec.Code)
	// Output:
	// status: 500
}

// ExampleHandler_GetUserURLs показывает список ссылок пользователя.
func ExampleHandler_GetUserURLs() {
	mock := &storage_mocks.MockStorage{
		GetAllByUserFunc: func(ctx context.Context, userID string) ([]storage.URLPair, error) {
			return []storage.URLPair{
				{ShortURL: "aaa111", OriginalURL: "https://example.com/a"},
			}, nil
		},
	}
	h := setupTest(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)
	req = setUserContext(req, testUserID)
	rec := httptest.NewRecorder()

	h.GetUserURLs(rec, req)

	var resp []models.URLPair
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	fmt.Println("status:", rec.Code)
	fmt.Println("items:", len(resp))
	fmt.Println("short:", resp[0].ShortURL)
	fmt.Println("original:", resp[0].OriginalURL)
	// Output:
	// status: 200
	// items: 1
	// short: http://localhost:8080/aaa111
	// original: https://example.com/a
}

// ExampleHandler_GetUserURLs_empty показывает 204 No Content,
// если у пользователя нет ссылок.
func ExampleHandler_GetUserURLs_empty() {
	mock := &storage_mocks.MockStorage{
		GetAllByUserFunc: func(ctx context.Context, userID string) ([]storage.URLPair, error) {
			return nil, nil
		},
	}
	h := setupTest(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)
	req = setUserContext(req, testUserID)
	rec := httptest.NewRecorder()

	h.GetUserURLs(rec, req)

	fmt.Println("status:", rec.Code)
	// Output:
	// status: 204
}

// ExampleHandler_BatchCreateShortURL показывает пакетное сокращение URL.
func ExampleHandler_BatchCreateShortURL() {
	mock := &storage_mocks.MockStorage{
		FindIDByURLFunc: func(ctx context.Context, url string) (string, bool) { return "", false },
		ExistsFunc:      func(ctx context.Context, id string) bool { return false },
		BatchSaveFunc:   func(ctx context.Context, items []storage.BatchItem, userID string) error { return nil },
	}
	h := setupTest(mock)

	body := strings.NewReader(`[
		{"correlation_id":"a","original_url":"https://example.com/1"},
		{"correlation_id":"b","original_url":"https://example.com/2"}
	]`)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten/batch", body)
	req.Header.Set("Content-Type", "application/json")
	req = setUserContext(req, testUserID)
	rec := httptest.NewRecorder()

	h.BatchCreateShortURL(rec, req)

	var resp []models.BatchResponseItem
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	fmt.Println("status:", rec.Code)
	fmt.Println("items:", len(resp))
	fmt.Println("first correlation_id:", resp[0].CorrelationID)
	fmt.Println("first prefix ok:", strings.HasPrefix(resp[0].ShortURL, "http://localhost:8080/"))
	// Output:
	// status: 201
	// items: 2
	// first correlation_id: a
	// first prefix ok: true
}

// ExampleHandler_DeleteUserURLs показывает асинхронную постановку задач
// на удаление: обработчик возвращает 202 Accepted.
func ExampleHandler_DeleteUserURLs() {
	mock := &storage_mocks.MockStorage{}
	h := setupTest(mock)

	body := strings.NewReader(`["aaa111","bbb222"]`)
	req := httptest.NewRequest(http.MethodDelete, "/api/user/urls", body)
	req.Header.Set("Content-Type", "application/json")
	req = setUserContext(req, testUserID)
	rec := httptest.NewRecorder()

	h.DeleteUserURLs(rec, req)

	fmt.Println("status:", rec.Code)
	// Output:
	// status: 202
}

// ExampleHandler_GetStats показывает ответ эндпоинта статистики.
func ExampleHandler_GetStats() {
	mock := &storage_mocks.MockStorage{
		CountURLsFunc:  func(ctx context.Context) (int, error) { return 42, nil },
		CountUsersFunc: func(ctx context.Context) (int, error) { return 7, nil },
	}
	h := setupTest(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/internal/stats", nil)
	rec := httptest.NewRecorder()
	h.GetStats(rec, req)

	fmt.Println("status:", rec.Code)
	fmt.Println("body:", rec.Body.String())
	// Output:
	// status: 200
	// body: {"urls":42,"users":7}
}
