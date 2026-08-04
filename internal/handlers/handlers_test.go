package handlers

import (
	"bytes"
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
	"github.com/go-chi/chi/v5"
)

type mockStorage struct {
	findIDByURLFunc func(url string) (string, bool)
	existsFunc      func(id string) bool
	saveFunc        func(id, url string) error
	getFunc         func(id string) (string, error)
	loadFunc        func() error
	saveToFileFunc  func() error
}

func (m *mockStorage) FindIDByURL(url string) (string, bool) {
	if m.findIDByURLFunc != nil {
		return m.findIDByURLFunc(url)
	}
	return "", false
}

func (m *mockStorage) Exists(id string) bool {
	if m.existsFunc != nil {
		return m.existsFunc(id)
	}
	return false
}

func (m *mockStorage) Save(id, url string) error {
	if m.saveFunc != nil {
		return m.saveFunc(id, url)
	}
	return nil
}

func (m *mockStorage) Get(id string) (string, error) {
	if m.getFunc != nil {
		return m.getFunc(id)
	}
	return "", storage.ErrNotFound
}

func (m *mockStorage) Load() error {
	if m.loadFunc != nil {
		return m.loadFunc()
	}
	return nil
}

func (m *mockStorage) SaveToFile() error {
	if m.saveToFileFunc != nil {
		return m.saveToFileFunc()
	}
	return nil
}

func setupTest(mock *mockStorage) *Handler {
	cfg := &config.Config{
		ServerAddr: ":8080",
		BaseURL:    "http://localhost:8080",
	}
	return NewHandler(mock, cfg)
}

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
		mockFind       func(string) (string, bool)
		mockExists     func(string) bool
		mockSave       func(string, string) error
		wantStatus     int
		wantBodyPrefix string
	}{
		{
			name:           "success new URL",
			body:           "https://example.com",
			mockFind:       func(string) (string, bool) { return "", false },
			mockExists:     func(string) bool { return false },
			mockSave:       func(string, string) error { return nil },
			wantStatus:     http.StatusCreated,
			wantBodyPrefix: "http://localhost:8080/",
		},
		{
			name:           "existing URL",
			body:           "https://example.com",
			mockFind:       func(string) (string, bool) { return "abc123", true },
			wantStatus:     http.StatusOK,
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

func TestGetLink(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		mockGet    func(string) (string, error)
		wantStatus int
		wantHeader string
	}{
		{
			name:       "success",
			id:         "abc123",
			mockGet:    func(string) (string, error) { return "https://original.com", nil },
			wantStatus: http.StatusTemporaryRedirect,
			wantHeader: "https://original.com",
		},
		{
			name:       "not found",
			id:         "notfound",
			mockGet:    func(string) (string, error) { return "", storage.ErrNotFound },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "storage error",
			id:         "error",
			mockGet:    func(string) (string, error) { return "", errors.New("db error") },
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "empty id",
			id:         "",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "id with slash",
			id:         "abc/def",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStorage{getFunc: tt.mockGet}
			h := setupTest(mock)

			// Use a chi router to set the URL parameter.
			r := chi.NewRouter()
			r.Get("/{id}", h.GetLink)
			req := httptest.NewRequest(http.MethodGet, "/"+tt.id, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}

			if tt.wantHeader != "" {
				if loc := res.Header.Get("Location"); loc != tt.wantHeader {
					t.Errorf("Location = %q, want %q", loc, tt.wantHeader)
				}
			}
		})
	}
}

func TestCreateShortURLJson(t *testing.T) {
	tests := []struct {
		name           string
		body           interface{}
		mockFind       func(string) (string, bool)
		mockExists     func(string) bool
		mockSave       func(string, string) error
		wantStatus     int
		wantBodyResult string
	}{
		{
			name:           "success new URL",
			body:           models.CreateUrlRequest{Url: "https://example.com"},
			mockFind:       func(string) (string, bool) { return "", false },
			mockExists:     func(string) bool { return false },
			mockSave:       func(string, string) error { return nil },
			wantStatus:     http.StatusCreated,
			wantBodyResult: "http://localhost:8080/",
		},
		{
			name:           "existing URL",
			body:           models.CreateUrlRequest{Url: "https://example.com"},
			mockFind:       func(string) (string, bool) { return "abc123", true },
			wantStatus:     http.StatusCreated, // as per code: returns 201 even for existing
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
			mockFind:   func(string) (string, bool) { return "", false },
			mockExists: func(string) bool { return false },
			mockSave:   func(string, string) error { return errors.New("storage error") },
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
