package storage

import (
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
	if err := s.Load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

// Save сохраняет ссылку по короткому идентификатору.
func (j *JSONStorage) Save(id string, url string) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	// Удаляем старую связь, если id уже существует
	if oldLink, ok := j.data[id]; ok {
		delete(j.urlMap, oldLink.OriginalUrl)
	}

	linkUuid := uuid.New().String()
	link := models.StorageLink{
		Uuid:        linkUuid,
		ShortUrl:    id,
		OriginalUrl: url,
	}
	j.data[id] = link
	j.urlMap[url] = id

	return j.saveToFile()
}

// Get возвращает оригинальный URL по короткому идентификатору.
func (j *JSONStorage) Get(id string) (string, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	link, ok := j.data[id]
	if !ok {
		return "", ErrNotFound
	}
	return link.OriginalUrl, nil
}

// Exists проверяет наличие записи по короткому идентификатору.
func (j *JSONStorage) Exists(id string) bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	_, ok := j.data[id]
	return ok
}

// FindIDByURL ищет короткий идентификатор по оригинальному URL.
func (j *JSONStorage) FindIDByURL(url string) (string, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	id, ok := j.urlMap[url]
	return id, ok
}

// Load загружает данные из JSON-файла в память.
// Поддерживает как новый формат (массив StorageLink), так и старый (map[string]string).
// При обнаружении старого формата выполняет миграцию и перезаписывает файл.
func (j *JSONStorage) Load() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	file, err := os.Open(j.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Пытаемся декодировать как массив (новый формат)
	var links []models.StorageLink
	dec := json.NewDecoder(file)
	err = dec.Decode(&links)
	if err == nil {
		// Новый формат – заполняем карты
		j.data = make(map[string]models.StorageLink)
		j.urlMap = make(map[string]string)
		for _, link := range links {
			j.data[link.ShortUrl] = link
			j.urlMap[link.OriginalUrl] = link.ShortUrl
		}
		return nil
	}

	// Если не массив, пробуем прочитать как map (старый формат)
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	var oldData map[string]string
	dec = json.NewDecoder(file)
	err = dec.Decode(&oldData)
	if err == nil {
		// Преобразуем старый формат в новый
		j.data = make(map[string]models.StorageLink)
		j.urlMap = make(map[string]string)
		for id, url := range oldData {
			link := models.StorageLink{
				Uuid:        uuid.New().String(), // генерируем новый UUID
				ShortUrl:    id,
				OriginalUrl: url,
			}
			j.data[id] = link
			j.urlMap[url] = id
		}
		// Перезаписываем файл в новом формате
		return j.saveToFile()
	}

	// Если файл пустой или содержит только EOF, считаем, что данных нет
	if err == io.EOF {
		return nil
	}
	return err
}

// saveToFile сохраняет текущие данные в файл (вызывается только при захваченной блокировке).
func (j *JSONStorage) saveToFile() error {
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

// SaveToFile — публичный метод для принудительного сохранения (с блокировкой на запись).
func (j *JSONStorage) SaveToFile() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.saveToFile()
}
