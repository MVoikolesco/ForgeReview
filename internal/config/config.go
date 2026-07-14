package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AppMode                string
	Port                   string
	ServiceName            string
	Version                string
	LogLevel               string
	GiteaBotUsername       string
	GiteaURL               string
	GiteaToken             string
	RedisAddr              string
	RedisPassword          string
	RedisDB                int
	RedisStream            string
	RedisGroup             string
	RedisConsumer          string
	DiffLogDir             string
	ReviewPromptConfigPath string

	OllamaURL            string
	OllamaModel          string
	OllamaTemperature    float64
	OllamaTopP           float64
	OllamaRepeatPenalty  float64
	OllamaNumCtx         int
	OllamaNumThread      int
	OllamaNumPredict     int
	OllamaKeepAlive      string
	OllamaTimeoutSeconds int

	ReviewMaxBlockChars    int
	ReviewMaxFilesPerBlock int
	ReviewConcurrency      int
	ReviewFinalRetries     int
	ReviewPublishManual    bool
	ReviewAllowRejection   bool
}

func Load() Config {
	reviewMaxBlockChars := getEnvPositiveInt("REVIEW_MAX_BLOCK_CHARS", getEnvPositiveInt("REVIEW_MAX_DIFF_CHARS", 4000))

	return Config{
		AppMode:                getEnv("APP_MODE", "api"),
		Port:                   getEnv("PORT", "8080"),
		ServiceName:            getEnv("SERVICE_NAME", "gitea-ai-reviewer"),
		Version:                getEnv("VERSION", "0.1.0"),
		LogLevel:               getEnv("LOG_LEVEL", "debug"),
		GiteaBotUsername:       getEnv("GITEA_BOT_USERNAME", "ia-reviewer"),
		GiteaURL:               getEnv("GITEA_URL", ""),
		GiteaToken:             getEnv("GITEA_TOKEN", ""),
		RedisAddr:              getEnv("REDIS_ADDR", "redis:6379"),
		RedisPassword:          getEnv("REDIS_PASSWORD", ""),
		RedisDB:                getEnvInt("REDIS_DB", 0),
		RedisStream:            getEnv("REDIS_STREAM", "gitea:review-jobs"),
		RedisGroup:             getEnv("REDIS_GROUP", "gitea-reviewers"),
		RedisConsumer:          getEnv("REDIS_CONSUMER", "worker-1"),
		DiffLogDir:             getEnv("DIFF_LOG_DIR", "/tmp/gitea-ai-reviewer-diffs"),
		ReviewPromptConfigPath: getEnv("REVIEW_PROMPT_CONFIG_PATH", "./config/review-prompts.yaml"),

		OllamaURL:            getEnv("OLLAMA_URL", "http://localhost:11434"),
		OllamaModel:          getEnv("OLLAMA_MODEL", "deepseek-coder:6.7b"),
		OllamaTemperature:    getEnvFloat("OLLAMA_TEMPERATURE", 0.1),
		OllamaTopP:           getEnvFloat("OLLAMA_TOP_P", 0.85),
		OllamaRepeatPenalty:  getEnvFloat("OLLAMA_REPEAT_PENALTY", 1.1),
		OllamaNumCtx:         getEnvPositiveInt("OLLAMA_NUM_CTX", 4096),
		OllamaNumThread:      getEnvPositiveInt("OLLAMA_NUM_THREADS", 2),
		OllamaNumPredict:     getEnvPositiveInt("OLLAMA_NUM_PREDICT", 400),
		OllamaKeepAlive:      getEnv("OLLAMA_KEEP_ALIVE", "5m"),
		OllamaTimeoutSeconds: getEnvPositiveInt("OLLAMA_TIMEOUT_SECONDS", 900),

		ReviewMaxBlockChars:    reviewMaxBlockChars,
		ReviewMaxFilesPerBlock: getEnvPositiveInt("REVIEW_MAX_FILES_PER_BLOCK", 2),
		ReviewConcurrency:      getEnvPositiveInt("REVIEW_CONCURRENCY", 1),
		ReviewFinalRetries:     getEnvPositiveInt("REVIEW_FINAL_RETRIES", 5),
		ReviewPublishManual:    getEnvBool("REVIEW_PUBLISH_MANUAL_REVIEWS", false),
		ReviewAllowRejection:   getEnvBool("REVIEW_ALLOW_AUTONOMOUS_REJECTION", false),
	}
}

func (c Config) Validate() error {
	if c.OllamaURL == "" {
		return fmt.Errorf("OLLAMA_URL nao pode estar vazio")
	}

	if c.OllamaModel == "" {
		return fmt.Errorf("OLLAMA_MODEL nao pode estar vazio")
	}

	if c.ReviewMaxBlockChars <= 0 {
		return fmt.Errorf("REVIEW_MAX_BLOCK_CHARS deve ser maior que 0")
	}

	if c.ReviewMaxFilesPerBlock <= 0 {
		return fmt.Errorf("REVIEW_MAX_FILES_PER_BLOCK deve ser maior que 0")
	}

	if c.ReviewConcurrency <= 0 {
		return fmt.Errorf("REVIEW_CONCURRENCY deve ser maior que 0")
	}

	if c.ReviewFinalRetries <= 0 {
		return fmt.Errorf("REVIEW_FINAL_RETRIES deve ser maior que 0")
	}

	if c.OllamaTimeoutSeconds <= 0 {
		return fmt.Errorf("OLLAMA_TIMEOUT_SECONDS deve ser maior que 0")
	}

	if c.OllamaNumThread <= 0 {
		return fmt.Errorf("OLLAMA_NUM_THREADS deve ser maior que 0")
	}

	return nil
}

func (c Config) DebugEnabled() bool {
	return c.LogLevel == "debug"
}

func (c Config) Address() string {
	return ":" + c.Port
}

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

func getEnvPositiveInt(key string, fallback int) int {
	parsed := getEnvInt(key, fallback)
	if parsed <= 0 {
		return fallback
	}

	return parsed
}

func getEnvFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}

	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}
