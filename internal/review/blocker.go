package review

import (
	"fmt"
	"strings"

	"gitea-agents/internal/diff"
	"gitea-agents/internal/filematch"
)

type ReviewBlock struct {
	Index   int
	Total   int
	Files   []string
	Content string
}

type ReviewFileFilter struct {
	IncludeDocs              bool
	DocFilePatterns          []string
	IgnoreFilePatterns       []string
	ForceIncludeFilePatterns []string
}

func BuildReviewBlocks(files []diff.ChangedFile, maxChars int, maxFilesPerBlock int) []ReviewBlock {
	return BuildReviewBlocksWithOptions(files, maxChars, maxFilesPerBlock, false)
}

func BuildReviewBlocksWithOptions(files []diff.ChangedFile, maxChars int, maxFilesPerBlock int, includeDocs bool) []ReviewBlock {
	filter := DefaultReviewFileFilter()
	filter.IncludeDocs = includeDocs
	return BuildReviewBlocksWithFilter(files, maxChars, maxFilesPerBlock, filter)
}

func BuildReviewBlocksWithFilter(files []diff.ChangedFile, maxChars int, maxFilesPerBlock int, filter ReviewFileFilter) []ReviewBlock {
	if maxChars <= 0 {
		maxChars = 4000
	}

	if maxFilesPerBlock <= 0 {
		maxFilesPerBlock = 2
	}

	filter = NormalizeReviewFileFilter(filter)

	var blocks []ReviewBlock
	var currentFiles []string
	var currentContent strings.Builder

	flush := func() {
		if len(currentFiles) == 0 {
			return
		}

		blocks = append(blocks, ReviewBlock{
			Files:   append([]string(nil), currentFiles...),
			Content: currentContent.String(),
		})
		currentFiles = nil
		currentContent.Reset()
	}

	for _, file := range files {
		if ShouldIgnoreReviewFileWithFilter(file.Path, filter) {
			continue
		}

		fileContent := formatReviewFile(file)
		wouldExceedChars := currentContent.Len() > 0 && currentContent.Len()+len(fileContent) > maxChars
		wouldExceedFiles := len(currentFiles) >= maxFilesPerBlock
		if wouldExceedChars || wouldExceedFiles {
			flush()
		}

		currentFiles = append(currentFiles, file.Path)
		currentContent.WriteString(fileContent)
	}

	flush()

	for index := range blocks {
		blocks[index].Index = index + 1
		blocks[index].Total = len(blocks)
	}

	return blocks
}

func DefaultReviewFileFilter() ReviewFileFilter {
	return ReviewFileFilter{
		IncludeDocs:              false,
		DocFilePatterns:          defaultDocFilePatterns(),
		IgnoreFilePatterns:       defaultIgnoreFilePatterns(),
		ForceIncludeFilePatterns: nil,
	}
}

func NormalizeReviewFileFilter(filter ReviewFileFilter) ReviewFileFilter {
	if filter.DocFilePatterns == nil {
		filter.DocFilePatterns = defaultDocFilePatterns()
	}
	if filter.IgnoreFilePatterns == nil {
		filter.IgnoreFilePatterns = defaultIgnoreFilePatterns()
	}
	if filter.ForceIncludeFilePatterns == nil {
		filter.ForceIncludeFilePatterns = []string{}
	}

	filter.DocFilePatterns = compactPatternList(filter.DocFilePatterns)
	filter.IgnoreFilePatterns = compactPatternList(filter.IgnoreFilePatterns)
	filter.ForceIncludeFilePatterns = compactPatternList(filter.ForceIncludeFilePatterns)
	return filter
}

func ShouldIgnoreReviewFile(path string, includeDocs bool) bool {
	filter := DefaultReviewFileFilter()
	filter.IncludeDocs = includeDocs
	return ShouldIgnoreReviewFileWithFilter(path, filter)
}

func ShouldIgnoreReviewFileWithFilter(path string, filter ReviewFileFilter) bool {
	filter = NormalizeReviewFileFilter(filter)
	if filematch.MatchesPath(filter.ForceIncludeFilePatterns, path) {
		return false
	}
	if !filter.IncludeDocs && IsDocumentationFileWithFilter(path, filter) {
		return true
	}

	return filematch.MatchesPath(filter.IgnoreFilePatterns, path)
}

func IsDocumentationFile(path string) bool {
	return IsDocumentationFileWithFilter(path, DefaultReviewFileFilter())
}

func IsDocumentationFileWithFilter(path string, filter ReviewFileFilter) bool {
	filter = NormalizeReviewFileFilter(filter)
	return filematch.MatchesPath(filter.DocFilePatterns, path)
}

func formatReviewFile(file diff.ChangedFile) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("===== INICIO ARQUIVO path=%s =====\n", file.Path))
	builder.WriteString(file.Patch)
	if !strings.HasSuffix(file.Patch, "\n") {
		builder.WriteByte('\n')
	}
	builder.WriteString(fmt.Sprintf("===== FIM ARQUIVO path=%s =====\n", file.Path))
	return builder.String()
}

func defaultDocFilePatterns() []string {
	return []string{
		"readme",
		"readme.*",
		"readme-*",
		"docs/**",
		"**/docs/**",
		"*.md",
		"*.mdx",
		"*.rst",
		"*.adoc",
		"license",
		"license.*",
		"changelog.*",
		"contributing.*",
		"code_of_conduct.*",
	}
}

func defaultIgnoreFilePatterns() []string {
	return []string{
		"*.yaml",
		"*.yml",
		"package-lock.json",
		"pnpm-lock.yaml",
		"yarn.lock",
		"go.sum",
		"composer.lock",
		"vendor/**",
		"**/vendor/**",
		"node_modules/**",
		"**/node_modules/**",
		"dist/**",
		"**/dist/**",
		"build/**",
		"**/build/**",
		"*.map",
		"*.min.js",
	}
}

func compactPatternList(patterns []string) []string {
	if patterns == nil {
		return nil
	}

	seen := map[string]struct{}{}
	result := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if _, exists := seen[pattern]; exists {
			continue
		}
		seen[pattern] = struct{}{}
		result = append(result, pattern)
	}

	return result
}
