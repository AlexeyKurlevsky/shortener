package pool

import (
	"sync"
	"testing"
)

// fake — простой «тяжёлый» объект, реализующий Reset().
type fake struct {
	Data []int
	Meta map[string]string
	Str  string
}

func (f *fake) Reset() {
	f.Data = f.Data[:0]
	clear(f.Meta)
	f.Str = ""
}

func TestNew_ReturnsPool(t *testing.T) {
	p := New(func() *fake { return &fake{} })
	if p == nil {
		t.Fatal("New вернул nil")
	}
}

func TestGet_CreatesNewWhenEmpty(t *testing.T) {
	p := New(func() *fake { return &fake{} })

	v := p.Get()
	if v == nil {
		t.Fatal("Get вернул nil из пустого пула")
	}
}

func TestPut_ResetsState(t *testing.T) {
	p := New(func() *fake { return &fake{} })

	v := p.Get()
	v.Data = append(v.Data, 1, 2, 3)
	v.Meta = map[string]string{"a": "b"}
	v.Str = "hello"

	p.Put(v)

	got := p.Get()
	if len(got.Data) != 0 {
		t.Fatalf("Data не сброшен: %v", got.Data)
	}
	if len(got.Meta) != 0 {
		t.Fatalf("Meta не сброшена: %v", got.Meta)
	}
	if got.Str != "" {
		t.Fatalf("Str не сброшен: %q", got.Str)
	}
}

func TestPool_ReusesObjects(t *testing.T) {
	p := New(func() *fake { return &fake{} })

	v := p.Get()
	p.Put(v)

	// Первый же Get после Put должен вернуть тот же указатель.
	got := p.Get()
	if got != v {
		t.Fatal("объект не переиспользован: ожидался тот же указатель")
	}
}

func TestPool_ConcurrentUse(t *testing.T) {
	p := New(func() *fake { return &fake{} })

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := p.Get()
			v.Data = append(v.Data, 1)
			v.Str = "x"
			p.Put(v)
		}()
	}
	wg.Wait()
}
