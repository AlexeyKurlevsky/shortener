// Package config загружает и валидирует конфигурацию приложения
// из флагов командной строки и переменных окружения.
package config

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
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
	AuditFile       string `env:"AUDIT_FILE"`
	AuditURL        string `env:"AUDIT_URL"`

	// HTTPS через ACME (autocert)
	EnableHTTPS  bool     `env:"ENABLE_HTTPS"`
	Domains      []string `env:"DOMAINS" envSeparator:","`
	ACMEEmail    string   `env:"ACME_EMAIL"`
	ACMECacheDir string   `env:"ACME_CACHE_DIR"`
}

func NewConfig() (*Config, error) {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, relying on system envs")
	}

	cfg := &Config{}

	// временные переменные для HTTPS/ACME — flag.Parse не умеет в []string
	var (
		domainsRaw  string
		enableHTTPS bool
		acmeEmail   string
		acmeCache   string
	)

	flag.StringVar(&cfg.ServerAddr, "a", ":8080", "address to run server (e.g., localhost:8888)")
	flag.StringVar(&cfg.BaseURL, "b", "http://localhost:8080", "base URL for shortened links (e.g., http://localhost:8000)")
	flag.StringVar(&cfg.FileStoragePath, "f", "storage.json", "path file storage")
	flag.StringVar(&cfg.LogLevel, "l", "info", "log level")
	flag.StringVar(&cfg.DatabaseDSN, "d", "", "DB DSN")
	// -k для секретного ключа, чтобы освободить -s под HTTPS
	flag.StringVar(&cfg.SecretKey, "k", "", "secret key for cookie signing (base64)")
	flag.StringVar(&cfg.AuditFile, "audit-file", "", "path to audit log file (disabled if empty)")
	flag.StringVar(&cfg.AuditURL, "audit-url", "", "remote audit server URL (disabled if empty)")

	// HTTPS / ACME
	flag.BoolVar(&enableHTTPS, "s", false, "enable HTTPS via ACME (autocert); same as ENABLE_HTTPS=true")
	flag.StringVar(&domainsRaw, "domains", "", "comma-separated domains for ACME (e.g., example.com,www.example.com)")
	flag.StringVar(&acmeEmail, "acme-email", "", "email for ACME registration")
	flag.StringVar(&acmeCache, "acme-cache", "certs", "directory for ACME certificate cache")

	flag.Parse()

	// Значения из флагов. env.Parse ниже перезапишет их, если переменная задана.
	cfg.EnableHTTPS = enableHTTPS
	cfg.Domains = splitCommaList(domainsRaw)
	cfg.ACMEEmail = acmeEmail
	cfg.ACMECacheDir = acmeCache

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

	// Валидация HTTPS: без доменов autocert не сможет получить сертификат.
	if cfg.EnableHTTPS && len(cfg.Domains) == 0 {
		return nil, fmt.Errorf("HTTPS enabled but no domains provided (use -domains or DOMAINS)")
	}

	return cfg, nil
}

// splitCommaList парсит "a, b ,c" → ["a","b","c"], отбрасывая пустые элементы.
func splitCommaList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
