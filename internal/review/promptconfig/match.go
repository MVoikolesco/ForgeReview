package promptconfig

import (
	"path"
	"strings"
)

func matchesAnyFile(patterns []string, files []string) bool {
	for _, file := range files {
		for _, pattern := range patterns {
			if MatchFilePattern(pattern, file) {
				return true
			}
		}
	}

	return false
}

func MatchFilePattern(pattern string, filePath string) bool {
	normalizedPattern := normalizePattern(pattern)
	normalizedPath := normalizePattern(filePath)
	baseName := path.Base(normalizedPath)

	if strings.HasSuffix(normalizedPattern, "/**") {
		prefix := strings.TrimSuffix(normalizedPattern, "/**")
		return normalizedPath == prefix || strings.HasPrefix(normalizedPath, prefix+"/")
	}

	target := baseName
	if strings.Contains(normalizedPattern, "/") {
		target = normalizedPath
	}

	if matched, err := path.Match(normalizedPattern, target); err == nil && matched {
		return true
	}

	return normalizedPath == normalizedPattern || baseName == normalizedPattern
}

func normalizePattern(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")), "./")
}
