package storage

import (
	"context"
	"sync"

	"github.com/AlexeyKurlevsky/shortener/internal/models"
	"github.com/google/uuid"
)

type MemoryStorage struct {
	mu     sync.RWMutex
	data   map[string]models.StorageLink // key: shortUrl
	urlMap map[string]string             // key: originalUrl -> shortUrl
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		data:   make(map[string]models.StorageLink),
		urlMap: make(map[string]string),
	}
}

func (m *MemoryStorage) Save(ctx context.Context, id, url, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	link := models.StorageLink{
		Uuid: uuid.New().String(),
		ShortenLink: models.ShortenLink{
			ShortUrl:    id,
			OriginalUrl: url,
		},
		UserID: userID,
	}
	m.data[id] = link
	m.urlMap[url] = id
	return nil
}

func (m *MemoryStorage) Get(ctx context.Context, id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	link, ok := m.data[id]
	if !ok {
		return "", ErrNotFound
	}
	return link.OriginalUrl, nil
}

func (m *MemoryStorage) Exists(ctx context.Context, id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.data[id]
	return ok
}

func (m *MemoryStorage) FindIDByURL(ctx context.Context, url string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.urlMap[url]
	return id, ok
}

func (m *MemoryStorage) Load(ctx context.Context) error {
	return nil
}

func (m *MemoryStorage) SaveToFile(ctx context.Context) error {
	return nil
}

func (m *MemoryStorage) BatchSave(ctx context.Context, items []BatchItem, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range items {
		link := models.StorageLink{
			Uuid: uuid.New().String(),
			ShortenLink: models.ShortenLink{
				ShortUrl:    item.ID,
				OriginalUrl: item.URL,
			},
			UserID: userID,
		}
		m.data[item.ID] = link
		m.urlMap[item.URL] = item.ID
	}
	return nil
}

func (m *MemoryStorage) GetAllByUser(ctx context.Context, userID string) ([]URLPair, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var pairs []URLPair
	for _, link := range m.data {
		if link.UserID == userID {
			pairs = append(pairs, URLPair{
				ShortURL:    link.ShortUrl,
				OriginalURL: link.OriginalUrl,
			})
		}
	}
	return pairs, nil
}
