package config

import (
	"errors"
	"os"
)

const defaultHTTPAddr = ":8080"

type Config struct {
	DatabaseURL string
	HTTPAddr    string
}

func Load() (Config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = defaultHTTPAddr
	}

	return Config{DatabaseURL: databaseURL, HTTPAddr: httpAddr}, nil
}
