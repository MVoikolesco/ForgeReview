package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config contains the process configuration loaded from environment variables.
// API and worker processes share this structure and validate it before startup.
type Config struct {
	AppMode     string
	Port        string
	ServiceName string
	Version     string
	LogLevel    string

	DatabaseDriver string
	DatabaseDSN    string

	AdminUsername string
	AdminPassword string

	GiteaURL         string
	GiteaToken       string
	GiteaBotUsername string

	RedisAddr     string
	RedisPassword string
	RedisStream   string
	RedisGroup    string
	RedisConsumer string
	RedisDB       int

	ReviewPromptConfigPath string
	DiffLogDir             string

	ReviewMaxBlockChars         int
	ReviewMaxFilesPerBlock      int
	ReviewRequestTimeoutSeconds int
}

// Load reads supported environment variables and returns a Config populated
// with operational defaults when values are absent.
func Load() Config {
	return Config{
		AppMode:     env("APP_MODE", "api"),
		Port:        env("PORT", "8080"),
		ServiceName: env("SERVICE_NAME", "forgereview"),
		Version:     env("VERSION", "1.0.0"),
		LogLevel:    env("LOG_LEVEL", "info"),

		DatabaseDriver: env("DATABASE_DRIVER", "sqlite"),
		DatabaseDSN:    env("DATABASE_DSN", "./data/forgereview.db"),

		AdminUsername: env("ADMIN_USERNAME", "admin"),
		AdminPassword: env("ADMIN_PASSWORD", "change-me"),

		GiteaURL:         env("GITEA_URL", ""),
		GiteaToken:       env("GITEA_TOKEN", ""),
		GiteaBotUsername: env("GITEA_BOT_USERNAME", "ia-reviewer"),

		RedisAddr:     env("REDIS_ADDR", "redis:6379"),
		RedisPassword: env("REDIS_PASSWORD", ""),
		RedisStream:   env("REDIS_STREAM", "gitea:review-jobs"),
		RedisGroup:    env("REDIS_GROUP", "gitea-reviewers"),
		RedisConsumer: env("REDIS_CONSUMER", "worker-1"),
		RedisDB:       envInt("REDIS_DB", 0),

		ReviewPromptConfigPath: env("REVIEW_PROMPT_CONFIG_PATH", "./config/review-prompts.yaml"),
		DiffLogDir:             env("DIFF_LOG_DIR", "./data/reviews"),

		ReviewMaxBlockChars:         envInt("REVIEW_MAX_BLOCK_CHARS", 2000),
		ReviewMaxFilesPerBlock:      envInt("REVIEW_MAX_FILES_PER_BLOCK", 2),
		ReviewRequestTimeoutSeconds: envInt("REVIEW_REQUEST_TIMEOUT_SECONDS", 900),
	}
}

// Validate checks mode, persistence, Redis, and API authentication settings.
// It returns an error describing the first invalid requirement.
func (c Config) Validate() error {
	if c.AppMode != "api" && c.AppMode != "worker" {
		return fmt.Errorf("APP_MODE must be api or worker")
	}
	if c.DatabaseDriver != "sqlite" {
		return fmt.Errorf("DATABASE_DRIVER must be sqlite")
	}
	if strings.TrimSpace(c.DatabaseDSN) == "" || strings.TrimSpace(c.RedisAddr) == "" {
		return fmt.Errorf("database and redis configuration are required")
	}
	if c.AppMode == "api" && (c.AdminUsername == "" || c.AdminPassword == "") {
		return fmt.Errorf("admin credentials are required")
	}
	return nil
}

// Address returns the TCP listen address derived from Config.Port.
func (c Config) Address() string {
	return ":" + c.Port
}

// Debug reports whether the configured log level enables Gin debug mode.
func (c Config) Debug() bool {
	return strings.EqualFold(c.LogLevel, "debug")
}

// env returns a trimmed environment value or the supplied fallback.
func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// envInt parses an integer environment value or returns the supplied fallback.
func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(env(key, ""))
	if err != nil {
		return fallback
	}
	return value
}
