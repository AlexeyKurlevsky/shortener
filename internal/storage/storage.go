package storage

import "errors"

var ErrNotFound = errors.New("url not found")

type Storage interface {
	Save(id string, url string) error
	Get(id string) (string, error)
	Exists(id string) bool
	FindIDByURL(url string) (string, bool)
	Load() error
	SaveToFile() error
}
