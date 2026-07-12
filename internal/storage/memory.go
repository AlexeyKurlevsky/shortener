package storage

import (
	"sync"

	"github.com/AlexeyKurlevsky/shortener/internal/models"
)

type MemoryStorage struct {
	mu     sync.RWMutex
	data   map[string]models.StorageLink // ключ: shortUrl
	urlMap map[string]string             // ключ: originalUrl -> shortUrl
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		data:   make(map[string]models.StorageLink),
		urlMap: make(map[string]string),
	}
}

func (m *MemoryStorage) Save(id string, url string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Если id уже существует, удаляем старую связь из urlMap
	if oldLink, ok := m.data[id]; ok {
		delete(m.urlMap, oldLink.OriginalUrl)
	}

	link := models.StorageLink{
		Uuid:        "", // можно оставить пустым или сгенерировать при необходимости
		ShortUrl:    id,
		OriginalUrl: url,
	}
	m.data[id] = link
	m.urlMap[url] = id

	return nil
}

func (m *MemoryStorage) Get(id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if link, ok := m.data[id]; ok {
		return link.OriginalUrl, nil
	}
	return "", ErrNotFound
}

func (m *MemoryStorage) Exists(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.data[id]
	return ok
}

func (m *MemoryStorage) FindIDByURL(url string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.urlMap[url]
	return id, ok
}

func (m *MemoryStorage) Load() error       { return nil }
func (m *MemoryStorage) SaveToFile() error { return nil }
