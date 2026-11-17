package middleware

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// test decode "hello world"
func testHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}
	if len(body) > 0 {
		_, _ = w.Write(body)
	} else {
		_, _ = w.Write([]byte("Hello, World!"))
	}
}

func jsonHandler(w http.ResponseWriter, r *http.Request) {
	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(data)
}

func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		return io.ReadAll(gr)
	}
	return io.ReadAll(resp.Body)
}

func TestGzipMiddleware_ResponseGzip(t *testing.T) {
	handler := GzipMiddleware(http.HandlerFunc(testHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json") // must match middleware condition
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if ce := resp.Header.Get("Content-Encoding"); ce != "gzip" {
		t.Errorf("expected Content-Encoding: gzip, got %q", ce)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	expected := "Hello, World!"
	if string(body) != expected {
		t.Errorf("expected body %q, got %q", expected, string(body))
	}
}

func TestGzipMiddleware_ResponseNoGzip(t *testing.T) {
	handler := GzipMiddleware(http.HandlerFunc(testHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No Accept-Encoding -> no compression regardless of Content-Type
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if ce := resp.Header.Get("Content-Encoding"); ce == "gzip" {
		t.Error("unexpected Content-Encoding: gzip")
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	expected := "Hello, World!"
	if string(body) != expected {
		t.Errorf("expected body %q, got %q", expected, string(body))
	}
}

func TestGzipMiddleware_RequestGzip(t *testing.T) {
	originalBody := "Hello, Server!"
	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	_, err := gw.Write([]byte(originalBody))
	if err != nil {
		t.Fatalf("failed to write gzip: %v", err)
	}
	gw.Close()

	req := httptest.NewRequest(http.MethodPost, "/", buf)
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json") // required for decompression

	handler := GzipMiddleware(http.HandlerFunc(testHandler))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if ce := resp.Header.Get("Content-Encoding"); ce != "gzip" {
		t.Errorf("expected Content-Encoding: gzip, got %q", ce)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if string(body) != originalBody {
		t.Errorf("expected echoed body %q, got %q", originalBody, string(body))
	}
}

func TestGzipMiddleware_RequestNoGzip(t *testing.T) {
	originalBody := "Plain request body"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(originalBody))
	req.Header.Set("Content-Type", "application/json") // not needed but harmless
	handler := GzipMiddleware(http.HandlerFunc(testHandler))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	// No Accept-Encoding -> no compression
	if ce := resp.Header.Get("Content-Encoding"); ce == "gzip" {
		t.Error("unexpected Content-Encoding: gzip")
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if string(body) != originalBody {
		t.Errorf("expected echoed body %q, got %q", originalBody, string(body))
	}
}

func TestGzipMiddleware_RequestGzipInvalidData(t *testing.T) {
	invalidData := "this is not gzip"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(invalidData))
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json") // must be set to trigger decompression

	handler := GzipMiddleware(http.HandlerFunc(testHandler))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", resp.StatusCode)
	}
}

func TestGzipMiddleware_Combined(t *testing.T) {
	originalBody := "Double compression test"
	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	_, err := gw.Write([]byte(originalBody))
	if err != nil {
		t.Fatalf("failed to write gzip: %v", err)
	}
	gw.Close()

	req := httptest.NewRequest(http.MethodPost, "/", buf)
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")

	handler := GzipMiddleware(http.HandlerFunc(testHandler))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if ce := resp.Header.Get("Content-Encoding"); ce != "gzip" {
		t.Errorf("expected Content-Encoding: gzip, got %q", ce)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if string(body) != originalBody {
		t.Errorf("expected echoed body %q, got %q", originalBody, string(body))
	}
}

func TestGzipMiddleware_JSONRequestGzipResponse(t *testing.T) {
	originalJSON := map[string]string{"key": "value", "foo": "bar"}
	jsonBytes, err := json.Marshal(originalJSON)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	if _, err = gw.Write(jsonBytes); err != nil {
		t.Fatalf("failed to write gzip: %v", err)
	}
	gw.Close()

	req := httptest.NewRequest(http.MethodPost, "/", buf)
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")

	handler := GzipMiddleware(http.HandlerFunc(jsonHandler))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if ce := resp.Header.Get("Content-Encoding"); ce != "gzip" {
		t.Errorf("expected Content-Encoding: gzip, got %q", ce)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type: application/json, got %q", ct)
	}

	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	var respData map[string]string
	if err := json.Unmarshal(body, &respData); err != nil {
		t.Fatalf("failed to unmarshal response JSON: %v", err)
	}
	if !equalMapString(originalJSON, respData) {
		t.Errorf("expected %v, got %v", originalJSON, respData)
	}
}

func TestGzipMiddleware_JSONRequestNoGzipResponse(t *testing.T) {
	originalJSON := map[string]string{"hello": "world"}
	jsonBytes, err := json.Marshal(originalJSON)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	if _, err = gw.Write(jsonBytes); err != nil {
		t.Fatalf("failed to write gzip: %v", err)
	}
	gw.Close()

	req := httptest.NewRequest(http.MethodPost, "/", buf)
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")
	// No Accept-Encoding

	handler := GzipMiddleware(http.HandlerFunc(jsonHandler))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	// No Accept-Encoding -> no compression
	if ce := resp.Header.Get("Content-Encoding"); ce == "gzip" {
		t.Error("unexpected Content-Encoding: gzip")
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type: application/json, got %q", ct)
	}

	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	var respData map[string]string
	if err := json.Unmarshal(body, &respData); err != nil {
		t.Fatalf("failed to unmarshal response JSON: %v", err)
	}
	if !equalMapString(originalJSON, respData) {
		t.Errorf("expected %v, got %v", originalJSON, respData)
	}
}

func TestGzipMiddleware_JSONRequestPlainResponseGzip(t *testing.T) {
	originalJSON := map[string]string{"status": "ok"}
	jsonBytes, err := json.Marshal(originalJSON)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")

	handler := GzipMiddleware(http.HandlerFunc(jsonHandler))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if ce := resp.Header.Get("Content-Encoding"); ce != "gzip" {
		t.Errorf("expected Content-Encoding: gzip, got %q", ce)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type: application/json, got %q", ct)
	}

	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	var respData map[string]string
	if err := json.Unmarshal(body, &respData); err != nil {
		t.Fatalf("failed to unmarshal response JSON: %v", err)
	}
	if !equalMapString(originalJSON, respData) {
		t.Errorf("expected %v, got %v", originalJSON, respData)
	}
}

func equalMapString(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func TestTrustedSubnetMiddleware(t *testing.T) {
	mustCIDR := func(s string) *net.IPNet {
		_, n, err := net.ParseCIDR(s)
		require.NoError(t, err)
		return n
	}

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	tests := []struct {
		name       string
		subnet     *net.IPNet
		realIP     string // "" => заголовок не выставляем
		wantStatus int
	}{
		{
			name:       "nil subnet denies all",
			subnet:     nil,
			realIP:     "10.0.0.1",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "nil subnet denies even empty header",
			subnet:     nil,
			realIP:     "",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "ip inside subnet v4",
			subnet:     mustCIDR("10.0.0.0/8"),
			realIP:     "10.5.6.7",
			wantStatus: http.StatusOK,
		},
		{
			name:       "ip outside subnet v4",
			subnet:     mustCIDR("10.0.0.0/8"),
			realIP:     "192.168.1.1",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "missing X-Real-IP",
			subnet:     mustCIDR("10.0.0.0/8"),
			realIP:     "",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "garbage X-Real-IP",
			subnet:     mustCIDR("10.0.0.0/8"),
			realIP:     "not-an-ip",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "ip inside subnet v6",
			subnet:     mustCIDR("::1/128"),
			realIP:     "::1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "ip outside subnet v6",
			subnet:     mustCIDR("::1/128"),
			realIP:     "::2",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "X-Real-IP with spaces is trimmed",
			subnet:     mustCIDR("10.0.0.0/8"),
			realIP:     "  10.1.2.3  ",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := TrustedSubnetMiddleware(tt.subnet)
			h := mw(okHandler)

			req := httptest.NewRequest(http.MethodGet, "/api/internal/stats", nil)
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}
