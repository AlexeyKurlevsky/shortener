package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/handlers"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"github.com/stretchr/testify/assert"
)

func TestRouter(t *testing.T) {
	st := storage.NewMemoryStorage()
	cfg := &config.Config{BaseURL: "http://localhost:8080"}
	h := handlers.NewHandler(st, cfg)
	router := NewRouter(h)

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

	req2 := httptest.NewRequest("GET", "/"+id, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	resp2 := w2.Result()
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusTemporaryRedirect, resp2.StatusCode)
	assert.Equal(t, "https://example.com", resp2.Header.Get("Location"))
}
