package models

type CreateUrlRequest struct {
	Url string `json:"url"`
}

type ShortUrlResponse struct {
	Result string `json:"result"`
}

type StorageLink struct {
	Uuid        string `json:"uuid"`
	ShortUrl    string `json:"short_url"`
	OriginalUrl string `json:"original_url"`
}
