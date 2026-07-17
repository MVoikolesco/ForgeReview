package config

import "testing"

func TestLoadReadsReviewPromptConfigPath(t *testing.T) {
	t.Setenv("OLLAMA_URL", "http://legacy")
	t.Setenv("REVIEW_MAX_BLOCK_CHARS", "1")
	t.Setenv("REVIEW_PROMPT_CONFIG_PATH", "/app/config/review-prompts.yaml")
	c := Load()
	if c.ReviewPromptConfigPath != "/app/config/review-prompts.yaml" {
		t.Fatal("unexpected prompt metadata path")
	}
}
func TestValidateWorkerAllowsDatabaseGiteaConfiguration(t *testing.T) {
	c := Load()
	c.AppMode = "worker"
	c.GiteaURL = ""
	c.GiteaToken = ""
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestValidateAPIBootstrap(t *testing.T) {
	c := Load()
	c.AppMode = "api"
	c.AdminUsername = "admin"
	c.AdminPassword = "secret"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
