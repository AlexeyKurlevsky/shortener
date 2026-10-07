package audit

import (
	"context"
	"sync"
	"testing"
)

// noopObserver — быстрейший наблюдатель: ничего не делает.
type noopObserver struct{}

func (noopObserver) Notify(context.Context, Event) error { return nil }

// slowishObserver — имитирует наблюдателя, который делает лёгкую работу
// (например, форматирование строки), но без блокирующего I/O.
type slowishObserver struct {
	sink int
}

func (o *slowishObserver) Notify(_ context.Context, e Event) error {
	o.sink += int(e.TS) + len(e.URL) + len(e.UserID)
	return nil
}

func benchEvent() Event {
	return Event{
		TS:     1700000000,
		Action: ActionShorten,
		UserID: "12315134",
		URL:    "https://example.com/some/path?query=value",
	}
}

// --- dispatch без наблюдателей: чистая стоимость обхода ---

func BenchmarkDispatch_NoObservers(b *testing.B) {
	pub := NewPublisher(1)
	e := benchEvent()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pub.dispatch(e)
	}
}

// --- dispatch с N наблюдателями, разные N ---

func BenchmarkDispatch_NoopObservers(b *testing.B) {
	for _, n := range []int{1, 5, 10, 50} {
		b.Run(nameN(n), func(b *testing.B) {
			pub := NewPublisher(1)
			for i := 0; i < n; i++ {
				pub.Subscribe(noopObserver{})
			}
			e := benchEvent()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pub.dispatch(e)
			}
		})
	}
}

func BenchmarkDispatch_WorkObservers(b *testing.B) {
	for _, n := range []int{1, 5, 10, 50} {
		b.Run(nameN(n), func(b *testing.B) {
			pub := NewPublisher(1)
			obs := make([]*slowishObserver, n)
			for i := range obs {
				obs[i] = &slowishObserver{}
				pub.Subscribe(obs[i])
			}
			e := benchEvent()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pub.dispatch(e)
			}
		})
	}
}

// --- стоимость Subscribe, чтобы понимать, окупается ли копирование ---

func BenchmarkSubscribe(b *testing.B) {
	pub := NewPublisher(1)
	o := noopObserver{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pub.Subscribe(o)
	}
}

// --- end-to-end: Publish через канал (асинхронная доставка) ---

func BenchmarkPublish_ChannelOnly(b *testing.B) {
	pub := NewPublisher(1024)
	// Start НЕ вызываем — измеряем только отправку в канал.

	e := benchEvent()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pub.Publish(e)
	}
}

func BenchmarkPublish_EndToEnd(b *testing.B) {
	for _, n := range []int{1, 5, 10} {
		b.Run(nameN(n), func(b *testing.B) {
			pub := NewPublisher(4096)
			for i := 0; i < n; i++ {
				pub.Subscribe(noopObserver{})
			}
			pub.Start()
			defer pub.Close()

			e := benchEvent()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pub.Publish(e)
			}
			b.StopTimer()

			// Дожидаемся дренажа буфера через Close в defer.
			// b.N событий должно быть обработано.
		})
	}
}

// --- параллельный Publish: показывает, что канал не блокирует конкурентных писателей ---

func BenchmarkPublish_Parallel(b *testing.B) {
	pub := NewPublisher(65536)
	for i := 0; i < 10; i++ {
		pub.Subscribe(noopObserver{})
	}
	pub.Start()
	defer pub.Close()

	e := benchEvent()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pub.Publish(e)
		}
	})
}

// --- с ошибками в наблюдателе: проверяем, что recover/лог не взрывают стоимость ---

type errorObserver struct{}

func (errorObserver) Notify(context.Context, Event) error { return context.Canceled }

func BenchmarkDispatch_WithErrors(b *testing.B) {
	pub := NewPublisher(1)
	for i := 0; i < 5; i++ {
		pub.Subscribe(errorObserver{})
	}
	e := benchEvent()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pub.dispatch(e)
	}
}

// --- смешанный набор: часть успешных, часть с ошибкой ---

func BenchmarkDispatch_MixedObservers(b *testing.B) {
	pub := NewPublisher(1)
	for i := 0; i < 5; i++ {
		pub.Subscribe(noopObserver{})
		pub.Subscribe(errorObserver{})
	}
	e := benchEvent()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pub.dispatch(e)
	}
}

// --- helpers ---

func nameN(n int) string {
	// fast enough, без strconv.Itoa на каждом запуске
	if n < 10 {
		return "N=0" + string(rune('0'+n))
	}
	return "N=" + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Заглушки, чтобы не тащить лишние импорты.
var (
	_ = sync.Mutex{}
	_ = context.Background
)
