package audit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPObserver_Notify_Success(t *testing.T) {
	var got Event
	var gotCT string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	obs := NewHTTPObserver(srv.URL)
	err := obs.Notify(context.Background(), Event{
		TS:     42,
		Action: ActionShorten,
		UserID: "u1",
		URL:    "https://example.com",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	want := Event{TS: 42, Action: ActionShorten, UserID: "u1", URL: "https://example.com"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestHTTPObserver_Notify_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	obs := NewHTTPObserver(srv.URL)
	err := obs.Notify(context.Background(), Event{TS: 1, Action: ActionFollow, URL: "u"})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

func TestHTTPObserver_Notify_BadURL(t *testing.T) {
	obs := NewHTTPObserver("://not-a-url")
	err := obs.Notify(context.Background(), Event{TS: 1, Action: ActionShorten, URL: "u"})
	if err == nil {
		t.Fatal("expected error for bad URL, got nil")
	}
}

func TestHTTPObserver_Notify_ContextCanceled(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	obs := NewHTTPObserver(srv.URL)
	err := obs.Notify(ctx, Event{TS: 1, Action: ActionShorten, URL: "u"})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

func TestHTTPObserver_Notify_ConnectionRefused(t *testing.T) {
	// Порт, который точно никто не слушает.
	obs := NewHTTPObserver("http://127.0.0.1:1")
	err := obs.Notify(context.Background(), Event{TS: 1, Action: ActionShorten, URL: "u"})
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}
