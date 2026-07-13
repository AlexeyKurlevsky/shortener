package config

import (
	"flag"
	"log"
	"strings"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	ServerAddr      string `env:"SERVER_ADDRESS"`
	BaseURL         string `env:"BASE_URL"`
	LogLevel        string `env:"LOG"`
	FileStoragePath string `env:"FILE_STORAGE_PATH"`
}

func NewConfig() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.ServerAddr, "a", ":8080", "address to run server (e.g., localhost:8888)")
	flag.StringVar(&cfg.BaseURL, "b", "http://localhost:8080", "base URL for shortened links (e.g., http://localhost:8000)")
	flag.StringVar(&cfg.FileStoragePath, "f", "storage.json", "path file storage")
	flag.StringVar(&cfg.LogLevel, "l", "info", "log level")
	flag.Parse()

	if err := env.Parse(cfg); err != nil {
		log.Fatal(err)
	}

	if !strings.HasPrefix(cfg.BaseURL, "http://") && !strings.HasPrefix(cfg.BaseURL, "https://") {
		cfg.BaseURL = "http://" + cfg.BaseURL
	}

	return cfg
}
