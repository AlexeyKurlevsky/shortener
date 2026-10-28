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
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type Config struct {
	ServerAddr      string `env:"SERVER_ADDRESS"      mapstructure:"server-addr"`
	BaseURL         string `env:"BASE_URL"            mapstructure:"base-url"`
	LogLevel        string `env:"LOG"                 mapstructure:"log-level"`
	FileStoragePath string `env:"FILE_STORAGE_PATH"   mapstructure:"file-storage-path"`
	DatabaseDSN     string `env:"DATABASE_DSN"        mapstructure:"database-dsn"`
	SecretKey       string `env:"SECRET_KEY"          mapstructure:"secret-key"`
	SecretKeyByte   []byte `mapstructure:"-"`
	AuditFile       string `env:"AUDIT_FILE"          mapstructure:"audit-file"`
	AuditURL        string `env:"AUDIT_URL"           mapstructure:"audit-url"`

	EnableHTTPS  bool     `env:"ENABLE_HTTPS"   mapstructure:"enable-https"`
	Domains      []string `env:"DOMAINS" envSeparator:"," mapstructure:"domains"`
	ACMEEmail    string   `env:"ACME_EMAIL"     mapstructure:"acme-email"`
	ACMECacheDir string   `env:"ACME_CACHE_DIR" mapstructure:"acme-cache"`
}

func NewConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on system envs")
	}

	v := viper.New()

	// ---- 1. Значения по умолчанию ----
	v.SetDefault("server-addr", ":8080")
	v.SetDefault("base-url", "http://localhost:8080")
	v.SetDefault("log-level", "info")
	v.SetDefault("file-storage-path", "storage.json")
	v.SetDefault("acme-cache", "certs")

	// ---- 2. JSON-файл (наименьший приоритет) ----
	// Путь: флаг -c/-config > env CONFIG > (нет файла).
	// Читаем путь ДО viper.ReadInConfig, чтобы учесть флаг.
	cfgPath := ""
	pf := newFlagSet()
	if err := pf.Parse(os.Args[1:]); err != nil {
		return nil, err
	}
	if f := pf.Lookup("config"); f != nil {
		cfgPath = f.Value.String()
	}
	if cfgPath == "" {
		cfgPath = os.Getenv("CONFIG")
	}

	if cfgPath != "" {
		v.SetConfigFile(cfgPath)
		v.SetConfigType("json")
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config file %q: %w", cfgPath, err)
		}
		log.Printf("Loaded config from %s", cfgPath)
	}

	// ---- 3. Переменные окружения ----
	// Явно связываем ключи viper с существующими именами env,
	// чтобы не менять их и не добавлять префикс.
	_ = v.BindEnv("server-addr", "SERVER_ADDRESS")
	_ = v.BindEnv("base-url", "BASE_URL")
	_ = v.BindEnv("log-level", "LOG")
	_ = v.BindEnv("file-storage-path", "FILE_STORAGE_PATH")
	_ = v.BindEnv("database-dsn", "DATABASE_DSN")
	_ = v.BindEnv("secret-key", "SECRET_KEY")
	_ = v.BindEnv("audit-file", "AUDIT_FILE")
	_ = v.BindEnv("audit-url", "AUDIT_URL")
	_ = v.BindEnv("enable-https", "ENABLE_HTTPS")
	_ = v.BindEnv("domains", "DOMAINS")
	_ = v.BindEnv("acme-email", "ACME_EMAIL")
	_ = v.BindEnv("acme-cache", "ACME_CACHE_DIR")

	// ---- 4. Флаги (наивысший приоритет) ----
	// BindPFlags связывает ВСЕ зарегистрированные флаги автоматически:
	// добавление нового флага в newFlagSet() больше не требует правок здесь.
	if err := v.BindPFlags(pf); err != nil {
		return nil, err
	}

	// ---- Слияние в структуру ----
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

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

	if cfg.EnableHTTPS && len(cfg.Domains) == 0 {
		return nil, fmt.Errorf("HTTPS enabled but no domains provided (use --domains, DOMAINS or \"domains\" in JSON)")
	}

	return &cfg, nil
}

// newFlagSet создаёт и возвращает FlagSet со всеми флагами приложения.
// Чтобы добавить новый флаг, достаточно добавить одну строку сюда —
// viper подхватит его автоматически через BindPFlags.
func newFlagSet() *pflag.FlagSet {
	pf := pflag.NewFlagSet("shortener", pflag.ContinueOnError)

	pf.StringP("config", "c", "", "path to JSON config file")
	pf.StringP("server-addr", "a", ":8080", "address to run server (e.g., localhost:8888)")
	pf.StringP("base-url", "b", "http://localhost:8080", "base URL for shortened links")
	pf.StringP("file-storage-path", "f", "storage.json", "path file storage")
	pf.StringP("log-level", "l", "info", "log level")
	pf.StringP("database-dsn", "d", "", "DB DSN")
	pf.StringP("secret-key", "k", "", "secret key for cookie signing (base64)")
	pf.String("audit-file", "", "path to audit log file (disabled if empty)")
	pf.String("audit-url", "", "remote audit server URL (disabled if empty)")
	pf.BoolP("enable-https", "s", false, "enable HTTPS via ACME (autocert); same as ENABLE_HTTPS=true")
	pf.String("domains", "", "comma-separated domains for ACME")
	pf.String("acme-email", "", "email for ACME registration")
	pf.String("acme-cache", "certs", "directory for ACME certificate cache")

	return pf
}
