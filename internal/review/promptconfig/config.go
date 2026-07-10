package promptconfig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ReviewPromptConfig struct {
	Default DefaultPromptConfig    `yaml:"default"`
	Stacks  map[string]StackConfig `yaml:"stacks"`
}

type DefaultPromptConfig struct {
	PartialPrompt            string   `yaml:"partial_prompt"`
	FinalPrompt              string   `yaml:"final_prompt"`
	IncludeDocs              bool     `yaml:"include_docs"`
	DocFilePatterns          []string `yaml:"doc_file_patterns"`
	IgnoreFilePatterns       []string `yaml:"ignore_file_patterns"`
	ForceIncludeFilePatterns []string `yaml:"force_include_file_patterns"`
}

type StackConfig struct {
	Prompt       string   `yaml:"prompt"`
	FilePatterns []string `yaml:"file_patterns"`
}

type PromptResolver struct {
	ConfigPath string
}

type ResolvedPrompt struct {
	Content string
	Stacks  []string
}

func NewPromptResolver(configPath string) PromptResolver {
	return PromptResolver{ConfigPath: configPath}
}

func (r PromptResolver) ResolvePartialPrompt(ctx context.Context, owner, repo string, files []string) (ResolvedPrompt, error) {
	cfg, baseDir, err := r.load(ctx)
	if err != nil {
		return ResolvedPrompt{}, err
	}

	basePrompt, err := readPromptFile(baseDir, cfg.Default.PartialPrompt)
	if err != nil {
		return ResolvedPrompt{}, err
	}

	var builder strings.Builder
	builder.WriteString(basePrompt)

	stacks, err := r.resolveStackPrompts(baseDir, cfg, files)
	if err != nil {
		return ResolvedPrompt{}, err
	}

	if len(stacks) > 0 {
		builder.WriteString("\n\n## Regras adicionais da stack\n")
		for _, stack := range stacks {
			builder.WriteString("\n### ")
			builder.WriteString(stack.Name)
			builder.WriteByte('\n')
			builder.WriteString(stack.Content)
			builder.WriteByte('\n')
		}
	}

	return ResolvedPrompt{
		Content: strings.TrimSpace(builder.String()),
		Stacks:  promptNames(stacks),
	}, nil
}

func (r PromptResolver) ResolveFinalPrompt(ctx context.Context, owner, repo string) (ResolvedPrompt, error) {
	cfg, baseDir, err := r.load(ctx)
	if err != nil {
		return ResolvedPrompt{}, err
	}

	prompt, err := readPromptFile(baseDir, cfg.Default.FinalPrompt)
	if err != nil {
		return ResolvedPrompt{}, err
	}

	return ResolvedPrompt{Content: strings.TrimSpace(prompt)}, nil
}

func (r PromptResolver) ShouldIncludeDocs(ctx context.Context, owner, repo string) (bool, error) {
	cfg, _, err := r.load(ctx)
	if err != nil {
		return false, err
	}

	return cfg.Default.IncludeDocs, nil
}

func (r PromptResolver) ResolveBlockFilter(ctx context.Context, owner, repo string) (ResolvedBlockFilter, error) {
	cfg, _, err := r.load(ctx)
	if err != nil {
		return ResolvedBlockFilter{}, err
	}

	return ResolvedBlockFilter{
		IncludeDocs:              cfg.Default.IncludeDocs,
		DocFilePatterns:          cloneStrings(cfg.Default.DocFilePatterns),
		IgnoreFilePatterns:       cloneStrings(cfg.Default.IgnoreFilePatterns),
		ForceIncludeFilePatterns: cloneStrings(cfg.Default.ForceIncludeFilePatterns),
	}, nil
}

func (r PromptResolver) Load(ctx context.Context) (ReviewPromptConfig, error) {
	cfg, _, err := r.load(ctx)
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
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return ReviewPromptConfig{}, "", fmt.Errorf("YAML de prompts invalido %q: %w", r.ConfigPath, err)
	}

	if err := cfg.Validate(); err != nil {
		return ReviewPromptConfig{}, "", fmt.Errorf("configuracao de prompts invalida %q: %w", r.ConfigPath, err)
	}

	return cfg, filepath.Dir(r.ConfigPath), nil
}

type ResolvedBlockFilter struct {
	IncludeDocs              bool
	DocFilePatterns          []string
	IgnoreFilePatterns       []string
	ForceIncludeFilePatterns []string
}

func (cfg ReviewPromptConfig) Validate() error {
	if strings.TrimSpace(cfg.Default.PartialPrompt) == "" {
		return fmt.Errorf("default.partial_prompt nao pode estar vazio")
	}
	if strings.TrimSpace(cfg.Default.FinalPrompt) == "" {
		return fmt.Errorf("default.final_prompt nao pode estar vazio")
	}
	for stackName, stackConfig := range cfg.Stacks {
		if strings.TrimSpace(stackConfig.Prompt) == "" {
			return fmt.Errorf("stacks.%s.prompt nao pode estar vazio", stackName)
		}
		if len(stackConfig.FilePatterns) == 0 {
			return fmt.Errorf("stacks.%s.file_patterns nao pode estar vazio", stackName)
		}
	}

	return nil
}

type namedPrompt struct {
	Name    string
	Content string
}

func (r PromptResolver) resolveStackPrompts(baseDir string, cfg ReviewPromptConfig, files []string) ([]namedPrompt, error) {
	if len(files) == 0 {
		return nil, nil
	}

	var stacks []namedPrompt
	seenStacks := map[string]struct{}{}

	stackNames := make([]string, 0, len(cfg.Stacks))
	for stackName := range cfg.Stacks {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)

	for _, stackName := range stackNames {
		stackConfig := cfg.Stacks[stackName]
		if !matchesAnyFile(stackConfig.FilePatterns, files) {
			continue
		}

		if _, exists := seenStacks[stackName]; !exists {
			content, err := readPromptFile(baseDir, stackConfig.Prompt)
			if err != nil {
				return nil, err
			}
			stacks = append(stacks, namedPrompt{Name: stackName, Content: content})
			seenStacks[stackName] = struct{}{}
		}
	}

	return stacks, nil
}

func readPromptFile(baseDir string, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("caminho de prompt vazio")
	}

	var candidates []string
	if filepath.IsAbs(path) {
		candidates = []string{path}
	} else {
		candidates = []string{
			path,
			filepath.Join(baseDir, path),
			filepath.Join(filepath.Dir(baseDir), path),
		}
	}

	var lastErr error
	for _, candidate := range candidates {
		content, err := os.ReadFile(candidate)
		if err == nil {
			return strings.TrimSpace(string(content)), nil
		}
		lastErr = err
	}

	return "", fmt.Errorf("erro ao ler prompt %q: %w", path, lastErr)
}

func promptNames(prompts []namedPrompt) []string {
	names := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		names = append(names, prompt.Name)
	}
	return names
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}

	return append([]string{}, values...)
}
