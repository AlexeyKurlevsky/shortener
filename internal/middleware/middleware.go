package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/AlexeyKurlevsky/shortener/internal/logger"
	"go.uber.org/zap"
)

func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content_type := r.Header.Get("Content-Type")
		ow := w

		if content_type == "application/json" || content_type == "text/html" || content_type == "text/plain" {

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

		rw := &logger.MyResponseWriter{
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
