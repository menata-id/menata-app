// Package config reads runtime configuration from the environment. It carries no business
// or Runtime Metadata concerns — those belong to internal/metadata.
package config

import "os"

// Config holds process-level configuration.
type Config struct {
	Port         string
	DatabaseURL  string
	MetadataPath string
}

// Load reads Config from the environment, applying defaults where unset.
func Load() Config {
	return Config{
		Port:         getenv("PORT", "8080"),
		DatabaseURL:  getenv("DATABASE_URL", ""),
		MetadataPath: getenv("METADATA_PATH", "metadata/task.yaml"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
