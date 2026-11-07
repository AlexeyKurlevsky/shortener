// Package auth реализует аутентификацию пользователей.
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

func GenerateUserID() string {
	return uuid.New().String()
}

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
		MaxAge:   86400 * 30,
	}
}

// ValidateSignedValue проверяет HMAC-подпись значения вида
// "<base64(userID)>.<base64(signature)>" и возвращает userID.
//
// Не привязан к *http.Request — используется и HTTP-middleware
// (через GetUserIDFromCookie), и gRPC-интерсептором, которому
// значение приходит из metadata "authorization".
func ValidateSignedValue(value string, secret []byte) (string, error) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return "", ErrInvalidCookie
	}
	data, signature := parts[0], parts[1]

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(data))
	expected := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(expected)) {
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

// GetUserIDFromCookie извлекает и проверяет подпись, возвращает userID.
func GetUserIDFromCookie(r *http.Request, secret []byte) (string, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return "", err
	}
	return ValidateSignedValue(cookie.Value, secret)
}
