package config

import "testing"

func TestLoadUsesReviewMaxBlockCharsFallbackFromOldName(t *testing.T) {
	t.Setenv("REVIEW_MAX_BLOCK_CHARS", "")
	t.Setenv("REVIEW_MAX_DIFF_CHARS", "1234")

	cfg := Load()

	if cfg.ReviewMaxBlockChars != 1234 {
		t.Fatalf("expected REVIEW_MAX_DIFF_CHARS fallback, got %d", cfg.ReviewMaxBlockChars)
	}
}

func TestLoadUsesLightweightDefaults(t *testing.T) {
	t.Setenv("OLLAMA_TIMEOUT_SECONDS", "")
	t.Setenv("OLLAMA_NUM_PREDICT", "")
	t.Setenv("OLLAMA_NUM_THREADS", "")
	t.Setenv("REVIEW_MAX_BLOCK_CHARS", "")
	t.Setenv("REVIEW_MAX_DIFF_CHARS", "")
	t.Setenv("REVIEW_MAX_FILES_PER_BLOCK", "")
	t.Setenv("REVIEW_FINAL_RETRIES", "")
	t.Setenv("REVIEW_PROMPT_CONFIG_PATH", "")

	cfg := Load()

	if cfg.OllamaTimeoutSeconds != 900 {
		t.Fatalf("expected default ollama timeout 900, got %d", cfg.OllamaTimeoutSeconds)
	}

	if cfg.OllamaNumPredict != 400 {
		t.Fatalf("expected default num_predict 400, got %d", cfg.OllamaNumPredict)
	}

	if cfg.OllamaNumThread != 2 {
		t.Fatalf("expected default num_thread 2, got %d", cfg.OllamaNumThread)
	}

	if cfg.ReviewMaxBlockChars != 4000 {
		t.Fatalf("expected default max block chars 4000, got %d", cfg.ReviewMaxBlockChars)
	}

	if cfg.ReviewMaxFilesPerBlock != 2 {
		t.Fatalf("expected default max files per block 2, got %d", cfg.ReviewMaxFilesPerBlock)
	}

	if cfg.ReviewFinalRetries != 5 {
		t.Fatalf("expected default final retries 5, got %d", cfg.ReviewFinalRetries)
	}

	if cfg.ReviewPromptConfigPath != "./config/review-prompts.yaml" {
		t.Fatalf("expected default prompt config path, got %q", cfg.ReviewPromptConfigPath)
	}
}

func TestLoadPrefersReviewMaxBlockChars(t *testing.T) {
	t.Setenv("REVIEW_MAX_DIFF_CHARS", "1234")
	t.Setenv("REVIEW_MAX_BLOCK_CHARS", "5678")

	cfg := Load()

	if cfg.ReviewMaxBlockChars != 5678 {
		t.Fatalf("expected REVIEW_MAX_BLOCK_CHARS preference, got %d", cfg.ReviewMaxBlockChars)
	}
}

func TestLoadReadsOllamaTimeoutSeconds(t *testing.T) {
	t.Setenv("OLLAMA_TIMEOUT_SECONDS", "321")

	cfg := Load()

	if cfg.OllamaTimeoutSeconds != 321 {
		t.Fatalf("expected configured timeout, got %d", cfg.OllamaTimeoutSeconds)
	}
}

func TestLoadReadsReviewFinalRetries(t *testing.T) {
	t.Setenv("REVIEW_FINAL_RETRIES", "7")

	cfg := Load()

	if cfg.ReviewFinalRetries != 7 {
		t.Fatalf("expected configured final retries, got %d", cfg.ReviewFinalRetries)
	}
}

func TestLoadReadsReviewPromptConfigPath(t *testing.T) {
	t.Setenv("REVIEW_PROMPT_CONFIG_PATH", "./custom/prompts.yaml")

	cfg := Load()

	if cfg.ReviewPromptConfigPath != "./custom/prompts.yaml" {
		t.Fatalf("expected custom prompt config path, got %q", cfg.ReviewPromptConfigPath)
	}
}

func TestLoadUsesDefaultsForInvalidPositiveIntegers(t *testing.T) {
	t.Setenv("OLLAMA_TIMEOUT_SECONDS", "invalid")
	t.Setenv("OLLAMA_NUM_PREDICT", "-1")
	t.Setenv("OLLAMA_NUM_THREADS", "0")
	t.Setenv("REVIEW_MAX_BLOCK_CHARS", "0")
	t.Setenv("REVIEW_MAX_DIFF_CHARS", "")
	t.Setenv("REVIEW_MAX_FILES_PER_BLOCK", "-2")

	cfg := Load()

	if cfg.OllamaTimeoutSeconds != 900 {
		t.Fatalf("expected default timeout, got %d", cfg.OllamaTimeoutSeconds)
	}

	if cfg.OllamaNumPredict != 400 {
		t.Fatalf("expected default num predict, got %d", cfg.OllamaNumPredict)
	}

	if cfg.OllamaNumThread != 2 {
		t.Fatalf("expected default num thread, got %d", cfg.OllamaNumThread)
	}

	if cfg.ReviewMaxBlockChars != 4000 {
		t.Fatalf("expected default max block chars, got %d", cfg.ReviewMaxBlockChars)
	}

	if cfg.ReviewMaxFilesPerBlock != 2 {
		t.Fatalf("expected default max files per block, got %d", cfg.ReviewMaxFilesPerBlock)
	}
}

func TestValidateReturnsErrorForInvalidReviewConfig(t *testing.T) {
	cfg := Load()
	cfg.ReviewMaxBlockChars = 0

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
