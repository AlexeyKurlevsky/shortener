package config

import (
	"flag"
	"log"
	"strings"

	"github.com/caarlos0/env"
)

type Config struct {
	ServerAddr      string `env:"SERVER_ADDRESS" envDefault:":8080"`
	BaseURL         string `env:"BASE_URL" envDefault:"http://localhost:8080"`
	LogLevel        string `env:"LOG" envDefault:"info"`
	FileStoragePath string `env:"FILE_STORAGE_PATH" envDefault:"storage.json"`
}

func NewConfig() *Config {
	var cfgFlag Config

	flag.StringVar(&cfgFlag.ServerAddr, "a", ":8080", "address to run server (e.g., localhost:8888)")
	flag.StringVar(&cfgFlag.BaseURL, "b", "http://localhost:8080", "base URL for shortened links (e.g., http://localhost:8000)")
	flag.StringVar(&cfgFlag.FileStoragePath, "f", "storage.json", "path file storage")
	flag.StringVar(&cfgFlag.LogLevel, "l", "info", "log level")
	flag.Parse()

	// Приоритет у env

	err := env.Parse(&cfgFlag)
	if err != nil {
		log.Fatal(err)
	}

	if !strings.HasPrefix(cfgFlag.BaseURL, "http://") && !strings.HasPrefix(cfgFlag.BaseURL, "https://") {
		cfgFlag.BaseURL = "http://" + cfgFlag.BaseURL
	}

	return &cfgFlag
}
