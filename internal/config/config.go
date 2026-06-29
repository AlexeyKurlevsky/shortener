package config

import (
	"flag"
	"log"
	"strings"
)

type Config struct {
	ServerAddr      string
	BaseURL         string
	FileStoragePath string
}

func NewConfig() *Config {
	var cfg Config

	flag.StringVar(&cfg.ServerAddr, "a", ":8080", "address to run server (e.g., localhost:8888)")
	flag.StringVar(&cfg.BaseURL, "b", "http://localhost:8080", "base URL for shortened links (e.g., http://localhost:8000)")
	flag.StringVar(&cfg.FileStoragePath, "f", "storage.json", "path file storage")
	flag.Parse()

	if !strings.HasPrefix(cfg.BaseURL, "http://") && !strings.HasPrefix(cfg.BaseURL, "https://") {
		cfg.BaseURL = "http://" + cfg.BaseURL
	}

	log.Printf("Config: ServerAddr=%s, BaseURL=%s, FileStoragePath=%s", cfg.ServerAddr, cfg.BaseURL, cfg.FileStoragePath)
	return &cfg
}
