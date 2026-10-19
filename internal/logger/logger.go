// Package logger инициализирует zap-логгер приложения и предоставляет
// функции структурированного логирования.
package logger

import (
	"errors"
	"net/http"
	"sync"
	"syscall"

	"go.uber.org/zap"
)

var (
	Log  *zap.Logger = zap.NewNop()
	once sync.Once
)

// Initialize создаёт логгер один раз. Повторные вызовы игнорируются.
func Initialize(level string) error {
	var initErr error
	once.Do(func() {
		lvl, err := zap.ParseAtomicLevel(level)
		if err != nil {
			initErr = err
			return
		}
		cfg := zap.NewProductionConfig()
		cfg.Level = lvl
		cfg.Sampling = nil // убирает zapcore.newCounters из профиля

		zl, err := cfg.Build()
		if err != nil {
			initErr = err
			return
		}
		Log = zl
		zap.ReplaceGlobals(zl) // чтобы zap.L() тоже работал
	})
	return initErr
}

// Sync сбрасывает буферы. Вызывайте при shutdown.
func Sync() error {
	if Log == nil {
		return nil
	}
	if err := Log.Sync(); err != nil && !errors.Is(err, syscall.ENOTTY) {
		return err
	}
	return nil
}

type ResponseRecorder struct {
	http.ResponseWriter
	Status int
	Size   int
}

func (rw *ResponseRecorder) WriteHeader(status int) {
	rw.Status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *ResponseRecorder) Write(b []byte) (int, error) {
	if rw.Status == 0 {
		rw.Status = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.Size += n
	return n, err
}

func (rw *ResponseRecorder) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
