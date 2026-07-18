package promptconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptResolverLoadsAllEditablePromptFiles(t *testing.T) {
	resolver := NewPromptResolver(writePromptFixture(t, validConfigYAML()))
	prompts, err := resolver.ResolvePrompts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if prompts.TechnicalReview != "TECHNICAL" || prompts.SecurityPerformance != "SECURITY" || prompts.ImportDivergence != "IMPORTS" || prompts.FinalResponse != "FINAL" {
		t.Fatalf("unexpected prompts %#v", prompts)
	}
}

func TestPromptResolverFailsForMissingOrEmptyPromptFiles(t *testing.T) {
	for _, tc := range []struct{ name, content string }{{"missing", ""}, {"empty", ""}} {
		t.Run(tc.name, func(t *testing.T) {
			configPath := writePromptFixture(t, validConfigYAML())
			path := filepath.Join(filepath.Dir(filepath.Dir(configPath)), "prompts", "technical.md")
			if tc.name == "missing" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := NewPromptResolver(configPath).Load(context.Background())
			if err == nil || !strings.Contains(err.Error(), "prompt") {
				t.Fatalf("expected explicit prompt error, got %v", err)
			}
		})
	}
}

func TestPromptResolverRejectsMissingPromptConfiguration(t *testing.T) {
	configPath := writePromptFixture(t, strings.Replace(validConfigYAML(), "technical_review: ../prompts/technical.md", "technical_review: ", 1))
	_, err := NewPromptResolver(configPath).Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), "prompts.technical_review nao pode estar vazio") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestPromptResolverRejectsLegacyStacks(t *testing.T) {
	configPath := writePromptFixture(t, validConfigYAML()+"\nstacks:\n  go: {}\n")
	_, err := NewPromptResolver(configPath).Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), "field stacks not found") {
		t.Fatalf("expected legacy stack configuration to fail, got %v", err)
	}
}

func TestPromptResolverResolvesGlobalBlockFilter(t *testing.T) {
	filter, err := NewPromptResolver(writePromptFixture(t, validConfigYAML())).ResolveBlockFilter(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if filter.IncludeDocs || strings.Join(filter.IgnoreFilePatterns, ",") != "*.lock,dist/**" {
		t.Fatalf("unexpected filter %#v", filter)
	}
}

func TestMatchFilePattern(t *testing.T) {
	if !MatchFilePattern("src/**/*.tsx", "src/components/Button.tsx") {
		t.Fatal("expected glob match")
	}
}

func writePromptFixture(t *testing.T, config string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{"config/review-prompts.yaml": config, "prompts/technical.md": "TECHNICAL", "prompts/security.md": "SECURITY", "prompts/imports.md": "IMPORTS", "prompts/final.md": "FINAL"} {
		path := filepath.Join(dir, name)
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
	return `prompts:
  technical_review: ../prompts/technical.md
  security_performance: ../prompts/security.md
  import_divergence: ../prompts/imports.md
  final_response: ../prompts/final.md
filters:
  include_docs: false
  ignore_file_patterns: ["*.lock", "dist/**"]
`
}
