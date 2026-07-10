package promptconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptResolverLoadsBaseStackAndCategoryPrompts(t *testing.T) {
	configPath := writePromptFixture(t, validConfigYAML())
	resolver := NewPromptResolver(configPath)

	resolved, err := resolver.ResolvePartialPrompt(context.Background(), "Qualyagro", "portal", []string{"app/Services/OrderService.php"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(resolved.Content, "BASE PARTIAL") ||
		!strings.Contains(resolved.Content, "LARAVEL STACK") ||
		!strings.Contains(resolved.Content, "LARAVEL SERVICES") {
		t.Fatalf("expected base, stack and category prompts, got %q", resolved.Content)
	}

	if strings.Join(resolved.Stacks, ",") != "laravel" {
		t.Fatalf("expected laravel stack, got %#v", resolved.Stacks)
	}

	if strings.Join(resolved.Categories, ",") != "laravel/services" {
		t.Fatalf("expected laravel services category, got %#v", resolved.Categories)
	}
}

func TestPromptResolverUsesDefaultProjectConfig(t *testing.T) {
	configPath := writePromptFixture(t, validConfigYAML())
	resolver := NewPromptResolver(configPath)

	resolved, err := resolver.ResolvePartialPrompt(context.Background(), "unknown", "repo", []string{"internal/app.go"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(resolved.Content, "GO STACK") {
		t.Fatalf("expected default go stack, got %q", resolved.Content)
	}
}

func TestPromptResolverDoesNotLoadUnmatchedStack(t *testing.T) {
	configPath := writePromptFixture(t, validConfigYAML())
	resolver := NewPromptResolver(configPath)

	resolved, err := resolver.ResolvePartialPrompt(context.Background(), "Qualyagro", "portal", []string{"resources/views/index.blade.php"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if strings.Contains(resolved.Content, "REACT STACK") {
		t.Fatalf("did not expect react stack for blade file, got %q", resolved.Content)
	}
}

func TestPromptResolverUsesCIForCodeIgniterAndCICDForWorkflows(t *testing.T) {
	configPath := writePromptFixture(t, validConfigYAML())
	resolver := NewPromptResolver(configPath)

	codeIgniterPrompt, err := resolver.ResolvePartialPrompt(context.Background(), "Qualyagro", "legacy", []string{"app/Models/UserModel.php"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(codeIgniterPrompt.Content, "CODEIGNITER STACK") ||
		!strings.Contains(codeIgniterPrompt.Content, "CODEIGNITER MODELS") ||
		strings.Contains(codeIgniterPrompt.Content, "CICD STACK") {
		t.Fatalf("expected CodeIgniter prompts only, got %q", codeIgniterPrompt.Content)
	}

	workflowPrompt, err := resolver.ResolvePartialPrompt(context.Background(), "mvoikolesco", "teste-bot", []string{".gitea/workflows/deploy.yml"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(workflowPrompt.Content, "CICD STACK") ||
		!strings.Contains(workflowPrompt.Content, "CICD WORKFLOWS") ||
		strings.Contains(workflowPrompt.Content, "CODEIGNITER STACK") {
		t.Fatalf("expected CICD prompts only, got %q", workflowPrompt.Content)
	}
}

func TestPromptResolverShouldIncludeDocs(t *testing.T) {
	configPath := writePromptFixture(t, validConfigYAML())
	resolver := NewPromptResolver(configPath)

	includeDocs, err := resolver.ShouldIncludeDocs(context.Background(), "Qualyagro", "portal")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !includeDocs {
		t.Fatal("expected project include_docs override")
	}

	includeDocs, err = resolver.ShouldIncludeDocs(context.Background(), "unknown", "repo")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if includeDocs {
		t.Fatal("expected default include_docs false")
	}
}

func TestPromptResolverReturnsInvalidYAMLError(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "review-prompts.yaml")
	if err := os.WriteFile(configPath, []byte("default: [invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := NewPromptResolver(configPath).Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), "YAML de prompts invalido") {
		t.Fatalf("expected invalid yaml error, got %v", err)
	}
}

func TestPromptResolverReturnsMissingPromptError(t *testing.T) {
	configPath := writePromptFixture(t, strings.ReplaceAll(validConfigYAML(), "../prompts/base/review_partial.md", "../prompts/base/missing.md"))

	_, err := NewPromptResolver(configPath).ResolvePartialPrompt(context.Background(), "unknown", "repo", []string{"internal/app.go"})
	if err == nil || !strings.Contains(err.Error(), "erro ao ler prompt") {
		t.Fatalf("expected missing prompt error, got %v", err)
	}
}

func TestMatchFilePattern(t *testing.T) {
	tests := []struct {
		pattern string
		file    string
	}{
		{"*.go", "internal/app.go"},
		{"go.mod", "go.mod"},
		{"database/migrations/*.php", "database/migrations/2026_01_01_create_users.php"},
		{"Dockerfile", "Dockerfile"},
		{".gitea/workflows/*.yml", ".gitea/workflows/deploy.yml"},
	}

	for _, tt := range tests {
		if !MatchFilePattern(tt.pattern, tt.file) {
			t.Fatalf("expected pattern %q to match %q", tt.pattern, tt.file)
		}
	}
}

func writePromptFixture(t *testing.T, configContent string) string {
	t.Helper()

	dir := t.TempDir()
	files := map[string]string{
		"config/review-prompts.yaml":         configContent,
		"prompts/base/review_partial.md":     "BASE PARTIAL",
		"prompts/base/review_final.md":       "BASE FINAL",
		"prompts/stacks/go.md":               "GO STACK",
		"prompts/stacks/laravel.md":          "LARAVEL STACK",
		"prompts/stacks/laravel/services.md": "LARAVEL SERVICES",
		"prompts/stacks/react.md":            "REACT STACK",
		"prompts/stacks/ci.md":               "CODEIGNITER STACK",
		"prompts/stacks/ci/models.md":        "CODEIGNITER MODELS",
		"prompts/stacks/cicd.md":             "CICD STACK",
		"prompts/stacks/cicd/workflows.md":   "CICD WORKFLOWS",
	}

	for file, content := range files {
		path := filepath.Join(dir, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return filepath.Join(dir, "config", "review-prompts.yaml")
}

func validConfigYAML() string {
	return `default:
  partial_prompt: ../prompts/base/review_partial.md
  final_prompt: ../prompts/base/review_final.md
  stacks:
    - go
  include_docs: false

projects:
  teste-bot:
    owner: mvoikolesco
    repo: teste-bot
    stacks:
      - go
      - cicd
    include_docs: false

  portal:
    owner: Qualyagro
    repo: portal
    stacks:
      - laravel
      - react
    include_docs: true

  legacy:
    owner: Qualyagro
    repo: legacy
    stacks:
      - ci
    include_docs: false

stacks:
  go:
    prompt: ../prompts/stacks/go.md
    file_patterns:
      - "*.go"
  laravel:
    prompt: ../prompts/stacks/laravel.md
    file_patterns:
      - "*.php"
    categories:
      services:
        prompt: ../prompts/stacks/laravel/services.md
        file_patterns:
          - "app/Services/*.php"
  react:
    prompt: ../prompts/stacks/react.md
    file_patterns:
      - "*.tsx"
  ci:
    prompt: ../prompts/stacks/ci.md
    file_patterns:
      - "*.php"
    categories:
      models:
        prompt: ../prompts/stacks/ci/models.md
        file_patterns:
          - "app/Models/*.php"
  cicd:
    prompt: ../prompts/stacks/cicd.md
    file_patterns:
      - ".gitea/workflows/*.yml"
    categories:
      workflows:
        prompt: ../prompts/stacks/cicd/workflows.md
        file_patterns:
          - ".gitea/workflows/*.yml"
`
}
