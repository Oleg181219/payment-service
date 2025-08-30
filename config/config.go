package config

import (
	"os"
	"strconv"
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
}

func LoadConfig() *Config {
	godotenv.Load()

	return &Config{
		ServerPort:       getEnv("SERVER_PORT", "8080"),
		TonApiURL:        getEnv("TON_API_URL", "https://tonapi.io"),
		ApiKey:           getEnv("API_KEY", ""),
		RequestTimeout:   getEnvAsDuration("REQUEST_TIMEOUT", 30*time.Second),
		MaxRetries:       getEnvAsInt("MAX_RETRIES", 3),
		AppWallet:        getEnv("APP_WALLET", ""),
		MinConfirmations: getEnvAsInt("MIN_CONFIRMATIONS", 1),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
