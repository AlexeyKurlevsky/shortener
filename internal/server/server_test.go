package server

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/handlers"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"github.com/AlexeyKurlevsky/shortener/internal/user"
	"github.com/stretchr/testify/assert"
)

type dummyPinger struct{}

func (d dummyPinger) Ping(ctx context.Context) error { return nil }

func TestRouter(t *testing.T) {
	secret := make([]byte, 32)
	_, err := rand.Read(secret)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		BaseURL:       "http://localhost:8080",
		SecretKeyByte: secret,
	}

	st := storage.NewMemoryStorage()
	h := handlers.NewHandler(st, cfg, dummyPinger{})
	userSvc := user.NewUserService(cfg)
	router := NewRouter(h, userSvc)

	// Тест создания короткой ссылки
	req := httptest.NewRequest("POST", "/", strings.NewReader("https://example.com"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	shortURL := string(body)
	assert.True(t, strings.HasPrefix(shortURL, cfg.BaseURL+"/"))
	parts := strings.Split(shortURL, "/")
	id := parts[len(parts)-1]

	// Тест получения оригинального URL по ID
	req2 := httptest.NewRequest("GET", "/"+id, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	resp2 := w2.Result()
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusTemporaryRedirect, resp2.StatusCode)
	assert.Equal(t, "https://example.com", resp2.Header.Get("Location"))
}
