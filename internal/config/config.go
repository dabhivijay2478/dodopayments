package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL         string
	Port                string
	PSPBaseURL          string
	SeedData            bool
	BootstrapAllowForce bool
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// Load reads optional .env, then builds Config from environment variables.
// Tries "." and ".." so `go test ./tests/...` works when the IDE cwd is the tests/ folder.
// Docker Compose and production inject vars directly; .env is for local development.
func Load() Config {
	for _, path := range []string{".env", "../.env"} {
		_ = godotenv.Load(path)
		if os.Getenv("DATABASE_URL") != "" {
			break
		}
	}

	return Config{
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		Port:                getEnvOrDefault("PORT", "8080"),
		PSPBaseURL:          getEnvOrDefault("PSP_BASE_URL", "http://localhost:9090"),
		SeedData:            os.Getenv("SEED_DATA") == "true",
		BootstrapAllowForce: os.Getenv("BOOTSTRAP_ALLOW_FORCE") == "true",
	}
}
