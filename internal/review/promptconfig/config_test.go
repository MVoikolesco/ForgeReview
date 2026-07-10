package promptconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptResolverLoadsMatchingStackPromptsAutomatically(t *testing.T) {
	configPath := writePromptFixture(t, validConfigYAML())
	resolver := NewPromptResolver(configPath)

	resolved, err := resolver.ResolvePartialPrompt(context.Background(), "AnyOwner", "any-repo", []string{"app/Services/OrderService.php"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	for _, expected := range []string{"BASE PARTIAL", "LARAVEL STACK", "PHP STACK"} {
		if !strings.Contains(resolved.Content, expected) {
			t.Fatalf("expected resolved prompt to contain %q, got %q", expected, resolved.Content)
		}
	}

	if strings.Join(resolved.Stacks, ",") != "laravel,php" {
		t.Fatalf("expected laravel and php stacks, got %#v", resolved.Stacks)
	}
}

func TestPromptResolverDoesNotLoadUnmatchedStack(t *testing.T) {
	configPath := writePromptFixture(t, validConfigYAML())
	resolver := NewPromptResolver(configPath)

	resolved, err := resolver.ResolvePartialPrompt(context.Background(), "AnyOwner", "any-repo", []string{"resources/views/index.blade.php"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if strings.Contains(resolved.Content, "REACT STACK") || strings.Contains(resolved.Content, "GO STACK") {
		t.Fatalf("did not expect unmatched stack prompts, got %q", resolved.Content)
	}
}

func TestPromptResolverLoadsFrameworkSpecificStacks(t *testing.T) {
	configPath := writePromptFixture(t, validConfigYAML())
	resolver := NewPromptResolver(configPath)

	codeIgniterPrompt, err := resolver.ResolvePartialPrompt(context.Background(), "AnyOwner", "legacy", []string{"app/Models/UserModel.php"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(codeIgniterPrompt.Content, "CODEIGNITER STACK") ||
		!strings.Contains(codeIgniterPrompt.Content, "PHP STACK") ||
		strings.Contains(codeIgniterPrompt.Content, "CICD STACK") {
		t.Fatalf("expected CodeIgniter and PHP prompts only, got %q", codeIgniterPrompt.Content)
	}

	workflowPrompt, err := resolver.ResolvePartialPrompt(context.Background(), "AnyOwner", "any-repo", []string{".gitea/workflows/deploy.yml"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(workflowPrompt.Content, "CICD STACK") ||
		strings.Contains(workflowPrompt.Content, "CODEIGNITER STACK") {
		t.Fatalf("expected CICD prompt only, got %q", workflowPrompt.Content)
	}
}

func TestPromptResolverUsesDefaultBasePromptsForAnyRepo(t *testing.T) {
	configPath := writePromptFixture(t, validConfigYAML())
	resolver := NewPromptResolver(configPath)

	partial, err := resolver.ResolvePartialPrompt(context.Background(), "Unknown", "repo", []string{"internal/app.go"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(partial.Content, "BASE PARTIAL") || !strings.Contains(partial.Content, "GO STACK") {
		t.Fatalf("expected base and Go prompts, got %q", partial.Content)
	}

	final, err := resolver.ResolveFinalPrompt(context.Background(), "Unknown", "repo")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if final.Content != "BASE FINAL" {
		t.Fatalf("expected base final prompt, got %q", final.Content)
	}
}

func TestPromptResolverResolvesGlobalBlockFilter(t *testing.T) {
	configPath := writePromptFixture(t, validConfigYAML())
	resolver := NewPromptResolver(configPath)

	filter, err := resolver.ResolveBlockFilter(context.Background(), "AnyOwner", "any-repo")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if filter.IncludeDocs {
		t.Fatal("expected default include_docs false")
	}
	if strings.Join(filter.IgnoreFilePatterns, ",") != "*.lock,dist/**" {
		t.Fatalf("unexpected ignore patterns %#v", filter.IgnoreFilePatterns)
	}
	if strings.Join(filter.DocFilePatterns, ",") != "*.md,docs/**" {
		t.Fatalf("unexpected doc patterns %#v", filter.DocFilePatterns)
	}
	if strings.Join(filter.ForceIncludeFilePatterns, ",") != ".env.example" {
		t.Fatalf("unexpected force include patterns %#v", filter.ForceIncludeFilePatterns)
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

	_, err := NewPromptResolver(configPath).ResolvePartialPrompt(context.Background(), "AnyOwner", "any-repo", []string{"internal/app.go"})
	if err == nil || !strings.Contains(err.Error(), "erro ao ler prompt") {
		t.Fatalf("expected missing prompt error, got %v", err)
	}
}

func TestPromptResolverValidatesStackConfig(t *testing.T) {
	configPath := writePromptFixture(t, strings.ReplaceAll(validConfigYAML(), "prompt: ../prompts/stacks/go.md", "prompt: "))

	_, err := NewPromptResolver(configPath).Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), "stacks.go.prompt nao pode estar vazio") {
		t.Fatalf("expected invalid stack prompt error, got %v", err)
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
		{"app/**/*.php", "app/Services/OrderService.php"},
		{"src/**/*.tsx", "src/components/Button.tsx"},
		{"**/dist/**", "frontend/dist/app.js"},
		{"Dockerfile", "Dockerfile"},
		{".gitea/workflows/**/*.yml", ".gitea/workflows/deploy.yml"},
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
		"config/review-prompts.yaml":       configContent,
		"prompts/base/review_partial.md":   "BASE PARTIAL",
		"prompts/base/review_final.md":     "BASE FINAL",
		"prompts/stacks/go.md":             "GO STACK",
		"prompts/stacks/laravel.md":        "LARAVEL STACK",
		"prompts/stacks/php.md":            "PHP STACK",
		"prompts/stacks/react.md":          "REACT STACK",
		"prompts/stacks/codeigniter.md":    "CODEIGNITER STACK",
		"prompts/stacks/cicd.md":           "CICD STACK",
		"prompts/stacks/empty-patterns.md": "EMPTY PATTERNS",
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
  include_docs: false
  doc_file_patterns:
    - "*.md"
    - "docs/**"
  ignore_file_patterns:
    - "*.lock"
    - "dist/**"
  force_include_file_patterns:
    - ".env.example"

stacks:
  codeigniter:
    prompt: ../prompts/stacks/codeigniter.md
    file_patterns:
      - "app/Config/**/*.php"
      - "app/Controllers/**/*.php"
      - "app/Models/**/*.php"
  go:
    prompt: ../prompts/stacks/go.md
    file_patterns:
      - "*.go"
      - "go.mod"
  laravel:
    prompt: ../prompts/stacks/laravel.md
    file_patterns:
      - "artisan"
      - "app/**/*.php"
      - "routes/**/*.php"
  php:
    prompt: ../prompts/stacks/php.md
    file_patterns:
      - "*.php"
      - "composer.json"
  react:
    prompt: ../prompts/stacks/react.md
    file_patterns:
      - "*.tsx"
      - "*.jsx"
  cicd:
    prompt: ../prompts/stacks/cicd.md
    file_patterns:
      - ".gitea/workflows/**/*.yml"
`
}
