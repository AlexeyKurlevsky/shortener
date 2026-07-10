package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testCfg = &config.Config{
	ServerAddr: "localhost:8080",
	BaseURL:    "http://localhost:8080",
}

func TestCreateShortURL(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		body           string
		expectedStatus int
		checkDB        bool
	}{
		{
			name:           "successful_post_valid_url",
			method:         http.MethodPost,
			body:           "https://example.com",
			expectedStatus: http.StatusCreated,
			checkDB:        true,
		},
		{
			name:           "empty_body",
			method:         http.MethodPost,
			body:           "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid_url_no_scheme",
			method:         http.MethodPost,
			body:           "example.com",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid_url_ftp",
			method:         http.MethodPost,
			body:           "ftp://example.com",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := storage.NewMemoryStorage()
			h := NewHandler(st, testCfg)
			r := chi.NewRouter()
			r.Post("/", h.CreateShortURL)

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
				expectedPrefix := testCfg.BaseURL + "/"
				assert.True(t, strings.HasPrefix(bodyStr, expectedPrefix))

				if tt.checkDB {
					parts := strings.Split(bodyStr, "/")
					id := parts[len(parts)-1]
					assert.NotEmpty(t, id)
					stored, err := st.Get(id)
					require.NoError(t, err)
					assert.Equal(t, tt.body, stored)
				}
			}
		})
	}
}

func TestGetLink(t *testing.T) {
	st := storage.NewMemoryStorage()
	err := st.Save("abc123", "https://example.com")
	require.NoError(t, err)

	h := NewHandler(st, testCfg)
	r := chi.NewRouter()
	r.Get("/{id}", h.GetLink)

	tests := []struct {
		name             string
		id               string
		expectedStatus   int
		expectedLocation string
	}{
		{
			name:             "existing_id",
			id:               "abc123",
			expectedStatus:   http.StatusTemporaryRedirect,
			expectedLocation: "https://example.com",
		},
		{
			name:           "non_existing_id",
			id:             "nonexistent",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid_id_with_slash",
			id:             "abc/123",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "empty_id",
			id:             "",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://localhost:8080/"+tt.id, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			if tt.expectedStatus == http.StatusTemporaryRedirect {
				assert.Equal(t, tt.expectedLocation, resp.Header.Get("Location"))
			}
		})
	}
}

func TestCreateShortURLDuplicate(t *testing.T) {
	st := storage.NewMemoryStorage()
	h := NewHandler(st, testCfg)
	r := chi.NewRouter()
	r.Post("/", h.CreateShortURL)

	link := "https://example.com"

	req1 := httptest.NewRequest("POST", "/", strings.NewReader(link))
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	resp1 := w1.Result()
	defer resp1.Body.Close()
	assert.Equal(t, http.StatusCreated, resp1.StatusCode)
	body1, _ := io.ReadAll(resp1.Body)
	shortURL1 := string(body1)

	req2 := httptest.NewRequest("POST", "/", strings.NewReader(link))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	resp2 := w2.Result()
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	body2, _ := io.ReadAll(resp2.Body)
	shortURL2 := string(body2)

	assert.Equal(t, shortURL1, shortURL2)
}
