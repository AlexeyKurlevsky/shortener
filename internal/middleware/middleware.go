// Package middleware содержит HTTP-middleware: аутентификацию, gzip
// и общие вспомогательные обёртки.
package middleware

import (
	"net"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/AlexeyKurlevsky/shortener/internal/logger"
)

func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		ow := w

		if contentType == "application/json" || contentType == "text/html" || contentType == "text/plain" {

			acceptEncoding := r.Header.Get("Accept-Encoding")
			supportsGzip := strings.Contains(acceptEncoding, "gzip")
			if supportsGzip {
				cw := newCompressWriter(w)
				ow = cw
				defer cw.Close()
			}

			contentEncoding := r.Header.Get("Content-Encoding")
			sendsGzip := strings.Contains(contentEncoding, "gzip")
			if sendsGzip {
				cr, err := newCompressReader(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				r.Body = cr
				defer cr.Close()
			}

		}

		next.ServeHTTP(ow, r)
	})
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rw := &logger.ResponseRecorder{
			ResponseWriter: w,
			Status:         http.StatusOK,
		}

		next.ServeHTTP(rw, r)

		// Log all required details.
		logger.Log.Info("HTTP request",
			zap.String("method", r.Method),
			zap.String("uri", r.URL.RequestURI()),
			zap.Duration("duration", time.Since(start)),
			zap.Int("status", rw.Status),
			zap.Int("response_size", rw.Size),
		)
	})
}

// TrustedSubnetMiddleware разрешает запрос, только если X-Real-IP входит
// в доверенную подсеть. Пустой cidr => доступ запрещён для всех.
func TrustedSubnetMiddleware(subnet *net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if subnet == nil { // пустой или невалидный trusted_subnet
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP")))
			if ip == nil || !subnet.Contains(ip) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
