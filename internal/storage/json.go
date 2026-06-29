package storage

import (
	"encoding/json"
	"io"
	"os"
	"sync"
)

type JSONStorage struct {
	filePath string
	mu       sync.RWMutex
	data     map[string]string // id -> url
	urlMap   map[string]string // url -> id
}

func NewJSONStorage(filePath string) (*JSONStorage, error) {
	s := &JSONStorage{
		filePath: filePath,
		data:     make(map[string]string),
		urlMap:   make(map[string]string),
	}
	if err := s.Load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (j *JSONStorage) Save(id string, url string) error {
	j.mu.Lock()
	j.data[id] = url
	j.urlMap[url] = id
	j.mu.Unlock()
	return j.SaveToFile()
}

func (j *JSONStorage) Get(id string) (string, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if val, ok := j.data[id]; ok {
		return val, nil
	}
	return "", ErrNotFound
}

func (j *JSONStorage) Exists(id string) bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	_, ok := j.data[id]
	return ok
}

func (j *JSONStorage) FindIDByURL(url string) (string, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	id, ok := j.urlMap[url]
	return id, ok
}

func (j *JSONStorage) Load() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	file, err := os.Open(j.filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	err = dec.Decode(&j.data)
	if err != nil && err != io.EOF {
		return err
	}
	j.urlMap = make(map[string]string)
	for id, url := range j.data {
		j.urlMap[url] = id
	}
	return nil
}

func (j *JSONStorage) SaveToFile() error {
	j.mu.RLock()
	defer j.mu.RUnlock()
	file, err := os.Create(j.filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(j.data)
}
