package storage

import "sync"

type MemoryStorage struct {
	mu     sync.RWMutex
	data   map[string]string
	urlMap map[string]string
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		data:   make(map[string]string),
		urlMap: make(map[string]string),
	}
}

func (m *MemoryStorage) Save(id string, url string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[id] = url
	m.urlMap[url] = id
	return nil
}

func (m *MemoryStorage) Get(id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if val, ok := m.data[id]; ok {
		return val, nil
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
