// Package config reads runtime configuration from the environment. It carries no business
// or Runtime Metadata concerns — those belong to internal/metadata.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds process-level configuration.
type Config struct {
	Port         string
	DatabaseURL  string
	MetadataPath string
	// TemplatePath is the template library root: the *.yaml Machine files an Application is installed
	// from, plus applications/ beside them (internal/installer). Separate from MetadataPath, which is
	// where installations land, because the two are genuinely different directories with different
	// rules -- one is only ever read, the other is written per Workspace. Deriving one from the other
	// (the library is MetadataPath's parent today) would be true only for the default layout.
	TemplatePath string

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
	// UploadsDir is the local-disk root for FieldTypeFile uploads (development-history.md Phase 11) --
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

	// GeminiAPIKey configures internal/aiassist's real Gemini client (the "New application"/
	// "extend an application" AI conversation, Flow 2 gap study Tahap 8). An empty value means no
	// AI assistant is configured -- internal/aiassist.NewClientFromConfig returns an
	// UnconfiguredClient (mirrors SMTPHost's own posture), and the feature's own entry point is
	// hidden rather than offered-then-broken (internal/web, same "hide, don't 403" convention
	// ShowMembersAndGroups already uses).
	GeminiAPIKey string

	// AndroidPackageName and AndroidCertFingerprints publish this origin's Digital Asset Links
	// (/.well-known/assetlinks.json), the proof Chrome asks for before a Trusted Web Activity -- the
	// Google Play wrapper around this PWA -- may open it without a browser address bar. The package
	// name is the one chosen in Play Console; the fingerprints are the SHA-256 of the key Play
	// *signs the release with* (Play Console > App integrity > App signing, not the upload keystore),
	// comma-separated so the upload and signing keys may both be listed. Either empty means no
	// Android app is declared: the route answers 404 rather than publishing a half-declared link.
	AndroidPackageName      string
	AndroidCertFingerprints []string

	// ScheduleIntervalMinutes is how often cmd/server's own ticker calls
	// execution.RunScheduledEvents (the SLA-breach reminder, Flow 2 canvas re-audit 2026-09-27) --
	// the first, and so far only, schedule-shaped Event this runtime evaluates. 15 minutes by
	// default: frequent enough that "overdue" reads as roughly real-time, infrequent enough that
	// it never competes meaningfully with request traffic for the pool.
	ScheduleIntervalMinutes int

	// TrustedProxies lists the addresses (CIDRs or bare IPs, comma-separated in TRUSTED_PROXIES) whose
	// X-Forwarded-For this app believes when rate limiting. Default is loopback, which is where the Caddy
	// reverse proxy on this host connects from; behind anything else the limiter would see the proxy as
	// the only client. A peer outside the list is never trusted, so a client cannot choose its own budget.
	TrustedProxies []string

	// StatementTimeoutSeconds bounds every SQL statement the app issues (db.Connect, K20). 20 by default: the
	// slowest statement measured at this app's volume is a small fraction of a second, so the bound exists to
	// stop a runaway query holding a pooled connection, not to shape normal traffic. 0 disables it.
	StatementTimeoutSeconds int
}

// Load reads Config from the environment, applying defaults where unset.
func Load() Config {
	port := getenv("PORT", "8080")
	return Config{
		Port:                    port,
		DatabaseURL:             getenv("DATABASE_URL", ""),
		MetadataPath:            getenv("METADATA_PATH", "metadata/workspaces"),
		TemplatePath:            getenv("TEMPLATE_PATH", "metadata"),
		AdminUsername:           getenv("ADMIN_USERNAME", ""),
		AdminPassword:           getenv("ADMIN_PASSWORD", ""),
		SessionSecret:           getenv("SESSION_SECRET", ""),
		AdminUserID:             getenv("ADMIN_USER_ID", "admin"),
		SecureCookies:           getenv("SECURE_COOKIES", "true") == "true",
		UploadsDir:              getenv("UPLOADS_DIR", "uploads"),
		SMTPHost:                getenv("SMTP_HOST", ""),
		SMTPPort:                getenv("SMTP_PORT", "587"),
		SMTPUsername:            getenv("SMTP_USERNAME", ""),
		SMTPPassword:            getenv("SMTP_PASSWORD", ""),
		SMTPFrom:                getenv("SMTP_FROM", ""),
		AppBaseURL:              getenv("APP_BASE_URL", "http://localhost:"+port),
		GeminiAPIKey:            getenv("GEMINI_API_KEY", ""),
		AndroidPackageName:      getenv("ANDROID_PACKAGE_NAME", ""),
		AndroidCertFingerprints: splitList(getenv("ANDROID_CERT_FINGERPRINTS", "")),
		TrustedProxies:          splitList(getenv("TRUSTED_PROXIES", "127.0.0.1,::1")),
		StatementTimeoutSeconds: getenvInt("STATEMENT_TIMEOUT_SECONDS", 20),
		ScheduleIntervalMinutes: getenvInt("SCHEDULE_INTERVAL_MINUTES", 15),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
