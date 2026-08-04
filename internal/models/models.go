package models

type CreateUrlRequest struct {
	Url string `json:"url"`
}

type ShortUrlResponse struct {
	Result string `json:"result"`
}
