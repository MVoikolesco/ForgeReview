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
	Default  DefaultPromptConfig      `yaml:"default"`
	Projects map[string]ProjectConfig `yaml:"projects"`
	Stacks   map[string]StackConfig   `yaml:"stacks"`
}

type DefaultPromptConfig struct {
	PartialPrompt            string   `yaml:"partial_prompt"`
	FinalPrompt              string   `yaml:"final_prompt"`
	Stacks                   []string `yaml:"stacks"`
	IncludeDocs              bool     `yaml:"include_docs"`
	DocFilePatterns          []string `yaml:"doc_file_patterns"`
	IgnoreFilePatterns       []string `yaml:"ignore_file_patterns"`
	ForceIncludeFilePatterns []string `yaml:"force_include_file_patterns"`
}

type ProjectConfig struct {
	Owner                    string   `yaml:"owner"`
	Repo                     string   `yaml:"repo"`
	PartialPrompt            string   `yaml:"partial_prompt"`
	FinalPrompt              string   `yaml:"final_prompt"`
	Stacks                   []string `yaml:"stacks"`
	IncludeDocs              *bool    `yaml:"include_docs"`
	DocFilePatterns          []string `yaml:"doc_file_patterns"`
	IgnoreFilePatterns       []string `yaml:"ignore_file_patterns"`
	ForceIncludeFilePatterns []string `yaml:"force_include_file_patterns"`
}

type StackConfig struct {
	Prompt       string                    `yaml:"prompt"`
	FilePatterns []string                  `yaml:"file_patterns"`
	Categories   map[string]CategoryConfig `yaml:"categories"`
}

type CategoryConfig struct {
	Prompt       string   `yaml:"prompt"`
	FilePatterns []string `yaml:"file_patterns"`
}

type PromptResolver struct {
	ConfigPath string
}

type ResolvedPrompt struct {
	Content    string
	Stacks     []string
	Categories []string
}

func NewPromptResolver(configPath string) PromptResolver {
	return PromptResolver{ConfigPath: configPath}
}

func (r PromptResolver) ResolvePartialPrompt(ctx context.Context, owner, repo string, files []string) (ResolvedPrompt, error) {
	cfg, baseDir, err := r.load(ctx)
	if err != nil {
		return ResolvedPrompt{}, err
	}

	project := cfg.ProjectFor(owner, repo)
	basePrompt, err := readPromptFile(baseDir, project.PartialPrompt)
	if err != nil {
		return ResolvedPrompt{}, err
	}

	var builder strings.Builder
	builder.WriteString(basePrompt)

	stacks, categories, err := r.resolvePromptAddons(baseDir, cfg, project.Stacks, files)
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

	if len(categories) > 0 {
		builder.WriteString("\n\n## Regras especificas por tipo de arquivo\n")
		for _, category := range categories {
			builder.WriteString("\n### ")
			builder.WriteString(category.Name)
			builder.WriteByte('\n')
			builder.WriteString(category.Content)
			builder.WriteByte('\n')
		}
	}

	return ResolvedPrompt{
		Content:    strings.TrimSpace(builder.String()),
		Stacks:     promptNames(stacks),
		Categories: promptNames(categories),
	}, nil
}

func (r PromptResolver) ResolveFinalPrompt(ctx context.Context, owner, repo string) (ResolvedPrompt, error) {
	cfg, baseDir, err := r.load(ctx)
	if err != nil {
		return ResolvedPrompt{}, err
	}

	project := cfg.ProjectFor(owner, repo)
	prompt, err := readPromptFile(baseDir, project.FinalPrompt)
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

	project := cfg.ProjectFor(owner, repo)
	return project.IncludeDocs, nil
}

