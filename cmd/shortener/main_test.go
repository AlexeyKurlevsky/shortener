package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

var testCfg *config.Config

func init() {
	testCfg = &config.Config{
		ServerAddr: "localhost:8080",
		BaseURL:    "http://localhost:8080", // без схемы добавится автоматически, но можно явно
	}
}

func TestCreateShortURL(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		body           string
		expectedStatus int
		expectedBody   string
		checkDB        bool
	}{
		{
			name:           "successful_post_valid_url",
			method:         http.MethodPost,
			body:           "https://example.com",
			expectedStatus: http.StatusCreated,
			checkDB:        true,
		},
		// ... остальные случаи
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db = make(map[string]string)

			r := chi.NewRouter()
			r.Post("/", func(w http.ResponseWriter, req *http.Request) {
				CreateShortURL(w, req, testCfg)
			})

			req := httptest.NewRequest(tt.method, "http://localhost:8080/", strings.NewReader(tt.body))
			req.Host = "localhost:8080"
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == http.StatusCreated {
				assert.Equal(t, "text/plain", resp.Header.Get("Content-Type"))

				bodyBytes, _ := io.ReadAll(resp.Body)
				bodyStr := string(bodyBytes)
				// Ожидаемый префикс: testCfg.BaseURL + "/"
				expectedPrefix := testCfg.BaseURL + "/"
				assert.True(t, strings.HasPrefix(bodyStr, expectedPrefix))

				if tt.checkDB {
					parts := strings.Split(bodyStr, "/")
					id := parts[len(parts)-1]
					assert.NotEmpty(t, id)
					storedLink, ok := db[id]
					assert.True(t, ok)
					assert.Equal(t, tt.body, storedLink)
				}
			}
		})
	}
}

func TestGetLink(t *testing.T) {
	// без изменений
}
