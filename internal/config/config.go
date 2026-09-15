// Package config reads runtime configuration from the environment. It carries no business
// or Runtime Metadata concerns — those belong to internal/metadata.
package config

import "os"

// Config holds process-level configuration.
type Config struct {
	Port         string
	DatabaseURL  string
	MetadataPath string

	// AdminUsername/AdminPassword and SessionSecret gate writes and reads behind a session
	// cookie (internal/authorization). Phase 2 (ROADMAP.md): one shared admin credential, no
	// User model yet. These have no safe default and must be set explicitly.
	AdminUsername string
	AdminPassword string
	SessionSecret string
	// SecureCookies must be true in production (HTTPS); false for local http://localhost dev,
	// where a Secure cookie would be silently dropped by the browser.
	SecureCookies bool
}

// Load reads Config from the environment, applying defaults where unset.
func Load() Config {
	return Config{
		Port:          getenv("PORT", "8080"),
		DatabaseURL:   getenv("DATABASE_URL", ""),
		MetadataPath:  getenv("METADATA_PATH", "metadata/app.yaml"),
		AdminUsername: getenv("ADMIN_USERNAME", ""),
		AdminPassword: getenv("ADMIN_PASSWORD", ""),
		SessionSecret: getenv("SESSION_SECRET", ""),
		SecureCookies: getenv("SECURE_COOKIES", "true") == "true",
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
