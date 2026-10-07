package audit

import (
	"context"
	"sync"

	"github.com/AlexeyKurlevsky/shortener/internal/logger"
	"go.uber.org/zap"
)

type Publisher struct {
	mu        sync.RWMutex
	observers []Observer

	ch chan Event
	wg sync.WaitGroup

	cancel context.CancelFunc
	once   sync.Once
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

// Start запускает воркер. Воркер живёт до Close().
func (p *Publisher) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-ctx.Done():
				p.drain()
				return
			case e := <-p.ch:
				p.dispatch(e)
			}
		}
	}()
}

// Publish не блокирует вызывающего. При переполнении буфера событие теряется.
func (p *Publisher) Publish(e Event) {
	select {
	case p.ch <- e:
	default:
		logger.Log.Warn("audit buffer full, event dropped",
			zap.String("action", string(e.Action)))
	}
}

// Close останавливает воркер: сигналит о завершении, дожидается обработки
// остатка очереди и возвращается. Идемпотентен.
func (p *Publisher) Close() {
	p.once.Do(func() {
		if p.cancel != nil {
			p.cancel()
		}
	})
	p.wg.Wait()
}

// drain вычитывает остаток буфера после сигнала о завершении.
func (p *Publisher) drain() {
	for {
		select {
		case e := <-p.ch:
			p.dispatch(e)
		default:
			return
		}
	}
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
