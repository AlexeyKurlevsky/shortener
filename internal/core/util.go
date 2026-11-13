package core

import (
	"context"
	"math/rand"
	"net/url"
	"strings"

	"github.com/AlexeyKurlevsky/shortener/internal/storage"
)

// GenerateID генерирует случайный короткий идентификатор длиной 8 символов
// из набора [a-zA-Z0-9]. Уникальность не гарантируется — вызывающая
// сторона должна проверять коллизии через storage.Exists.
func GenerateID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 8
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// generateUniqueID генерирует ID, которого ещё нет в хранилище.
func generateUniqueID(ctx context.Context, stor storage.Storage) string {
	for {
		id := GenerateID()
		if !stor.Exists(ctx, id) {
			return id
		}
	}
}

// IsValidURL проверяет корректность URL: абсолютный URI, схема http/https,
// непустые Scheme и Host.
func IsValidURL(str string) bool {
	u, err := url.ParseRequestURI(str)
	if err != nil {
		return false
	}
	if u.Scheme == "" || u.Host == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return true
}

// NormalizeURL приводит URL к каноническому виду: удаляет пробелы по краям
// и завершающий слэш.
func NormalizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	return strings.TrimSuffix(trimmed, "/")
}
