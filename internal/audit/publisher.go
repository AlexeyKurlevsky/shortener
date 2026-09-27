package audit

import (
	"context"
	"sync"

	"github.com/AlexeyKurlevsky/shortener/internal/logger"
	"go.uber.org/zap"
)

// Publisher — издатель событий аудита.
// Обработчики вызывают Publish, наблюдатели подписываются через Subscribe.
type Publisher struct {
	mu        sync.RWMutex
	observers []Observer

	ch chan Event
	wg sync.WaitGroup
}

func NewPublisher(buffer int) *Publisher {
	if buffer <= 0 {
		buffer = 1024
	}
	return &Publisher{ch: make(chan Event, buffer)}
}

func (p *Publisher) Subscribe(o Observer) {
	p.mu.Lock()
	p.observers = append(p.observers, o)
	p.mu.Unlock()
}

// Start запускает воркер, разгребающий очередь событий.
func (p *Publisher) Start(ctx context.Context) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-ctx.Done():
				// Сливаем остаток очереди и выходим.
				for {
					select {
					case e := <-p.ch:
						p.dispatch(e)
					default:
						return
					}
				}
			case e, ok := <-p.ch:
				if !ok {
					return
				}
				p.dispatch(e)
			}
		}
	}()
}

// Publish не блокирует вызывающего.
func (p *Publisher) Publish(ctx context.Context, e Event) {
	select {
	case p.ch <- e:
	case <-ctx.Done():
		logger.Log.Warn("audit publish canceled", zap.Error(ctx.Err()))
	default:
		logger.Log.Warn("audit buffer full, event dropped",
			zap.String("action", string(e.Action)))
	}
}

// Close корректно останавливает воркер.
func (p *Publisher) Close() {
	close(p.ch)
	p.wg.Wait()
}

func (p *Publisher) dispatch(e Event) {
	p.mu.RLock()
	subs := make([]Observer, len(p.observers))
	copy(subs, p.observers)
	p.mu.RUnlock()

	for _, o := range subs {
		func(o Observer) {
			defer func() {
				if r := recover(); r != nil {
					logger.Log.Error("audit observer panic", zap.Any("panic", r))
				}
			}()
			if err := o.Notify(context.Background(), e); err != nil {
				logger.Log.Error("audit observer failed", zap.Error(err))
			}
		}(o)
	}
}
