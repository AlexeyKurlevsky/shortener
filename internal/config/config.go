package config

import (
	"flag"
	"log"
	"strings"

	"github.com/caarlos0/env"
)

type Config struct {
	ServerAddr      string `env:"SERVER_ADDRESS"`
	BaseURL         string `env:"BASE_URL"`
	LogLevel        string `env:"LOG" envDefault:"info"`
	FileStoragePath string
}

func NewConfig() *Config {
	var cfgFlag Config
	var cfgEnv Config

	flag.StringVar(&cfgFlag.ServerAddr, "a", ":8080", "address to run server (e.g., localhost:8888)")
	flag.StringVar(&cfgFlag.BaseURL, "b", "http://localhost:8080", "base URL for shortened links (e.g., http://localhost:8000)")
	flag.StringVar(&cfgFlag.FileStoragePath, "f", "storage.json", "path file storage")
	flag.StringVar(&cfgFlag.LogLevel, "l", "info", "log level")
	flag.Parse()

	err := env.Parse(&cfgEnv)
	if err != nil {
		log.Fatal(err)
	}

	// Приоритет у env
	if cfgEnv.BaseURL != "" {
		cfgFlag.BaseURL = cfgEnv.BaseURL
	}

	if cfgEnv.ServerAddr != "" {
		cfgFlag.ServerAddr = cfgEnv.ServerAddr
	}

	if cfgEnv.LogLevel != "" {
		cfgFlag.LogLevel = cfgEnv.LogLevel
	}

	if !strings.HasPrefix(cfgFlag.BaseURL, "http://") && !strings.HasPrefix(cfgFlag.BaseURL, "https://") {
		cfgFlag.BaseURL = "http://" + cfgFlag.BaseURL
	}

	return &cfgFlag
}s