func (r PromptResolver) ResolveBlockFilter(ctx context.Context, owner, repo string) (ResolvedBlockFilter, error) {
	cfg, _, err := r.load(ctx)
	if err != nil {
		return ResolvedBlockFilter{}, err
	}

	project := cfg.ProjectFor(owner, repo)
	return ResolvedBlockFilter{
		IncludeDocs:              project.IncludeDocs,
		DocFilePatterns:          cloneStrings(project.DocFilePatterns),
		IgnoreFilePatterns:       cloneStrings(project.IgnoreFilePatterns),
		ForceIncludeFilePatterns: cloneStrings(project.ForceIncludeFilePatterns),
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

func (cfg ReviewPromptConfig) ProjectFor(owner string, repo string) ResolvedProject {
	base := ResolvedProject{
		PartialPrompt:            cfg.Default.PartialPrompt,
		FinalPrompt:              cfg.Default.FinalPrompt,
		Stacks:                   uniqueStrings(cfg.Default.Stacks),
		IncludeDocs:              cfg.Default.IncludeDocs,
		DocFilePatterns:          uniqueStrings(cfg.Default.DocFilePatterns),
		IgnoreFilePatterns:       uniqueStrings(cfg.Default.IgnoreFilePatterns),
		ForceIncludeFilePatterns: uniqueStrings(cfg.Default.ForceIncludeFilePatterns),
	}

	for _, project := range cfg.Projects {
		if strings.EqualFold(project.Owner, owner) && strings.EqualFold(project.Repo, repo) {
			return resolveProject(base, project)
		}
	}

	return base
}

type ResolvedProject struct {
	PartialPrompt            string
	FinalPrompt              string
	Stacks                   []string
	IncludeDocs              bool
	DocFilePatterns          []string
	IgnoreFilePatterns       []string
	ForceIncludeFilePatterns []string
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
	if cfg.Projects == nil {
		cfg.Projects = map[string]ProjectConfig{}
	}

	return nil
}

func resolveProject(base ResolvedProject, project ProjectConfig) ResolvedProject {
	resolved := base
	if strings.TrimSpace(project.PartialPrompt) != "" {
		resolved.PartialPrompt = project.PartialPrompt
	}
	if strings.TrimSpace(project.FinalPrompt) != "" {
		resolved.FinalPrompt = project.FinalPrompt
	}
	if project.Stacks != nil {
		resolved.Stacks = uniqueStrings(project.Stacks)
	}
	if project.IncludeDocs != nil {
		resolved.IncludeDocs = *project.IncludeDocs
	}
	if project.DocFilePatterns != nil {
		resolved.DocFilePatterns = uniqueStrings(project.DocFilePatterns)
	}
	if project.IgnoreFilePatterns != nil {
		resolved.IgnoreFilePatterns = uniqueStrings(project.IgnoreFilePatterns)
	}
	if project.ForceIncludeFilePatterns != nil {
		resolved.ForceIncludeFilePatterns = uniqueStrings(project.ForceIncludeFilePatterns)
	}

	return resolved
}

type namedPrompt struct {
	Name    string
	Content string
}

func (r PromptResolver) resolvePromptAddons(baseDir string, cfg ReviewPromptConfig, projectStacks []string, files []string) ([]namedPrompt, []namedPrompt, error) {
	var stacks []namedPrompt
	var categories []namedPrompt
	seenStacks := map[string]struct{}{}
	seenCategories := map[string]struct{}{}

	for _, stackName := range projectStacks {
		stackName = strings.TrimSpace(stackName)
		if stackName == "" {
			continue
		}

		stackConfig, ok := cfg.Stacks[stackName]
		if !ok {
			continue
		}
		if !matchesAnyFile(stackConfig.FilePatterns, files) {
			continue
		}

		if _, exists := seenStacks[stackName]; !exists {
			content, err := readPromptFile(baseDir, stackConfig.Prompt)
			if err != nil {
				return nil, nil, err
			}
			stacks = append(stacks, namedPrompt{Name: stackName, Content: content})
			seenStacks[stackName] = struct{}{}
		}

		categoryNames := make([]string, 0, len(stackConfig.Categories))
		for categoryName := range stackConfig.Categories {
			categoryNames = append(categoryNames, categoryName)
		}
		sort.Strings(categoryNames)

		for _, categoryName := range categoryNames {
			categoryConfig := stackConfig.Categories[categoryName]
			if !matchesAnyFile(categoryConfig.FilePatterns, files) {
				continue
			}
			fullName := stackName + "/" + categoryName
			if _, exists := seenCategories[fullName]; exists {
				continue
			}
			content, err := readPromptFile(baseDir, categoryConfig.Prompt)
			if err != nil {
				return nil, nil, err
			}
			categories = append(categories, namedPrompt{Name: fullName, Content: content})
			seenCategories[fullName] = struct{}{}
		}
	}

	return stacks, categories, nil
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

func uniqueStrings(values []string) []string {
	if values == nil {
		return nil
	}

	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}

	return append([]string{}, values...)
}
