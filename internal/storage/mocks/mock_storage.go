// Package mocks содержит мок-реализации интерфейса storage.Storage
// для использования в тестах.
package mocks

import (
	"context"

	"github.com/AlexeyKurlevsky/shortener/internal/storage"
)

// MockStorage — конфигурируемая реализация storage.Storage.
//
// Поведение каждого метода задаётся соответствующей функцией.
// Если функция не задана, метод ведёт себя как «нулевая» реализация:
// Save/BatchSave/DeleteURLs/Load/SaveToFile — без ошибки,
// FindIDByURL — (\"\", false), Exists — false, Get — storage.ErrNotFound,
// GetAllByUser — (nil, nil), CountURLs/CountUsers — (0, nil).
type MockStorage struct {
	FindIDByURLFunc  func(ctx context.Context, url string) (string, bool)
	ExistsFunc       func(ctx context.Context, id string) bool
	SaveFunc         func(ctx context.Context, id, url, userID string) error
	GetFunc          func(ctx context.Context, id string) (string, error)
	LoadFunc         func(ctx context.Context) error
	SaveToFileFunc   func(ctx context.Context) error
	BatchSaveFunc    func(ctx context.Context, items []storage.BatchItem, userID string) error
	GetAllByUserFunc func(ctx context.Context, userID string) ([]storage.URLPair, error)
	DeleteURLsFunc   func(ctx context.Context, ids []string, userID string) error
	CountURLsFunc    func(ctx context.Context) (int, error)
	CountUsersFunc   func(ctx context.Context) (int, error)
	StatsFunc        func(ctx context.Context) (storage.Stats, error)
}

func (m *MockStorage) FindIDByURL(ctx context.Context, url string) (string, bool) {
	if m.FindIDByURLFunc != nil {
		return m.FindIDByURLFunc(ctx, url)
	}
	return "", false
}

func (m *MockStorage) Exists(ctx context.Context, id string) bool {
	if m.ExistsFunc != nil {
		return m.ExistsFunc(ctx, id)
	}
	return false
}

func (m *MockStorage) Save(ctx context.Context, id, url, userID string) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, id, url, userID)
	}
	return nil
}

func (m *MockStorage) Get(ctx context.Context, id string) (string, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, id)
	}
	return "", storage.ErrNotFound
}

func (m *MockStorage) Load(ctx context.Context) error {
	if m.LoadFunc != nil {
		return m.LoadFunc(ctx)
	}
	return nil
}

func (m *MockStorage) SaveToFile(ctx context.Context) error {
	if m.SaveToFileFunc != nil {
		return m.SaveToFileFunc(ctx)
	}
	return nil
}

func (m *MockStorage) BatchSave(ctx context.Context, items []storage.BatchItem, userID string) error {
	if m.BatchSaveFunc != nil {
		return m.BatchSaveFunc(ctx, items, userID)
	}
	return nil
}

func (m *MockStorage) GetAllByUser(ctx context.Context, userID string) ([]storage.URLPair, error) {
	if m.GetAllByUserFunc != nil {
		return m.GetAllByUserFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockStorage) DeleteURLs(ctx context.Context, ids []string, userID string) error {
	if m.DeleteURLsFunc != nil {
		return m.DeleteURLsFunc(ctx, ids, userID)
	}
	return nil
}

func (m *MockStorage) CountURLs(ctx context.Context) (int, error) {
	if m.CountURLsFunc != nil {
		return m.CountURLsFunc(ctx)
	}
	return 0, nil
}

func (m *MockStorage) CountUsers(ctx context.Context) (int, error) {
	if m.CountUsersFunc != nil {
		return m.CountUsersFunc(ctx)
	}
	return 0, nil
}

func (m *MockStorage) Stats(ctx context.Context) (storage.Stats, error) {
	if m.StatsFunc != nil {
		return m.StatsFunc(ctx)
	}
	return storage.Stats{}, nil
}

// Проверка на этапе компиляции, что MockStorage реализует интерфейс.
var _ storage.Storage = (*MockStorage)(nil)
