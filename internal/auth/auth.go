package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

const CookieName = "user_id"

var ErrInvalidCookie = errors.New("invalid cookie")

// GenerateUserID создаёт новый UUID.
func GenerateUserID() string {
	return uuid.New().String()
}

// CreateSignedCookie создаёт куку со значением userID и подписью.
func CreateSignedCookie(userID string, secret []byte) http.Cookie {
	data := base64.URLEncoding.EncodeToString([]byte(userID))
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(data))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	value := data + "." + signature
	return http.Cookie{
		Name:     CookieName,
		Value:    value,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		MaxAge:   86400 * 30, // 30 дней
	}
}

// GetUserIDFromCookie извлекает и проверяет подпись, возвращает userID.
func GetUserIDFromCookie(r *http.Request, secret []byte) (string, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return "", err
	}
	value := cookie.Value
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return "", ErrInvalidCookie
	}
	data, signature := parts[0], parts[1]

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(data))
	expected := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	if signature != expected {
		return "", ErrInvalidCookie
	}

	userIDBytes, err := base64.URLEncoding.DecodeString(data)
	if err != nil {
		return "", ErrInvalidCookie
	}
	userID := string(userIDBytes)
	if userID == "" {
		return "", ErrInvalidCookie
	}
	return userID, nil
}
