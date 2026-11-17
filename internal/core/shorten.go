package core

import (
	"context"

	"github.com/AlexeyKurlevsky/shortener/internal/models"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
)

// ShortenURL — общая бизнес-логика создания короткой ссылки.
//
// Последовательность:
//  1. Валидирует URL через IsValidURL; при ошибке возвращает models.AppError (400).
//  2. Нормализует URL.
//  3. Если URL уже есть в хранилище — возвращает *models.DuplicateURLError
//     с существующим ID.
//  4. Генерирует уникальный ID, сохраняет пару.
//  5. Разрешает гонку: PostgresStorage.Save использует
//     `ON CONFLICT (original_url) DO NOTHING`, поэтому при параллельной
//     вставке Save молча ничего не вставит. Повторный FindIDByURL вернёт
//     чужой ID — в этом случае возвращаем *models.DuplicateURLError.
//
// Возвращает models.ShortenLink с IsNew = true при успехе.
func ShortenURL(
	ctx context.Context,
	originalURL string,
	userID string,
	stor storage.Storage,
) (models.ShortenLink, error) {
	var result models.ShortenLink

	if !IsValidURL(originalURL) {
		return result, newInvalidURLError()
	}
	originalURL = NormalizeURL(originalURL)

	if existingID, ok := stor.FindIDByURL(ctx, originalURL); ok {
		return result, &models.DuplicateURLError{
			ExistingID:  existingID,
			OriginalURL: originalURL,
		}
	}

	shortID := generateUniqueID(ctx, stor)
	if err := stor.Save(ctx, shortID, originalURL, userID); err != nil {
		return result, newStorageSaveError()
	}

	// Разрешаем гонку с параллельным запросом, который успел вставить
	// тот же original_url между FindIDByURL и Save.
	if actualID, ok := stor.FindIDByURL(ctx, originalURL); ok && actualID != shortID {
		return result, &models.DuplicateURLError{
			ExistingID:  actualID,
			OriginalURL: originalURL,
		}
	}

	result.OriginalURL = originalURL
	result.ShortURL = shortID
	result.IsNew = true
	return result, nil
}

// ExpandURL — общая бизнес-логика раскрытия короткого идентификатора.
// Возвращает storage.ErrNotFound / storage.ErrGone или другую ошибку storage.
func ExpandURL(ctx context.Context, id string, stor storage.Storage) (string, error) {
	return stor.Get(ctx, id)
}

// ListUserURLs — общая бизнес-логика получения всех ссылок пользователя.
// Возвращает storage.URLPair; подстановку baseURL и сериализацию
// в конкретный формат (JSON/proto) делают фасады.
func ListUserURLs(ctx context.Context, userID string, stor storage.Storage) ([]storage.URLPair, error) {
	return stor.GetAllByUser(ctx, userID)
}
