package handlers

import (
	"context"
	"sync"
	"time"

	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/AlexeyKurlevsky/shortener/internal/logger"
	"github.com/AlexeyKurlevsky/shortener/internal/storage"
	"go.uber.org/zap"
)

type deleteTask struct {
	UserID string
	ID     string
}

type Handler struct {
	storage       storage.Storage
	cfg           *config.Config
	db            Pinger
	deleteChan    chan deleteTask
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	batchSize     int
	flushInterval time.Duration
}

func NewHandler(storage storage.Storage, cfg *config.Config, db Pinger) *Handler {
	ctx, cancel := context.WithCancel(context.Background())
	h := &Handler{
		storage:       storage,
		cfg:           cfg,
		db:            db,
		deleteChan:    make(chan deleteTask, 1000),
		ctx:           ctx,
		cancel:        cancel,
		batchSize:     100,             // размер батча
		flushInterval: 5 * time.Second, // интервал принудительного сброса
	}
	h.wg.Add(1)
	go h.deleteWorker()
	return h
}

// Close останавливает воркер и ждёт его завершения
func (h *Handler) Close() {
	h.cancel()
	close(h.deleteChan)
	h.wg.Wait()
}

// deleteWorker – воркер, реализующий паттерн fan‑in
func (h *Handler) deleteWorker() {
	defer h.wg.Done()
	ticker := time.NewTicker(h.flushInterval)
	defer ticker.Stop()

	batch := make([]deleteTask, 0, h.batchSize*2)

	for {
		select {
		case task, ok := <-h.deleteChan:
			if !ok {
				// канал закрыт – сбрасываем оставшиеся задачи
				h.flushBatch(batch)
				return
			}
			batch = append(batch, task)
			if len(batch) >= h.batchSize {
				h.flushBatch(batch)
				batch = make([]deleteTask, 0, h.batchSize*2)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				h.flushBatch(batch)
				batch = make([]deleteTask, 0, h.batchSize*2)
			}

		case <-h.ctx.Done():
			// сигнал завершения – сбрасываем оставшиеся задачи
			h.flushBatch(batch)
			return
		}
	}
}

// flushBatch группирует задачи по UserID и вызывает DeleteURLs для каждой группы
func (h *Handler) flushBatch(tasks []deleteTask) {
	if len(tasks) == 0 {
		return
	}
	groups := make(map[string][]string)
	for _, t := range tasks {
		groups[t.UserID] = append(groups[t.UserID], t.ID)
	}
	for userID, ids := range groups {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := h.storage.DeleteURLs(ctx, ids, userID); err != nil {
			logger.Log.Error("failed to delete URLs", zap.Error(err), zap.String("userID", userID), zap.Strings("ids", ids))
		}
		cancel()
	}
}
