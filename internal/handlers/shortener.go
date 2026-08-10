package handlers

import (
	"context"
	"math/rand"
	"net/url"
	"strings"

	"github.com/AlexeyKurlevsky/shortener/internal/models"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
)

// generateID генерирует случайный короткий идентификатор
func generateID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 8
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// IsValidURL проверяет корректность URL (схема http/https)
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

// normalizeURL убирает пробелы и завершающий слэш
func normalizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	return strings.TrimSuffix(trimmed, "/")
}

// handleShorten содержит основную логику сокращения одного URL
func handleShorten(ctx context.Context, url string, store storage.Storage) (models.ShortenLink, error) {
	var result models.ShortenLink

	if !IsValidURL(url) {
		return result, newInvalidURLError()
	}
	url = normalizeURL(url)

	// Проверяем, существует ли URL
	if shortURL, ok := store.FindIDByURL(ctx, url); ok {
		return result, &DuplicateURLError{
			ExistingID:  shortURL,
			OriginalURL: url,
		}
	}

	// Генерируем новый ID
	var shortURL string
	for {
		shortURL = generateID()
		if !store.Exists(ctx, shortURL) {
			break
		}
	}
	if err := store.Save(ctx, shortURL, url); err != nil {
		return result, newStorageSaveError()
	}

	result.OriginalUrl = url
	result.ShortUrl = shortURL
	result.IsNew = true
	return result, nil
}

func prepareBatchItems(ctx context.Context, items []models.BatchRequestItem, store storage.Storage) (map[string]string, []storage.BatchItem, error) {
	urlMap := make(map[string]string)
	newItems := make([]storage.BatchItem, 0)

	for _, item := range items {
		if !IsValidURL(item.OriginalURL) {
			return nil, nil, newInvalidURLError()
		}
		norm := normalizeURL(item.OriginalURL)
		if _, ok := urlMap[norm]; ok {
			continue // дубликат в пределах батча
		}
		// Проверяем в хранилище
		if id, ok := store.FindIDByURL(ctx, norm); ok {
			urlMap[norm] = id
		} else {
			// Генерируем новый ID
			var newID string
			for {
				newID = generateID()
				if !store.Exists(ctx, newID) {
					break
				}
			}
			urlMap[norm] = newID
			newItems = append(newItems, storage.BatchItem{ID: newID, URL: norm})
		}
	}
	return urlMap, newItems, nil
}

func buildBatchResponse(items []models.BatchRequestItem, urlMap map[string]string, baseURL string) []models.BatchResponseItem {
	respItems := make([]models.BatchResponseItem, len(items))
	for i, item := range items {
		norm := normalizeURL(item.OriginalURL)
		id := urlMap[norm]
		fullURL := baseURL + "/" + id
		respItems[i] = models.BatchResponseItem{
			CorrelationID: item.CorrelationID,
			ShortURL:      fullURL,
		}
	}
	return respItems
}
