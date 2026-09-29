package audit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordingObserver — складывает события в срез для проверок.
type recordingObserver struct {
	mu     sync.Mutex
	events []Event
}

func (o *recordingObserver) Notify(_ context.Context, e Event) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, e)
	return nil
}

func (o *recordingObserver) snapshot() []Event {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]Event, len(o.events))
	copy(out, o.events)
	return out
}

// failingObserver — всегда возвращает ошибку.
type failingObserver struct {
	calls int32
}

func (o *failingObserver) Notify(_ context.Context, _ Event) error {
	atomic.AddInt32(&o.calls, 1)
	return errors.New("boom")
}

// panickingObserver — паникует при вызове.
type panickingObserver struct {
	calls int32
}

func (o *panickingObserver) Notify(_ context.Context, _ Event) error {
	atomic.AddInt32(&o.calls, 1)
	panic("simulated panic")
}

func TestPublisher_DeliversToAllObservers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pub := NewPublisher(16)
	obs1 := &recordingObserver{}
	obs2 := &recordingObserver{}
	pub.Subscribe(obs1)
	pub.Subscribe(obs2)
	pub.Start(ctx)

	want := []Event{
		{TS: 1, Action: ActionShorten, URL: "a"},
		{TS: 2, Action: ActionFollow, URL: "b"},
	}
	for _, e := range want {
		pub.Publish(context.Background(), e)
	}

	// Ждём доставки всем наблюдателям (короткое окно + поллинг).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(obs1.snapshot()) == len(want) && len(obs2.snapshot()) == len(want) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	got1 := obs1.snapshot()
	got2 := obs2.snapshot()
	if len(got1) != len(want) || len(got2) != len(want) {
		t.Fatalf("obs1=%d obs2=%d, want %d", len(got1), len(got2), len(want))
	}
	for i := range want {
		if got1[i] != want[i] {
			t.Errorf("obs1[%d] = %+v, want %+v", i, got1[i], want[i])
		}
		if got2[i] != want[i] {
			t.Errorf("obs2[%d] = %+v, want %+v", i, got2[i], want[i])
		}
	}
}

func TestPublisher_IsolatesObserverErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pub := NewPublisher(4)
	failing := &failingObserver{}
	rec := &recordingObserver{}
	pub.Subscribe(failing)
	pub.Subscribe(rec)
	pub.Start(ctx)

	pub.Publish(context.Background(), Event{TS: 1, Action: ActionShorten, URL: "a"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(rec.snapshot()) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := len(rec.snapshot()); got != 1 {
		t.Errorf("recorder got %d events, want 1 (failing observer should not block others)", got)
	}
	if atomic.LoadInt32(&failing.calls) == 0 {
		t.Error("failing observer was not called")
	}
}

func TestPublisher_RecoversFromPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pub := NewPublisher(4)
	panicky := &panickingObserver{}
	rec := &recordingObserver{}
	pub.Subscribe(panicky)
	pub.Subscribe(rec)
	pub.Start(ctx)

	pub.Publish(context.Background(), Event{TS: 1, Action: ActionShorten, URL: "a"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(rec.snapshot()) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := len(rec.snapshot()); got != 1 {
		t.Errorf("recorder got %d events, want 1 (panic should be recovered)", got)
	}
}

func TestPublisher_DropsWhenBufferFull(t *testing.T) {
	// Start НЕ вызываем — воркер не вычитывает канал, буфер переполняется.
	pub := NewPublisher(1)
	slow := &recordingObserver{}
	pub.Subscribe(slow)

	// Заполняем буфер (1) и отправляем ещё несколько — лишние должны быть
	// отброшены без блокировки вызывающего.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			pub.Publish(context.Background(), Event{TS: int64(i), Action: ActionShorten, URL: "u"})
		}
		close(done)
	}()

	select {
	case <-done:
		// ок — Publish не заблокировался
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Publish blocked on full buffer")
	}

	// Start запускаем после — события из буфера всё же доедут.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pub.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(slow.snapshot()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Точное число не проверяем — нам важен факт отсутствия блокировки и доставки.
	if got := len(slow.snapshot()); got == 0 {
		t.Error("expected at least one event to be delivered")
	}
}

func TestPublisher_PublishCanceledContext(t *testing.T) {
	// Start не вызываем, буфер полон → Publish с отменённым ctx должен выйти без блокировки.
	pub := NewPublisher(1)
	pub.Publish(context.Background(), Event{TS: 1, Action: ActionShorten, URL: "u"}) // занимает единственный слот

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		pub.Publish(ctx, Event{TS: 2, Action: ActionFollow, URL: "u"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Publish with canceled ctx blocked")
	}
}

func TestPublisher_CloseDrainsQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pub := NewPublisher(64)
	rec := &recordingObserver{}
	pub.Subscribe(rec)
	pub.Start(ctx)

	const total = 50
	for i := 0; i < total; i++ {
		pub.Publish(context.Background(), Event{TS: int64(i), Action: ActionShorten, URL: "u"})
	}

	// Close должен дождаться, пока воркер вычитает оставшиеся события
	// (ветка <-ctx.Done() внутри Start тоже дренирует очередь).
	pub.Close()

	if got := len(rec.snapshot()); got != total {
		t.Errorf("after Close got %d events, want %d", got, total)
	}
}

func TestPublisher_SubscribeAfterStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pub := NewPublisher(8)
	pub.Start(ctx)

	rec := &recordingObserver{}
	pub.Subscribe(rec) // подписка после старта тоже должна работать

	pub.Publish(context.Background(), Event{TS: 1, Action: ActionShorten, URL: "a"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(rec.snapshot()) == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("event not delivered to late subscriber")
}
