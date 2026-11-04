// Package tlsconfig для настройки TLS
package tlsconfig

import (
	"crypto/tls"
	"net/http"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// NewManager создаёт autocert.Manager для списка доменов.
// cacheDir — директория для хранения полученных сертификатов
// (например, "certs"); email — контакт для Let's Encrypt.
func NewManager(domains []string, email, cacheDir string) *autocert.Manager {
	return &autocert.Manager{
		Cache:      autocert.DirCache(cacheDir),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(domains...),
		Email:      email,
	}
}

// NewTLSConfig оборачивает менеджер в *tls.Config, который можно
// положить в http.Server.TLSConfig перед ListenAndServeTLS("", "").
func NewTLSConfig(m *autocert.Manager) *tls.Config {
	return &tls.Config{
		GetCertificate: m.GetCertificate,
		NextProtos:     []string{"h2", "http/1.1", acme.ALPNProto},
		MinVersion:     tls.VersionTLS12,
	}
}

// NewHTTPHandler возвращает хендлер для порта :80 — он обслуживает
// ACME HTTP-01 челленджи и редиректит остальной трафик на HTTPS.
// Если redirectTo == "" — редирект на тот же хост по https.
func NewHTTPHandler(m *autocert.Manager, redirectTo string) http.Handler {
	return m.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + r.Host + r.URL.RequestURI()
		if redirectTo != "" {
			target = redirectTo
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}))
}
