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

const (
	defaultWorkers = 5    // количество параллельных воркеров
	taskChanSize   = 1000 // буфер общего канала
	workerChanSize = 100  // буфер каждого канала воркера
)

// DeleteTask – задача на удаление одного URL для конкретного пользователя
type DeleteTask struct {
	UserID string
	ID     string
}

type Handler struct {
	storage storage.Storage
	cfg     *config.Config
	db      Pinger

	// Fan‑In канал – сюда хендлеры отправляют задачи
	taskChan chan DeleteTask

	// Fan‑Out каналы – по одному на каждого воркера
	workerChans []chan DeleteTask

	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	numWorkers int
}

func NewHandler(storage storage.Storage, cfg *config.Config, db Pinger) *Handler {
	ctx, cancel := context.WithCancel(context.Background())

	h := &Handler{
		storage:     storage,
		cfg:         cfg,
		db:          db,
		taskChan:    make(chan DeleteTask, taskChanSize),
		workerChans: make([]chan DeleteTask, defaultWorkers),
		ctx:         ctx,
		cancel:      cancel,
		numWorkers:  defaultWorkers,
	}

	// Создаём каналы для каждого воркера
	for i := 0; i < h.numWorkers; i++ {
		h.workerChans[i] = make(chan DeleteTask, workerChanSize)
	}

	// Запускаем диспетчер (Fan‑Out)
	h.wg.Add(1)
	go h.dispatcher()

	// Запускаем воркеров
	for i := 0; i < h.numWorkers; i++ {
		h.wg.Add(1)
		go h.worker(i, h.workerChans[i])
	}

	return h
}

// Shutdown завершает работу обработчиков, дожидаясь завершения всех горутин
func (h *Handler) Shutdown() {
	// Сигнал остановки всем горутинам
	h.cancel()
	// Закрываем входной канал – диспетчер дочитает оставшиеся задачи и закроет каналы воркеров
	close(h.taskChan)
	// Ждём завершения всех горутин (диспетчер + воркеры)
	h.wg.Wait()
}

// Паттерн FanOut
func (h *Handler) dispatcher() {
	defer h.wg.Done()
	defer func() {
		// Когда диспетчер завершается, закрываем все каналы воркеров
		for _, ch := range h.workerChans {
			close(ch)
		}
	}()

	workerIndex := 0
	for task := range h.taskChan {
		// Выбираем следующего воркера по кругу
		workerIdx := workerIndex % h.numWorkers
		workerIndex++

		select {
		case <-h.ctx.Done():
			// При сигнале отмены завершаемся (оставшиеся задачи будут отброшены)
			return
		case h.workerChans[workerIdx] <- task:
			// отправлено успешно
		}
	}
}

func (h *Handler) worker(workerID int, taskChan <-chan DeleteTask) {
	defer h.wg.Done()

	const (
		batchSize     = 50 // максимальный размер батча для одного пользователя
		flushInterval = 5 * time.Second
	)

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	// Локальный буфер: userID -> []ids
	pending := make(map[string][]string)

	// Функция сброса буфера в БД
	flush := func() {
		if len(pending) == 0 {
			return
		}
		for userID, ids := range pending {
			if len(ids) == 0 {
				continue
			}
			// Копируем, чтобы не держать ссылку на map
			idsCopy := make([]string, len(ids))
			copy(idsCopy, ids)

			ctx, cancel := context.WithTimeout(h.ctx, 30*time.Second)
			if err := h.storage.DeleteURLs(ctx, idsCopy, userID); err != nil {
				logger.Log.Error("failed to delete URLs",
					zap.Int("worker", workerID),
					zap.Error(err),
					zap.String("userID", userID),
					zap.Strings("ids", idsCopy))
			}
			cancel()
		}
		// Очищаем map
		for k := range pending {
			delete(pending, k)
		}
	}

	for {
		select {
		case <-h.ctx.Done():
			// Принудительно сбрасываем буфер перед выходом
			flush()
			return

		case task, ok := <-taskChan:
			if !ok {
				// Канал закрыт – больше задач не будет
				flush()
				return
			}
			// Добавляем задачу в буфер
			pending[task.UserID] = append(pending[task.UserID], task.ID)

			// Проверяем общий размер накопленных задач
			total := 0
			for _, list := range pending {
				total += len(list)
			}
			if total >= batchSize {
				flush()
			}

		case <-ticker.C:
			// Периодический сброс
			flush()
		}
	}
}
