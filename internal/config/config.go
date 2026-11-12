// Package config loads application settings from environment variables.
//
// Every setting has a sensible default for local development, so the app can
// start with only DATABASE_URL set. Production (Render) sets the rest in the
// dashboard or render.yaml.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings.
type Config struct {
	Env     string // "development" or "production"
	Port    string
	BaseURL string // public URL, e.g. https://careers.sinapis.org — used in apply links and OAuth redirects
	OrgName string
	// Location is the time zone used to display dates (default Africa/Nairobi).
	Location *time.Location
	// PrivacyEmail is shown in the candidate privacy notice for data requests.
	PrivacyEmail string

	DatabaseURL string

	// AppSecret signs session cookies and encrypts stored OAuth refresh tokens.
	AppSecret string

	// Google sign-in (OAuth client of type "Web application").
	GoogleClientID     string
	GoogleClientSecret string
	AllowedEmailDomain string   // e.g. sinapis.org; empty = any Google account that has been invited
	AdminEmails        []string // bootstrap admins: these accounts are created as admins on first sign-in

	// AI provider. LLMProvider is "vertex", "gemini" (AI Studio API key) or "demo".
	LLMProvider        string
	LLMModel           string
	GCPProject         string
	GCPLocation        string
	GeminiAPIKey       string
	ServiceAccountJSON string // contents of a service-account key (alternative to GOOGLE_APPLICATION_CREDENTIALS path)
	ServiceAccountFile string

	// File storage: "local" (Render disk) or "gcs".
	StorageBackend string
	StorageDir     string
	GCSBucket      string

	// Screening worker.
	ScreeningWorkers int
	ScreeningDelay   time.Duration
	MaxUploadMB      int64

	Debug bool
}

// Load reads configuration from the environment.
func Load() (Config, error) {
	c := Config{
		Env:                env("APP_ENV", "development"),
		Port:               env("PORT", "8080"),
		BaseURL:            strings.TrimRight(env("BASE_URL", "http://localhost:8080"), "/"),
		OrgName:            env("ORG_NAME", "Sinapis"),
		PrivacyEmail:       env("PRIVACY_CONTACT_EMAIL", ""),
		DatabaseURL:        env("DATABASE_URL", ""),
		AppSecret:          env("APP_SECRET", ""),
		GoogleClientID:     env("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: env("GOOGLE_CLIENT_SECRET", ""),
		AllowedEmailDomain: strings.ToLower(strings.TrimSpace(env("ALLOWED_EMAIL_DOMAIN", ""))),
		AdminEmails:        splitList(env("ADMIN_EMAILS", "")),
		LLMProvider:        strings.ToLower(env("LLM_PROVIDER", "demo")),
		LLMModel:           env("LLM_MODEL", "gemini-2.5-flash"),
		GCPProject:         env("GOOGLE_CLOUD_PROJECT", ""),
		GCPLocation:        env("GOOGLE_CLOUD_LOCATION", "us-central1"),
		GeminiAPIKey:       env("GEMINI_API_KEY", ""),
		ServiceAccountJSON: env("GOOGLE_SERVICE_ACCOUNT_JSON", ""),
		ServiceAccountFile: env("GOOGLE_APPLICATION_CREDENTIALS", ""),
		StorageBackend:     strings.ToLower(env("STORAGE_BACKEND", "local")),
		StorageDir:         env("STORAGE_DIR", "./data/files"),
		GCSBucket:          env("GCS_BUCKET", ""),
		Debug:              env("DEBUG", "") == "true",
	}

	var err error
	if c.Location, err = time.LoadLocation(env("TIMEZONE", "Africa/Nairobi")); err != nil {
		return c, fmt.Errorf("TIMEZONE: %w", err)
	}
	if c.ScreeningWorkers, err = envInt("SCREENING_WORKERS", 2); err != nil {
		return c, err
	}
	delayMs, err := envInt("SCREENING_DELAY_MS", 1000)
	if err != nil {
		return c, err
	}
	c.ScreeningDelay = time.Duration(delayMs) * time.Millisecond
	maxMB, err := envInt("MAX_UPLOAD_MB", 10)
	if err != nil {
		return c, err
	}
	c.MaxUploadMB = int64(maxMB)

	return c, c.validate()
}

// IsProduction reports whether the app runs in production mode.
func (c Config) IsProduction() bool { return c.Env == "production" }

// GoogleSignInEnabled reports whether Google OAuth is configured.
func (c Config) GoogleSignInEnabled() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != ""
}

// IsAdminEmail reports whether email is one of the bootstrap admins.
func (c Config) IsAdminEmail(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, a := range c.AdminEmails {
		if a == email {
			return true
		}
	}
	return false
}

func (c Config) validate() error {
	var problems []string
	if c.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL is required")
	}
	if c.IsProduction() {
		if len(c.AppSecret) < 32 {
			problems = append(problems, "APP_SECRET must be at least 32 characters in production")
		}
		if !c.GoogleSignInEnabled() {
			problems = append(problems, "GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET are required in production")
		}
		if c.LLMProvider == "demo" {
			problems = append(problems, "LLM_PROVIDER=demo is not allowed in production (set vertex or gemini)")
		}
		if !strings.HasPrefix(c.BaseURL, "https://") {
			problems = append(problems, "BASE_URL must use https in production")
		}
	}
	switch c.LLMProvider {
	case "demo":
	case "vertex":
		if c.GCPProject == "" {
			problems = append(problems, "GOOGLE_CLOUD_PROJECT is required when LLM_PROVIDER=vertex")
		}
	case "gemini":
		if c.GeminiAPIKey == "" {
			problems = append(problems, "GEMINI_API_KEY is required when LLM_PROVIDER=gemini")
		}
	default:
		problems = append(problems, "LLM_PROVIDER must be vertex, gemini or demo")
	}
	switch c.StorageBackend {
	case "local":
	case "gcs":
		if c.GCSBucket == "" {
			problems = append(problems, "GCS_BUCKET is required when STORAGE_BACKEND=gcs")
		}
	default:
		problems = append(problems, "STORAGE_BACKEND must be local or gcs")
	}
	if len(problems) > 0 {
		return fmt.Errorf("configuration problems:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// Secret returns the app secret, with a fixed development fallback so the app
// starts locally without one (never used in production: validate requires it).
func (c Config) Secret() string {
	if c.AppSecret == "" {
		return "dev-insecure-secret-change-me-0000000000"
	}
	return c.AppSecret
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envInt(key string, def int) (int, error) {
	v := env(key, "")
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number: %w", key, err)
	}
	return n, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
