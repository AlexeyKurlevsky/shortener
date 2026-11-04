// Package config загружает и валидирует конфигурацию приложения
// из флагов командной строки, переменных окружения и JSON-файла.
//
// Приоритет (по возрастанию):
//  1. значения по умолчанию
//  2. JSON-файл (-c/-config или CONFIG)
//  3. переменные окружения
//  4. флаги командной строки (только явно переданные)
package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	ServerAddr      string `env:"SERVER_ADDRESS" json:"server_address"`
	BaseURL         string `env:"BASE_URL" json:"base_url"`
	LogLevel        string `env:"LOG" json:"log_level"`
	FileStoragePath string `env:"FILE_STORAGE_PATH" json:"file_storage_path"`
	DatabaseDSN     string `env:"DATABASE_DSN" json:"database_dsn"`
	SecretKey       string `env:"SECRET_KEY" json:"secret_key"`
	SecretKeyByte   []byte `json:"-"`
	AuditFile       string `env:"AUDIT_FILE" json:"audit_file"`
	AuditURL        string `env:"AUDIT_URL" json:"audit_url"`

	// HTTPS через ACME (autocert)
	EnableHTTPS  bool     `env:"ENABLE_HTTPS" json:"enable_https"`
	Domains      []string `env:"DOMAINS" envSeparator:"," json:"domains"`
	ACMEEmail    string   `env:"ACME_EMAIL" json:"acme_email"`
	ACMECacheDir string   `env:"ACME_CACHE_DIR" json:"acme_cache_dir"`
}

func NewConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on system envs")
	}

	// ---- Флаги во временные переменные ----
	var (
		configPath      string
		serverAddr      string
		baseURL         string
		fileStoragePath string
		logLevel        string
		databaseDSN     string
		secretKey       string
		auditFile       string
		auditURL        string
		enableHTTPS     bool
		domainsRaw      string
		acmeEmail       string
		acmeCache       string
	)

	flag.StringVar(&configPath, "c", "", "path to JSON config file")
	flag.StringVar(&configPath, "config", "", "path to JSON config file")
	flag.StringVar(&serverAddr, "a", ":8080", "address to run server (e.g., localhost:8888)")
	flag.StringVar(&baseURL, "b", "http://localhost:8080", "base URL for shortened links")
	flag.StringVar(&fileStoragePath, "f", "storage.json", "path file storage")
	flag.StringVar(&logLevel, "l", "info", "log level")
	flag.StringVar(&databaseDSN, "d", "", "DB DSN")
	flag.StringVar(&secretKey, "k", "", "secret key for cookie signing (base64)")
	flag.StringVar(&auditFile, "audit-file", "", "path to audit log file (disabled if empty)")
	flag.StringVar(&auditURL, "audit-url", "", "remote audit server URL (disabled if empty)")
	flag.BoolVar(&enableHTTPS, "s", false, "enable HTTPS via ACME (autocert); same as ENABLE_HTTPS=true")
	flag.StringVar(&domainsRaw, "domains", "", "comma-separated domains for ACME")
	flag.StringVar(&acmeEmail, "acme-email", "", "email for ACME registration")
	flag.StringVar(&acmeCache, "acme-cache", "certs", "directory for ACME certificate cache")

	flag.Parse()

	// ---- 1. Значения по умолчанию ----
	cfg := &Config{
		ServerAddr:      ":8080",
		BaseURL:         "http://localhost:8080",
		FileStoragePath: "storage.json",
		LogLevel:        "info",
		ACMECacheDir:    "certs",
	}

	// ---- 2. JSON-файл (наименьший приоритет) ----
	// Путь: явный флаг -c/-config > env CONFIG.
	cfgPath := configPath
	if cfgPath == "" {
		cfgPath = os.Getenv("CONFIG")
	}
	if cfgPath != "" {
		if err := loadJSON(cfgPath, cfg); err != nil {
			return nil, err
		}
		log.Printf("Loaded config from %s", cfgPath)
	}

	// ---- 3. Переменные окружения (перезаписывают JSON) ----
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	// ---- 4. Флаги: только явно переданные (наивысший приоритет) ----
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "a":
			cfg.ServerAddr = serverAddr
		case "b":
			cfg.BaseURL = baseURL
		case "f":
			cfg.FileStoragePath = fileStoragePath
		case "l":
			cfg.LogLevel = logLevel
		case "d":
			cfg.DatabaseDSN = databaseDSN
		case "k":
			cfg.SecretKey = secretKey
		case "audit-file":
			cfg.AuditFile = auditFile
		case "audit-url":
			cfg.AuditURL = auditURL
		case "s":
			cfg.EnableHTTPS = enableHTTPS
		case "domains":
			cfg.Domains = splitCommaList(domainsRaw)
		case "acme-email":
			cfg.ACMEEmail = acmeEmail
		case "acme-cache":
			cfg.ACMECacheDir = acmeCache
		}
	})

	// ---- Пост-обработка ----
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

	// autocert без доменов не сможет получить сертификат.
	if cfg.EnableHTTPS && len(cfg.Domains) == 0 {
		return nil, fmt.Errorf("HTTPS enabled but no domains provided (use -domains, DOMAINS or \"domains\" in JSON)")
	}

	return cfg, nil
}

// loadJSON читает JSON-файл и накладывает значения на cfg.
// Отсутствующие в файле поля остаются без изменений.
func loadJSON(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file %q: %w", path, err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse config file %q: %w", path, err)
	}
	return nil
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
