package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort       string
	TonApiURL        string
	ApiKey           string
	RequestTimeout   time.Duration
	MaxRetries       int
	AppWallet        string
	MinConfirmations int
	DatabaseURL      string
}

func LoadConfig() *Config {
	// Подхват .env (если лежит в корне)
	_ = godotenv.Load()

	cfg := &Config{
		ServerPort:       firstNonEmpty(os.Getenv("SERVER_PORT"), os.Getenv("PORT"), "8080"),
		TonApiURL:        firstNonEmpty(os.Getenv("TONAPI_BASE"), os.Getenv("TON_API_URL"), os.Getenv("TONAPI_URL"), "https://tonapi.io"),
		ApiKey:           firstNonEmpty(os.Getenv("API_KEY"), os.Getenv("TONAPI_TOKEN"), ""),
		RequestTimeout:   saneDuration(getEnvAsDuration("REQUEST_TIMEOUT", 30*time.Second), 1*time.Second),
		MaxRetries:       getEnvAsInt("MAX_RETRIES", 3),
		AppWallet:        getEnv("APP_WALLET", ""),
		MinConfirmations: getEnvAsInt("MIN_CONFIRMATIONS", 1),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
	}

	// Жёсткий дефолт для локальной БД, если переменная пустая
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		cfg.DatabaseURL = "jdbc:postgresql://localhost:5008/woolf_db?sslmode=disable"
	}
	// Нормализуем для pgx (режем jdbc:, дописываем sslmode=disable при отсутствии)
	cfg.DatabaseURL = normalizeDSN(cfg.DatabaseURL)

	// В mock‑режиме можно подставить тестовый кошелёк, если пусто
	if strings.TrimSpace(cfg.AppWallet) == "" && strings.EqualFold(os.Getenv("TONAPI_MODE"), "mock") {
		cfg.AppWallet = "UQDoj1UzJasYurg5oLsfA69pmVG7ATWTxyxawgfGFvLffbX8"
	}

	return cfg
}

// ----------------- helpers -----------------

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvAsInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvAsDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// saneDuration — не даём поставить слишком маленький таймаут
func saneDuration(d time.Duration, min time.Duration) time.Duration {
	if d < min {
		return min
	}
	return d
}

// normalizeDSN — приводит DSN к виду, который понимает pgx:
// - убирает префикс "jdbc:"
// - добавляет sslmode=disable, если его нет
func normalizeDSN(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, "jdbc:") {
		s = strings.TrimPrefix(s, "jdbc:")
	}
	if !strings.Contains(s, "sslmode=") {
		sep := "?"
		if strings.Contains(s, "?") {
			sep = "&"
		}
		s = s + sep + "sslmode=disable"
	}
	return s
}