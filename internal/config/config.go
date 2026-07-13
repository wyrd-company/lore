package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL     string
	AIGatewayAPIKey string
	IngestToken     string
	AdminToken      string
	PublicBaseURL   string
	ListenAddress   string
}

func FromEnvironment() Config {
	listen := os.Getenv("LORE_LISTEN_ADDRESS")
	if listen == "" {
		listen = ":8080"
	}
	return Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		AIGatewayAPIKey: os.Getenv("AI_GATEWAY_API_KEY"),
		IngestToken:     os.Getenv("LORE_INGEST_TOKEN"),
		AdminToken:      os.Getenv("LORE_ADMIN_TOKEN"),
		PublicBaseURL:   os.Getenv("PUBLIC_BASE_URL"),
		ListenAddress:   listen,
	}
}

func (c Config) ValidateDatabase() error {
	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	return nil
}

func (c Config) ValidateServer() error {
	if err := c.ValidateDatabase(); err != nil {
		return err
	}
	if c.IngestToken == "" {
		return errors.New("LORE_INGEST_TOKEN is required")
	}
	if c.AdminToken == "" {
		return errors.New("LORE_ADMIN_TOKEN is required")
	}
	return nil
}
