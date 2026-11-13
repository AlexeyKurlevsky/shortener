package core

import (
	"context"
	"testing"

	"github.com/AlexeyKurlevsky/shortener/internal/models"
	storage_mocks "github.com/AlexeyKurlevsky/shortener/internal/storage/mocks"
)

// BenchmarkGenerateID замеряет скорость генерации короткого идентификатора.
func BenchmarkGenerateID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GenerateID()
	}
}

// BenchmarkIsValidURL проверяет производительность валидации URL на наборе валидных и невалидных строк.
func BenchmarkIsValidURL(b *testing.B) {
	validURLs := []string{
		"http://example.com",
		"https://example.com/path",
		"http://localhost:8080",
	}
	invalidURLs := []string{
		"example.com",
		"ftp://example.com",
		"http://",
		"https://",
		"",
		"just a string",
	}
	all := append(validURLs, invalidURLs...)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsValidURL(all[i%len(all)])
	}
}

// BenchmarkNormalizeURL замеряет нормализацию URL (обрезка пробелов и завершающего слэша).
func BenchmarkNormalizeURL(b *testing.B) {
	urls := []string{
		"  http://example.com/  ",
		"https://example.com/path/",
		"http://example.com",
		"  https://example.com/  ",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NormalizeURL(urls[i%len(urls)])
	}
}

// BenchmarkHandleShorten замеряет полный цикл сокращения одного URL (без дубликатов).
func BenchmarkHandleShorten(b *testing.B) {
	ctx := context.Background()
	store := &storage_mocks.MockStorage{}
	userID := "user123"
	url := "https://example.com/very/long/path"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ShortenURL(ctx, url, userID, store)
	}
}

// BenchmarkPrepareBatchItems замеряет подготовку элементов для батчевого сокращения.
func BenchmarkPrepareBatchItems(b *testing.B) {
	ctx := context.Background()
	store := &storage_mocks.MockStorage{}
	userID := "user123"
	items := []models.BatchRequestItem{
		{CorrelationID: "1", OriginalURL: "https://example.com/1"},
		{CorrelationID: "2", OriginalURL: "https://example.com/2"},
		{CorrelationID: "3", OriginalURL: "https://example.com/3"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = PrepareBatchItems(ctx, items, store, userID)
	}
}

// BenchmarkBuildBatchResponse замеряет формирование ответа для батча.
func BenchmarkBuildBatchResponse(b *testing.B) {
	items := []models.BatchRequestItem{
		{CorrelationID: "1", OriginalURL: "https://example.com/1"},
		{CorrelationID: "2", OriginalURL: "https://example.com/2"},
	}
	urlMap := map[string]string{
		"https://example.com/1": "abc123",
		"https://example.com/2": "def456",
	}
	baseURL := "http://short.url"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildBatchResponse(items, urlMap, baseURL)
	}
}
