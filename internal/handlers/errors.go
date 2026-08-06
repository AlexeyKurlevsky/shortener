package handlers

import (
	"errors"
	"net/http"

	"github.com/AlexeyKurlevsky/shortener/internal/models"
)

func newInvalidURLError() error {
	return models.AppError{Status: http.StatusBadRequest, Err: errors.New("invalid URL")}
}

func newStorageSaveError() error {
	return models.AppError{Status: http.StatusInternalServerError, Err: errors.New("can't save link in storage")}
}

type DuplicateURLError struct {
	ExistingID  string
	OriginalURL string
}

func (e DuplicateURLError) Error() string {
	return "URL already exists"
}
