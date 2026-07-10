package filematch

import (
	"path"
	"strings"
)

func MatchesAnyFile(patterns []string, files []string) bool {
	for _, file := range files {
		if MatchesPath(patterns, file) {
			return true
		}
	}

	return false
}

func MatchesPath(patterns []string, filePath string) bool {
	for _, pattern := range patterns {
		if Match(pattern, filePath) {
			return true
		}
	}

	return false
}

func Match(pattern string, filePath string) bool {
	normalizedPattern := Normalize(pattern)
	if normalizedPattern == "" {
		return false
	}

	normalizedPath := Normalize(filePath)
	if normalizedPath == "" {
		return false
	}

	baseName := path.Base(normalizedPath)

	if matchDoubleStarPattern(normalizedPattern, normalizedPath) {
		return true
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

func Normalize(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")), "./")
}

func matchDoubleStarPattern(pattern string, filePath string) bool {
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		if strings.HasPrefix(prefix, "**/") {
			dir := strings.TrimPrefix(prefix, "**/")
			return filePath == dir ||
				strings.HasPrefix(filePath, dir+"/") ||
				strings.Contains(filePath, "/"+dir+"/")
		}

		return filePath == prefix || strings.HasPrefix(filePath, prefix+"/")
	}

	if strings.HasPrefix(pattern, "**/") {
		suffix := strings.TrimPrefix(pattern, "**/")
		return filePath == suffix || strings.HasSuffix(filePath, "/"+suffix)
	}

	return false
}
