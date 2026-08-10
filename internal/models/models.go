package models

import (
	"net/http"
)

type CreateUrlRequest struct {
	Url string `json:"url"`
}

type AppError struct {
	Status int
	Err    error
}

func (e AppError) Error() string {
	return e.Err.Error()
}

type ShortUrlResponse struct {
	Result string `json:"result"`
}

type ShortenLink struct {
	ShortUrl    string `json:"short_url"`
	OriginalUrl string `json:"original_url"`
	IsNew       bool   `json:"-"`
}

func (s *ShortenLink) GetFullLink(baseURL string) string {
	return baseURL + "/" + s.ShortUrl
}

func (s *ShortenLink) GetStatusCode() int {
	if s.IsNew {
		return http.StatusCreated
	}
	return http.StatusOK
}

type StorageLink struct {
	Uuid string `json:"uuid"`
	ShortenLink
	UserID string `json:"user_id"`
}

type BatchRequestItem struct {
	CorrelationID string `json:"correlation_id"`
	OriginalURL   string `json:"original_url"`
}

type BatchResponseItem struct {
	CorrelationID string `json:"correlation_id"`
	ShortURL      string `json:"short_url"`
}

type URLPair struct {
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}

type DuplicateURLError struct {
	ExistingID  string
	OriginalURL string
}

func (e DuplicateURLError) Error() string {
	return "duplicate URL"
}
