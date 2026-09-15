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
	// per-user login yet. These have no safe default and must be set explicitly.
	AdminUsername string
	AdminPassword string
	SessionSecret string
	// AdminUserID is the mch_user record the shared admin credential resolves to (ROADMAP.md
	// Phase 7) -- the session cookie signs this real identity, not a hardcoded literal. Defaults
	// to the placeholder "admin" before a real mch_user record exists (bootstrap: log in with
	// the shared credential, create the record, then set this env var to its id and restart).
	AdminUserID string
	// SecureCookies must be true in production (HTTPS); false for local http://localhost dev,
	// where a Secure cookie would be silently dropped by the browser.
	SecureCookies bool
	// UploadsDir is the local-disk root for FieldTypeFile uploads (ROADMAP.md Phase 11) --
	// 007 SS4.10's single-binary constraint, no object storage until a real scale case forces one.
	UploadsDir string
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
		AdminUserID:   getenv("ADMIN_USER_ID", "admin"),
		SecureCookies: getenv("SECURE_COOKIES", "true") == "true",
		UploadsDir:    getenv("UPLOADS_DIR", "uploads"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
