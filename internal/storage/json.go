package storage

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"

	"github.com/AlexeyKurlevsky/shortener/internal/models"
	"github.com/google/uuid"
)

// JSONStorage хранит ссылки в файле JSON.
type JSONStorage struct {
	filePath string
	mu       sync.RWMutex
	data     map[string]models.StorageLink // ключ: ShortUrl
	urlMap   map[string]string             // ключ: OriginalUrl -> ShortUrl
}

// NewJSONStorage создаёт хранилище и загружает данные из файла (если он существует).
func NewJSONStorage(filePath string) (*JSONStorage, error) {
	s := &JSONStorage{
		filePath: filePath,
		data:     make(map[string]models.StorageLink),
		urlMap:   make(map[string]string),
	}
	if err := s.Load(context.Background()); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

// Save сохраняет ссылку по короткому идентификатору.
func (j *JSONStorage) Save(ctx context.Context, id, url string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	// Удаляем старую связь, если id уже существует
	if oldLink, ok := j.data[id]; ok {
		delete(j.urlMap, oldLink.OriginalUrl)
	}

	linkUuid := uuid.New().String()
	link := models.StorageLink{
		Uuid: linkUuid,
		ShortenLink: models.ShortenLink{
			ShortUrl:    id,
			OriginalUrl: url,
		},
	}
	j.data[id] = link
	j.urlMap[url] = id

	return j.saveToFile(ctx)
}

// Get возвращает оригинальный URL по короткому идентификатору.
func (j *JSONStorage) Get(ctx context.Context, id string) (string, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	link, ok := j.data[id]
	if !ok {
		return "", ErrNotFound
	}
	return link.OriginalUrl, nil
}

// Exists проверяет наличие записи по короткому идентификатору.
func (j *JSONStorage) Exists(ctx context.Context, id string) bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	_, ok := j.data[id]
	return ok
}

// FindIDByURL ищет короткий идентификатор по оригинальному URL.
func (j *JSONStorage) FindIDByURL(ctx context.Context, url string) (string, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	id, ok := j.urlMap[url]
	return id, ok
}

// Load загружает данные из JSON-файла в память.
func (j *JSONStorage) Load(ctx context.Context) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	file, err := os.Open(j.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var links []models.StorageLink
	dec := json.NewDecoder(file)
	err = dec.Decode(&links)
	if err == nil {
		j.data = make(map[string]models.StorageLink)
		j.urlMap = make(map[string]string)
		for _, link := range links {
			j.data[link.ShortUrl] = link
			j.urlMap[link.OriginalUrl] = link.ShortUrl
		}
		return nil
	}
	// Если файл пустой или содержит только EOF, считаем, что данных нет
	if err == io.EOF {
		return nil
	}
	return err
}

func (j *JSONStorage) saveToFile(ctx context.Context) error {
	links := make([]models.StorageLink, 0, len(j.data))
	for _, link := range j.data {
		links = append(links, link)
	}

	file, err := os.Create(j.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(links)
}

func (j *JSONStorage) SaveToFile(ctx context.Context) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.saveToFile(ctx)
}

func (j *JSONStorage) BatchSave(ctx context.Context, items []BatchItem) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, item := range items {
		link := models.StorageLink{
			Uuid: uuid.New().String(),
			ShortenLink: models.ShortenLink{
				ShortUrl:    item.ID,
				OriginalUrl: item.URL,
			},
		}
		j.data[item.ID] = link
		j.urlMap[item.URL] = item.ID
	}
	return j.saveToFile(ctx)
}
