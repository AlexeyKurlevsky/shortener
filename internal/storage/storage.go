package storage

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("url not found")

type Storage interface {
	Save(ctx context.Context, id string, url string) error
	Get(ctx context.Context, id string) (string, error)
	Exists(ctx context.Context, id string) bool
	FindIDByURL(ctx context.Context, url string) (string, bool)
	Load(ctx context.Context) error
	SaveToFile(ctx context.Context) error
	BatchSave(ctx context.Context, items []BatchItem) error
}

type BatchItem struct {
	ID  string
	URL string
}
