package main

import (
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
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
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST requests are allowed!", http.StatusMethodNotAllowed)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}

	link := string(bodyBytes)
	if !IsValidURL(link) {
		http.Error(w, "Invalid link", http.StatusBadRequest)
		return
	}

	// Генерируем уникальный ID
	var id string
	for {
		id = generateID()
		if _, exists := db[id]; !exists {
			break
		}
	}
	db[id] = link

	// Формируем полный короткий URL
	shortURL := "http://" + r.Host + "/" + id
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(shortURL)) // возвращаем не ID, а полный URL
}

func GetLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET requests are allowed!", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}

	originalURL, exists := db[id]
	if !exists {
		http.Error(w, "URL not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Location", originalURL)
	w.WriteHeader(http.StatusTemporaryRedirect) // 307
}

func mainHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		CreateShortURL(w, r)
	case http.MethodGet:
		GetLink(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func run() error {
	db = make(localDB)
	mux := http.NewServeMux()
	mux.HandleFunc("/", mainHandler)
	return http.ListenAndServe(":8080", mux)
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}
