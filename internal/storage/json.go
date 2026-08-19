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

type JSONStorage struct {
	filePath string
	mu       sync.RWMutex
	data     map[string]models.StorageLink
	urlMap   map[string]string
}

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

func (j *JSONStorage) Save(ctx context.Context, id, url, userID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	// Если запись с таким id уже существует, удаляем старую из urlMap
	if oldLink, ok := j.data[id]; ok {
		delete(j.urlMap, oldLink.OriginalUrl)
	}
	link := models.StorageLink{
		Uuid: uuid.New().String(),
		ShortenLink: models.ShortenLink{
			ShortUrl:    id,
			OriginalUrl: url,
		},
		UserID:    userID,
		IsDeleted: false, // новая запись активна
	}
	j.data[id] = link
	j.urlMap[url] = id
	return j.saveToFile(ctx)
}

func (j *JSONStorage) Get(ctx context.Context, id string) (string, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	link, ok := j.data[id]
	if !ok {
		return "", ErrNotFound
	}
	if link.IsDeleted {
		return "", ErrGone
	}
	return link.OriginalUrl, nil
}

func (j *JSONStorage) Exists(ctx context.Context, id string) bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	_, ok := j.data[id]
	return ok
}

// FindIDByURL возвращает ID только для активных (не удалённых) записей
func (j *JSONStorage) FindIDByURL(ctx context.Context, url string) (string, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	id, ok := j.urlMap[url]
	if !ok {
		return "", false
	}
	link, ok := j.data[id]
	if !ok || link.IsDeleted {
		return "", false
	}
	return id, true
}

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
			// Добавляем в urlMap только активные записи (для поиска)
			if !link.IsDeleted {
				j.urlMap[link.OriginalUrl] = link.ShortUrl
			}
		}
		return nil
	}
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

func (j *JSONStorage) BatchSave(ctx context.Context, items []BatchItem, userID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, item := range items {
		link := models.StorageLink{
			Uuid: uuid.New().String(),
			ShortenLink: models.ShortenLink{
				ShortUrl:    item.ID,
				OriginalUrl: item.URL,
			},
			UserID:    userID,
			IsDeleted: false,
		}
		j.data[item.ID] = link
		j.urlMap[item.URL] = item.ID
	}
	return j.saveToFile(ctx)
}

func (j *JSONStorage) GetAllByUser(ctx context.Context, userID string) ([]URLPair, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var pairs []URLPair
	for _, link := range j.data {
		if link.UserID == userID && !link.IsDeleted {
			pairs = append(pairs, URLPair{
				ShortURL:    link.ShortUrl,
				OriginalURL: link.OriginalUrl,
			})
		}
	}
	return pairs, nil
}

// DeleteURLs помечает записи как удалённые (если они принадлежат пользователю)
func (j *JSONStorage) DeleteURLs(ctx context.Context, ids []string, userID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, id := range ids {
		if link, ok := j.data[id]; ok && link.UserID == userID {
			link.IsDeleted = true
			j.data[id] = link
			// Удаляем из urlMap, чтобы FindIDByURL больше не находил этот URL
			delete(j.urlMap, link.OriginalUrl)
		}
	}
	return j.saveToFile(ctx)
}
