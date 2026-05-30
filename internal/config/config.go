package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	Port        string
	PSPBaseURL  string
	SeedData    bool
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// Load reads optional .env from the working directory, then builds Config from environment variables.
// Docker Compose and production should inject the same variables; .env is for local development.
func Load() Config {
	_ = godotenv.Load()

	return Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Port:        getEnvOrDefault("PORT", "8080"),
		PSPBaseURL:  getEnvOrDefault("PSP_BASE_URL", "http://localhost:9090"),
		SeedData:    os.Getenv("SEED_DATA") == "true",
	}
}
