package review

import (
	"fmt"
	"strings"

	"gitea-agents/internal/diff"
)

type ReviewBlock struct {
	Index   int
	Total   int
	Files   []string
	Content string
}

func BuildReviewBlocks(files []diff.ChangedFile, maxChars int, maxFilesPerBlock int) []ReviewBlock {
	return BuildReviewBlocksWithOptions(files, maxChars, maxFilesPerBlock, false)
}

func BuildReviewBlocksWithOptions(files []diff.ChangedFile, maxChars int, maxFilesPerBlock int, includeDocs bool) []ReviewBlock {
	if maxChars <= 0 {
		maxChars = 4000
	}

	if maxFilesPerBlock <= 0 {
		maxFilesPerBlock = 2
	}

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
		if ShouldIgnoreReviewFile(file.Path, includeDocs) {
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

func ShouldIgnoreReviewFile(path string, includeDocs bool) bool {
	normalized := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	baseName := normalized
	if slashIndex := strings.LastIndex(baseName, "/"); slashIndex >= 0 {
		baseName = baseName[slashIndex+1:]
	}

	if !includeDocs && (baseName == "readme" || strings.HasPrefix(baseName, "readme.") || strings.HasPrefix(baseName, "readme-")) {
		return true
	}

	if !includeDocs && isDocumentationFile(normalized, baseName) {
		return true
	}

	if strings.HasSuffix(baseName, ".yaml") || strings.HasSuffix(baseName, ".yml") {
		return true
	}

	if normalized == "package-lock.json" ||
		strings.HasSuffix(normalized, "/package-lock.json") ||
		normalized == "pnpm-lock.yaml" ||
		strings.HasSuffix(normalized, "/pnpm-lock.yaml") ||
		normalized == "yarn.lock" ||
		strings.HasSuffix(normalized, "/yarn.lock") ||
		normalized == "go.sum" ||
		strings.HasSuffix(normalized, "/go.sum") ||
		normalized == "composer.lock" ||
		strings.HasSuffix(normalized, "/composer.lock") {
		return true
	}

	if strings.Contains(normalized, "/vendor/") ||
		strings.HasPrefix(normalized, "vendor/") ||
		strings.Contains(normalized, "/node_modules/") ||
		strings.HasPrefix(normalized, "node_modules/") ||
		strings.Contains(normalized, "/dist/") ||
		strings.HasPrefix(normalized, "dist/") ||
		strings.Contains(normalized, "/build/") ||
		strings.HasPrefix(normalized, "build/") {
		return true
	}

	return strings.HasSuffix(normalized, ".map") || strings.HasSuffix(normalized, ".min.js")
}

func IsDocumentationFile(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	baseName := normalized
	if slashIndex := strings.LastIndex(baseName, "/"); slashIndex >= 0 {
		baseName = baseName[slashIndex+1:]
	}

	return baseName == "readme" ||
		strings.HasPrefix(baseName, "readme.") ||
		strings.HasPrefix(baseName, "readme-") ||
		isDocumentationFile(normalized, baseName)
}

func isDocumentationFile(path string, baseName string) bool {
	if strings.HasPrefix(path, "docs/") || strings.Contains(path, "/docs/") {
		return true
	}

	if strings.HasSuffix(baseName, ".md") || strings.HasSuffix(baseName, ".mdx") ||
		strings.HasSuffix(baseName, ".rst") || strings.HasSuffix(baseName, ".adoc") {
		return true
	}

	return baseName == "license" ||
		strings.HasPrefix(baseName, "license.") ||
		strings.HasPrefix(baseName, "changelog.") ||
		strings.HasPrefix(baseName, "contributing.") ||
		strings.HasPrefix(baseName, "code_of_conduct.")
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
