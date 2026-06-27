package main

import (
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

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

func CreateShortURL(w http.ResponseWriter, r *http.Request) {
	log.Printf("[CreateShortURL] Method=%s, Path=%s", r.Method, r.URL.Path)

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[CreateShortURL] Error reading body: %v", err)
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}

	link := string(bodyBytes)
	log.Printf("[CreateShortURL] Received link: %q", link)

	if !IsValidURL(link) {
		log.Printf("[CreateShortURL] Invalid URL: %q", link)
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
	log.Printf("[CreateShortURL] Stored ID=%s -> %s", id, link)

	host := strings.Split(r.Host, ":")[0]
	shortURL := "http://" + host + flagTinyURL + "/" + id
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(shortURL))
	log.Printf("[CreateShortURL] Response: %s", shortURL)
}

func GetLink(w http.ResponseWriter, r *http.Request) {
	log.Printf("[GetLink] Method=%s, Path=%s", r.Method, r.URL.Path)

	id := chi.URLParam(r, "id")
	log.Printf("[GetLink] Extracted id=%q", id)

	if id == "" || strings.Contains(id, "/") {
		log.Printf("[GetLink] Invalid id: %q", id)
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}

	originalURL, exists := db[id]
	if !exists {
		log.Printf("[GetLink] ID %q not found in DB", id)
		http.Error(w, "URL not found", http.StatusNotFound)
		return
	}

	log.Printf("[GetLink] Redirecting %q -> %q", id, originalURL)
	w.Header().Set("Location", originalURL)
	w.WriteHeader(http.StatusTemporaryRedirect) // 307
}

func run() error {
	db = make(localDB)
	r := chi.NewRouter()
	r.Use(middleware.Logger, middleware.Recoverer)
	r.Post("/", CreateShortURL)
	r.Get("/{id}", GetLink)

	parseFlags()
	log.Println("Server starting on :8080")
	return http.ListenAndServe(flagRunAddr, r)
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
