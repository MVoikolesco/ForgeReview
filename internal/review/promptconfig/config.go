package promptconfig

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ReviewPromptConfig struct {
	Prompts PromptFilesConfig `yaml:"prompts"`
	Filters FilterConfig      `yaml:"filters"`
}

type PromptFilesConfig struct {
	TechnicalReview     string `yaml:"technical_review"`
	SecurityPerformance string `yaml:"security_performance"`
	ImportDivergence    string `yaml:"import_divergence"`
	FinalResponse       string `yaml:"final_response"`
}

type FilterConfig struct {
	IncludeDocs              bool     `yaml:"include_docs"`
	DocFilePatterns          []string `yaml:"doc_file_patterns"`
	IgnoreFilePatterns       []string `yaml:"ignore_file_patterns"`
	ForceIncludeFilePatterns []string `yaml:"force_include_file_patterns"`
}

type PromptSet struct {
	TechnicalReview     string
	SecurityPerformance string
	ImportDivergence    string
	FinalResponse       string
}

type PromptResolver struct{ ConfigPath string }

func NewPromptResolver(configPath string) PromptResolver {
	return PromptResolver{ConfigPath: configPath}
}

func (r PromptResolver) ResolvePrompts(ctx context.Context) (PromptSet, error) {
	cfg, baseDir, err := r.load(ctx)
	if err != nil {
		return PromptSet{}, err
	}
	return loadPromptSet(baseDir, cfg.Prompts)
}

func loadPromptSet(baseDir string, files PromptFilesConfig) (PromptSet, error) {
	technical, err := readPromptFile(baseDir, files.TechnicalReview)
	if err != nil {
		return PromptSet{}, err
	}
	security, err := readPromptFile(baseDir, files.SecurityPerformance)
	if err != nil {
		return PromptSet{}, err
	}
	imports, err := readPromptFile(baseDir, files.ImportDivergence)
	if err != nil {
		return PromptSet{}, err
	}
	final, err := readPromptFile(baseDir, files.FinalResponse)
	if err != nil {
		return PromptSet{}, err
	}
	return PromptSet{TechnicalReview: technical, SecurityPerformance: security, ImportDivergence: imports, FinalResponse: final}, nil
}

func (r PromptResolver) ResolveBlockFilter(ctx context.Context, owner, repo string) (ResolvedBlockFilter, error) {
	cfg, _, err := r.load(ctx)
	if err != nil {
		return ResolvedBlockFilter{}, err
	}
	return ResolvedBlockFilter{IncludeDocs: cfg.Filters.IncludeDocs, DocFilePatterns: cloneStrings(cfg.Filters.DocFilePatterns), IgnoreFilePatterns: cloneStrings(cfg.Filters.IgnoreFilePatterns), ForceIncludeFilePatterns: cloneStrings(cfg.Filters.ForceIncludeFilePatterns)}, nil
}

func (r PromptResolver) Load(ctx context.Context) (ReviewPromptConfig, error) {
	cfg, baseDir, err := r.load(ctx)
	if err == nil {
		_, err = loadPromptSet(baseDir, cfg.Prompts)
	}
	return cfg, err
}

func (r PromptResolver) load(ctx context.Context) (ReviewPromptConfig, string, error) {
	if err := ctx.Err(); err != nil {
		return ReviewPromptConfig{}, "", err
	}
	if strings.TrimSpace(r.ConfigPath) == "" {
		return ReviewPromptConfig{}, "", fmt.Errorf("caminho do YAML de prompts nao configurado")
	}
	raw, err := os.ReadFile(r.ConfigPath)
	if err != nil {
		return ReviewPromptConfig{}, "", fmt.Errorf("erro ao ler YAML de prompts %q: %w", r.ConfigPath, err)
	}
	var cfg ReviewPromptConfig
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return ReviewPromptConfig{}, "", fmt.Errorf("YAML de prompts invalido %q: %w", r.ConfigPath, err)
	}
	if err := cfg.Validate(); err != nil {
		return ReviewPromptConfig{}, "", fmt.Errorf("configuracao de prompts invalida %q: %w", r.ConfigPath, err)
	}
	return cfg, filepath.Dir(r.ConfigPath), nil
}

type ResolvedBlockFilter struct {
	IncludeDocs                                                   bool
	DocFilePatterns, IgnoreFilePatterns, ForceIncludeFilePatterns []string
}

func (cfg ReviewPromptConfig) Validate() error {
	for name, path := range map[string]string{"prompts.technical_review": cfg.Prompts.TechnicalReview, "prompts.security_performance": cfg.Prompts.SecurityPerformance, "prompts.import_divergence": cfg.Prompts.ImportDivergence, "prompts.final_response": cfg.Prompts.FinalResponse} {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%s nao pode estar vazio", name)
		}
	}
	return nil
}

func readPromptFile(baseDir, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("caminho de prompt vazio")
	}
	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(baseDir, candidate)
	}
	raw, err := os.ReadFile(candidate)
	if err != nil {
		return "", fmt.Errorf("erro ao ler prompt %q: %w", path, err)
	}
	content := strings.TrimSpace(string(raw))
	if content == "" {
		return "", fmt.Errorf("prompt %q nao pode estar vazio", path)
	}
	return content, nil
}

func cloneStrings(values []string) []string { return append([]string(nil), values...) }
