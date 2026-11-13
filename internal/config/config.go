package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port        string
	DatabaseURL string
}

func Load() (Config, error) {
	cfg := Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}

	if p, err := strconv.Atoi(cfg.Port); err != nil || p < 1 || p > 65535 {
		return Config{}, fmt.Errorf("invalid PORT %q: must be a number between 1 and 65535", cfg.Port)
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
