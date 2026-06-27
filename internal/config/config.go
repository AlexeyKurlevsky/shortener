package config

import (
	"flag"
	"strings"
)

type Config struct {
	ServerAddr string
	BaseURL    string
}

func NewConfig() *Config {
	var cfg Config

	flag.StringVar(&cfg.ServerAddr, "a", "localhost:8888", "address and port to run server (e.g., localhost:8080)")
	flag.StringVar(&cfg.BaseURL, "b", "localhost:8000", "base address for shortened URLs (e.g., localhost:8000)")
	flag.Parse()

	if !strings.HasPrefix(cfg.BaseURL, "http://") && !strings.HasPrefix(cfg.BaseURL, "https://") {
		cfg.BaseURL = "http://" + cfg.BaseURL
	}

	return &cfg
}
