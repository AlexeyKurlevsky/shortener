package config

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"log"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	ServerAddr      string `env:"SERVER_ADDRESS"`
	BaseURL         string `env:"BASE_URL"`
	LogLevel        string `env:"LOG"`
	FileStoragePath string `env:"FILE_STORAGE_PATH"`
	DatabaseDSN     string `env:"DATABASE_DSN"`
	SecretKey       string `env:"SECRET_KEY"`
	SecretKeyByte   []byte
}

func NewConfig() (*Config, error) {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, relying on system envs")
	}

	cfg := &Config{}

	flag.StringVar(&cfg.ServerAddr, "a", ":8080", "address to run server (e.g., localhost:8888)")
	flag.StringVar(&cfg.BaseURL, "b", "http://localhost:8080", "base URL for shortened links (e.g., http://localhost:8000)")
	flag.StringVar(&cfg.FileStoragePath, "f", "storage.json", "path file storage")
	flag.StringVar(&cfg.LogLevel, "l", "info", "log level")
	flag.StringVar(&cfg.DatabaseDSN, "d", "", "DB DSN")
	flag.StringVar(&cfg.SecretKey, "s", "", "secret key for cookie signing (base64)")
	flag.Parse()

	if err := env.Parse(cfg); err != nil {
		log.Fatal(err)
	}

	if !strings.HasPrefix(cfg.BaseURL, "http://") && !strings.HasPrefix(cfg.BaseURL, "https://") {
		cfg.BaseURL = "http://" + cfg.BaseURL
	}

	if cfg.SecretKey != "" {
		key, err := base64.StdEncoding.DecodeString(cfg.SecretKey)
		if err != nil {
			return nil, err
		}
		cfg.SecretKeyByte = key
	} else {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		cfg.SecretKeyByte = key
	}

	return cfg, nil
}
