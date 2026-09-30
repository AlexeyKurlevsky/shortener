package handlers

import (
	"context"
	"math/rand"
	"net/url"
	"strings"

	"github.com/AlexeyKurlevsky/shortener/internal/models"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
)

// generateID генерирует случайный короткий идентификатор длиной 8 символов
// из набора [a-zA-Z0-9]. Используется как суффикс короткой ссылки.
//
// Уникальность не гарантируется — вызывающая сторона должна проверять
// отсутствие коллизий через storage.Exists.
func generateID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 8
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// IsValidURL проверяет корректность URL.
//
// Возвращает true, если строка успешно парсится как абсолютный URI,
// содержит непустые Scheme и Host, а схема равна "http" или "https".
// Во всех остальных случаях возвращает false.
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

// normalizeURL приводит URL к каноническому виду: удаляет пробелы
// по краям и завершающий слэш. Используется для сравнения и хранения
// оригинальных ссылок.
func normalizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	return strings.TrimSuffix(trimmed, "/")
}

// handleShorten содержит основную логику сокращения одного URL.
//
// Последовательность действий:
//  1. Проверяет корректность URL через IsValidURL — иначе возвращает
//     ошибку newInvalidURLError.
//  2. Нормализует URL через normalizeURL.
//  3. Если URL уже есть в хранилище, возвращает *DuplicateURLError
//     с существующим ID.
//  4. Иначе генерирует уникальный короткий ID (с проверкой коллизий
//     через storage.Exists) и сохраняет пару через storage.Save.
//
// При успехе возвращает models.ShortenLink с флагом IsNew = true.
// Ошибки сохранения оборачиваются в newStorageSaveError.
func handleShorten(ctx context.Context, url string, store storage.Storage, userID string) (models.ShortenLink, error) {
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
	if err := store.Save(ctx, shortURL, url, userID); err != nil {
		return result, newStorageSaveError()
	}

	result.OriginalURL = url
	result.ShortURL = shortURL
	result.IsNew = true
	return result, nil
}

// prepareBatchItems подготавливает данные для пакетного создания коротких ссылок.
//
// Для каждого элемента входного среза:
//   - проверяет корректность OriginalURL (при ошибке возвращает
//     newInvalidURLError для всего батча);
//   - нормализует URL через normalizeURL;
//   - пропускает дубликаты внутри самого батча;
//   - если URL уже есть в хранилище — переиспользует существующий ID;
//   - иначе генерирует новый уникальный ID и добавляет элемент
//     в список новых для сохранения.
//
// Возвращает:
//   - urlMap — отображение нормализованный URL → короткий ID
//     (для формирования ответа);
//   - newItems — элементы, которых ещё нет в хранилище
//     (для передачи в storage.BatchSave);
//   - ошибку — при невалидном URL во входных данных.
func prepareBatchItems(ctx context.Context, items []models.BatchRequestItem, store storage.Storage, userID string) (map[string]string, []storage.BatchItem, error) {
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

// buildBatchResponse формирует ответ на пакетный запрос создания коротких ссылок.
//
// Для каждого входного элемента находит соответствующий короткий ID
// в urlMap по нормализованному URL и составляет полную короткую ссылку
// вида baseURL + "/" + id, сохраняя CorrelationID исходного запроса.
//
// Порядок элементов в ответе соответствует порядку входного среза items.
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
