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

	// SMTPHost/Port/Username/Password/From configure internal/mail's SMTPMailer (email
	// verification, Phase 21 round 2). An empty SMTPHost means no real SMTP is configured --
	// internal/mail.NewMailerFromConfig falls back to logging instead of sending, never guessing
	// at credentials that were never supplied.
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	// AppBaseURL is this app's own externally-reachable origin, needed to build an absolute link
	// inside an email body -- a relative link means nothing once it's outside a browser tab that
	// already knows the origin. Defaults to a local dev URL using Port.
	AppBaseURL string
}

// Load reads Config from the environment, applying defaults where unset.
func Load() Config {
	port := getenv("PORT", "8080")
	return Config{
		Port:          port,
		DatabaseURL:   getenv("DATABASE_URL", ""),
		MetadataPath:  getenv("METADATA_PATH", "metadata/app.yaml"),
		AdminUsername: getenv("ADMIN_USERNAME", ""),
		AdminPassword: getenv("ADMIN_PASSWORD", ""),
		SessionSecret: getenv("SESSION_SECRET", ""),
		AdminUserID:   getenv("ADMIN_USER_ID", "admin"),
		SecureCookies: getenv("SECURE_COOKIES", "true") == "true",
		UploadsDir:    getenv("UPLOADS_DIR", "uploads"),
		SMTPHost:      getenv("SMTP_HOST", ""),
		SMTPPort:      getenv("SMTP_PORT", "587"),
		SMTPUsername:  getenv("SMTP_USERNAME", ""),
		SMTPPassword:  getenv("SMTP_PASSWORD", ""),
		SMTPFrom:      getenv("SMTP_FROM", ""),
		AppBaseURL:    getenv("APP_BASE_URL", "http://localhost:"+port),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
