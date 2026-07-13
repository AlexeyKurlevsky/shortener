package models

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
}

func (s *ShortenLink) GetFullLink(baseURL string) string {
	fullLink := baseURL + "/" + s.ShortUrl
	return fullLink
}

type StorageLink struct {
	Uuid string `json:"uuid"`
	ShortenLink
}
