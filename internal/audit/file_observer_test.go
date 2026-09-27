package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestFileObserver_Notify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	obs, err := NewFileObserver(path)
	if err != nil {
		t.Fatalf("NewFileObserver: %v", err)
	}

	events := []Event{
		{TS: 1, Action: ActionShorten, UserID: "u1", URL: "https://a.example"},
		{TS: 2, Action: ActionFollow, UserID: "u1", URL: "https://b.example"},
		{TS: 3, Action: ActionFollow, URL: "https://c.example"}, // без UserID
	}

	for _, e := range events {
		if err := obs.Notify(context.Background(), e); err != nil {
			t.Fatalf("Notify: %v", err)
		}
	}
	if err := obs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Проверяем содержимое — по одной JSON-строке на событие.
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	var got []Event
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("unmarshal %q: %v", sc.Text(), err)
		}
		got = append(got, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(got) != len(events) {
		t.Fatalf("got %d events, want %d", len(got), len(events))
	}
	for i := range events {
		if got[i] != events[i] {
			t.Errorf("event %d = %+v, want %+v", i, got[i], events[i])
		}
	}
}

func TestFileObserver_Append(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	// Первый наблюдатель пишет событие и закрывается.
	obs1, err := NewFileObserver(path)
	if err != nil {
		t.Fatalf("NewFileObserver 1: %v", err)
	}
	if err := obs1.Notify(context.Background(), Event{TS: 1, Action: ActionShorten, URL: "a"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	_ = obs1.Close()

	// Второй — дописывает, не затирая предыдущее.
	obs2, err := NewFileObserver(path)
	if err != nil {
		t.Fatalf("NewFileObserver 2: %v", err)
	}
	if err := obs2.Notify(context.Background(), Event{TS: 2, Action: ActionFollow, URL: "b"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	_ = obs2.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Ожидаем две строки с ts=1 и ts=2.
	if n := len(splitLines(string(data))); n != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", n, string(data))
	}
}

func TestFileObserver_ConcurrentNotify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	obs, err := NewFileObserver(path)
	if err != nil {
		t.Fatalf("NewFileObserver: %v", err)
	}
	defer obs.Close()

	const goroutines = 20
	const perGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				_ = obs.Notify(context.Background(), Event{
					TS:     int64(i),
					Action: ActionShorten,
					URL:    "https://example.com",
				})
			}
		}(g)
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Каждая строка должна быть валидным JSON — проверяет отсутствие гонок при записи.
	lines := splitLines(string(data))
	if len(lines) != goroutines*perGoroutine {
		t.Fatalf("got %d lines, want %d", len(lines), goroutines*perGoroutine)
	}
	for i, l := range lines {
		var e Event
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("line %d is not valid JSON: %q (%v)", i, l, err)
		}
	}
}

func TestFileObserver_BadPath(t *testing.T) {
	_, err := NewFileObserver("/nonexistent-dir-xyz/audit.log")
	if err == nil {
		t.Fatal("expected error for bad path, got nil")
	}
}

func TestFileObserver_NotifyAfterClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	obs, err := NewFileObserver(path)
	if err != nil {
		t.Fatalf("NewFileObserver: %v", err)
	}
	if err := obs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Повторный Close не должен паниковать и должен вернуть nil.
	if err := obs.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	// Notify после Close — ожидаем ошибку, но без паники.
	if err := obs.Notify(context.Background(), Event{TS: 1, Action: ActionShorten, URL: "a"}); err == nil {
		t.Error("expected error after Close, got nil")
	}
}

// splitLines — вспомогательная функция.
func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
