package core

import (
	"context"

	"github.com/AlexeyKurlevsky/shortener/internal/models"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
)

// core.PrepareBatchItems подготавливает данные для пакетного создания коротких
// ссылок: валидирует и нормализует URL, отбрасывает дубликаты внутри батча,
// переиспользует существующие ID и генерирует новые для отсутствующих.
//
// Возвращает:
//   - urlMap  — нормализованный URL → короткий ID (для ответа);
//   - newItems — элементы, которых ещё нет в хранилище (для storage.BatchSave);
//   - ошибку — models.AppError (400) при невалидном URL.
//
// userID оставлен для симметрии с storage.BatchSave; текущая реализация
// его не использует.
func PrepareBatchItems(
	ctx context.Context,
	items []models.BatchRequestItem,
	stor storage.Storage,
	userID string,
) (map[string]string, []storage.BatchItem, error) {
	_ = userID

	urlMap := make(map[string]string)
	newItems := make([]storage.BatchItem, 0, len(items))

	for _, item := range items {
		if !IsValidURL(item.OriginalURL) {
			return nil, nil, newInvalidURLError()
		}
		norm := NormalizeURL(item.OriginalURL)
		if _, ok := urlMap[norm]; ok {
			continue // дубликат в пределах батча
		}
		if id, ok := stor.FindIDByURL(ctx, norm); ok {
			urlMap[norm] = id
			continue
		}
		newID := generateUniqueID(ctx, stor)
		urlMap[norm] = newID
		newItems = append(newItems, storage.BatchItem{ID: newID, URL: norm})
	}
	return urlMap, newItems, nil
}

// core.BuildBatchResponse формирует ответ на пакетный запрос, сохраняя порядок
// входных элементов и их CorrelationID.
func BuildBatchResponse(
	items []models.BatchRequestItem,
	urlMap map[string]string,
	baseURL string,
) []models.BatchResponseItem {
	respItems := make([]models.BatchResponseItem, len(items))
	for i, item := range items {
		norm := NormalizeURL(item.OriginalURL)
		id := urlMap[norm]
		respItems[i] = models.BatchResponseItem{
			CorrelationID: item.CorrelationID,
			ShortURL:      baseURL + "/" + id,
		}
	}
	return respItems
}
