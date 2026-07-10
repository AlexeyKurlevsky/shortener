package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			expectedBody:   "",
			checkDB:        true,
		},
		{
			name:           "invalid_url",
			method:         http.MethodPost,
			body:           "not_a_valid_url",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid link\n",
			checkDB:        false,
		},
		{
			name:           "method_not_allowed_get",
			method:         http.MethodGet,
			body:           "",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   "Only POST requests are allowed!\n",
			checkDB:        false,
		},
		{
			name:           "empty_body",
			method:         http.MethodPost,
			body:           "",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid link\n",
			checkDB:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db = make(map[string]string)

			req := httptest.NewRequest(tt.method, "http://localhost:8080/", strings.NewReader(tt.body))
			req.Host = "localhost:8080"
			w := httptest.NewRecorder()

			CreateShortURL(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "статус ответа не совпадает")

			if tt.expectedStatus == http.StatusCreated {
				assert.Equal(t, "text/plain", resp.Header.Get("Content-Type"), "неправильный Content-Type")
			}

			bodyBytes, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "не удалось прочитать тело ответа")
			bodyStr := string(bodyBytes)

			if tt.expectedBody != "" {
				assert.Equal(t, tt.expectedBody, bodyStr, "тело ответа не совпадает")
			} else {
				expectedPrefix := "http://" + req.Host + "/"
				assert.True(t, strings.HasPrefix(bodyStr, expectedPrefix),
					"ответ должен начинаться с %q, получено %q", expectedPrefix, bodyStr)

				if tt.checkDB {
					parts := strings.Split(bodyStr, "/")
					id := parts[len(parts)-1]
					assert.NotEmpty(t, id, "ID в ответе пустой")

					storedLink, ok := db[id]
					assert.True(t, ok, "ID %q не найден в базе", id)
					assert.Equal(t, tt.body, storedLink, "в базе сохранён неверный URL")
				}
			}
		})
	}
}

func TestGetLink(t *testing.T) {
	// Подготавливаем тестовые данные в глобальной db
	// (будем пересоздавать перед каждым тестом)
	tests := []struct {
		name           string
		method         string
		path           string // полный путь, например "/abc123"
		setupDB        func() // функция для наполнения db перед тестом
		expectedStatus int
		expectedHeader map[string]string // проверка заголовков
		expectedBody   string            // тело ответа (ошибка)
	}{
		{
			name:   "successful_redirect",
			method: http.MethodGet,
			path:   "/abc123",
			setupDB: func() {
				db["abc123"] = "https://example.com"
			},
			expectedStatus: http.StatusTemporaryRedirect,
			expectedHeader: map[string]string{
				"Location": "https://example.com",
			},
			expectedBody: "",
		},
		{
			name:           "id_not_found",
			method:         http.MethodGet,
			path:           "/notexists",
			setupDB:        func() {},
			expectedStatus: http.StatusNotFound,
			expectedHeader: nil,
			expectedBody:   "URL not found\n",
		},
		{
			name:           "empty_id",
			method:         http.MethodGet,
			path:           "/",
			setupDB:        func() {},
			expectedStatus: http.StatusBadRequest,
			expectedHeader: nil,
			expectedBody:   "Invalid id\n",
		},
		{
			name:           "id_with_slash",
			method:         http.MethodGet,
			path:           "/abc/def",
			setupDB:        func() {},
			expectedStatus: http.StatusBadRequest,
			expectedHeader: nil,
			expectedBody:   "Invalid id\n",
		},
		{
			name:           "method_not_allowed_post",
			method:         http.MethodPost,
			path:           "/abc123",
			setupDB:        func() {},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedHeader: nil,
			expectedBody:   "Only GET requests are allowed!\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db = make(map[string]string)
			if tt.setupDB != nil {
				tt.setupDB()
			}

			req := httptest.NewRequest(tt.method, "http://localhost:8080"+tt.path, nil)
			w := httptest.NewRecorder()

			GetLink(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "статус ответа не совпадает")

			for key, expectedVal := range tt.expectedHeader {
				actual := resp.Header.Get(key)
				assert.Equal(t, expectedVal, actual, "заголовок %q не совпадает", key)
			}

			if tt.expectedBody != "" {
				bodyBytes, err := io.ReadAll(resp.Body)
				require.NoError(t, err, "не удалось прочитать тело ответа")
				assert.Equal(t, tt.expectedBody, string(bodyBytes), "тело ответа не совпадает")
			} else {
				bodyBytes, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.Empty(t, string(bodyBytes), "при редиректе тело должно быть пустым")
			}
		})
	}
}
