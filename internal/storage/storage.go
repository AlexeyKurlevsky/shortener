package storage

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("url not found")
	ErrGone     = errors.New("url is deleted")
)

type URLPair struct {
	ShortURL    string
	OriginalURL string
}

type BatchItem struct {
	ID  string
	URL string
}

// Stats — согласованный снимок счётчиков сервиса.
// Оба значения получены на одном снапшоте данных.
type Stats struct {
	URLs  int
	Users int
}

type Storage interface {
	Save(ctx context.Context, id string, url string, userID string) error
	Get(ctx context.Context, id string) (string, error)
	Exists(ctx context.Context, id string) bool
	FindIDByURL(ctx context.Context, url string) (string, bool)
	Load(ctx context.Context) error
	SaveToFile(ctx context.Context) error
	BatchSave(ctx context.Context, items []BatchItem, userID string) error
	GetAllByUser(ctx context.Context, userID string) ([]URLPair, error)
	DeleteURLs(ctx context.Context, ids []string, userID string) error
	CountURLs(ctx context.Context) (int, error)
	CountUsers(ctx context.Context) (int, error)
	Stats(ctx context.Context) (Stats, error)
}
