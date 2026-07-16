package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AppMode, Port, ServiceName, Version, LogLevel             string
	GiteaBotUsername, GiteaURL, GiteaToken                    string
	RedisAddr, RedisPassword                                  string
	RedisDB                                                   int
	RedisStream, RedisGroup, RedisConsumer                    string
	DiffLogDir, ReviewPromptConfigPath                        string
	DatabaseDriver, DatabaseDSN, AdminUsername, AdminPassword string
}

func Load() Config {
	return Config{
		AppMode: getEnv("APP_MODE", "api"), Port: getEnv("PORT", "8080"), ServiceName: getEnv("SERVICE_NAME", "gitea-ai-reviewer"), Version: getEnv("VERSION", "0.1.0"), LogLevel: getEnv("LOG_LEVEL", "info"),
		GiteaBotUsername: getEnv("GITEA_BOT_USERNAME", "ia-reviewer"), GiteaURL: getEnv("GITEA_URL", ""), GiteaToken: getEnv("GITEA_TOKEN", ""),
		RedisAddr: getEnv("REDIS_ADDR", "redis:6379"), RedisPassword: getEnv("REDIS_PASSWORD", ""), RedisDB: getEnvInt("REDIS_DB", 0), RedisStream: getEnv("REDIS_STREAM", "gitea:review-jobs"), RedisGroup: getEnv("REDIS_GROUP", "gitea-reviewers"), RedisConsumer: getEnv("REDIS_CONSUMER", "worker-1"),
		DiffLogDir: getEnv("DIFF_LOG_DIR", "/logs/diffs"), ReviewPromptConfigPath: "./config/review-prompts.yaml", DatabaseDriver: getEnv("DATABASE_DRIVER", "sqlite"), DatabaseDSN: getEnv("DATABASE_DSN", "./data/forgereview.db"), AdminUsername: getEnv("ADMIN_USERNAME", "admin"), AdminPassword: getEnv("ADMIN_PASSWORD", "change-me"),
	}
}
func (c Config) Validate() error {
	if c.AppMode != "api" && c.AppMode != "worker" {
		return fmt.Errorf("APP_MODE must be api or worker")
	}
	if c.DatabaseDriver != "sqlite" {
		return fmt.Errorf("DATABASE_DRIVER must be sqlite")
	}
	if strings.TrimSpace(c.DatabaseDSN) == "" {
		return fmt.Errorf("DATABASE_DSN is required")
	}
	if c.RedisAddr == "" {
		return fmt.Errorf("REDIS_ADDR is required")
	}
	if c.AppMode == "api" && (c.AdminUsername == "" || c.AdminPassword == "") {
		return fmt.Errorf("ADMIN_USERNAME and ADMIN_PASSWORD are required")
	}
	if c.AppMode == "worker" && (c.GiteaURL == "" || c.GiteaToken == "") {
		return fmt.Errorf("GITEA_URL and GITEA_TOKEN are required for worker")
	}
	return nil
}
func (c Config) DebugEnabled() bool { return c.LogLevel == "debug" }
func (c Config) Address() string    { return ":" + c.Port }
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
