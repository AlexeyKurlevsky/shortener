package main

import (
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AlexeyKurlevsky/shortener/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type localDB map[string]string

var db localDB

func init() {
	rand.Seed(time.Now().UnixNano())
}

func generateID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 8
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func IsValidURL(str string) bool {
	u, err := url.ParseRequestURI(str)
	if err != nil {
		return false
	}
	if u.Scheme == "" || u.Host == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return true
}

func CreateShortURL(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	log.Printf("[CreateShortURL] Method=%s, Path=%s", r.Method, r.URL.Path)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	link := string(body)
	if !IsValidURL(link) {
		http.Error(w, "Invalid link", http.StatusBadRequest)
		return
	}
	var id string
	for {
		id = generateID()
		if _, exists := db[id]; !exists {
			break
		}
	}
	db[id] = link
	shortURL := cfg.BaseURL + "/" + id
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(shortURL))
}

func GetLink(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	original, ok := db[id]
	if !ok {
		http.Error(w, "URL not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Location", original)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func run(cfg *config.Config) error {
	db = make(localDB)
	r := chi.NewRouter()
	r.Use(middleware.Logger, middleware.Recoverer)
	r.Post("/", func(w http.ResponseWriter, req *http.Request) {
		CreateShortURL(w, req, cfg)
	})
	r.Get("/{id}", GetLink)

	log.Printf("Server starting on %s", cfg.ServerAddr)
	return http.ListenAndServe(cfg.ServerAddr, r)
}

func main() {
	cfg := config.NewConfig()
	if err := run(cfg); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
