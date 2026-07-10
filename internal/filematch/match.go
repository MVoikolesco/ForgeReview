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

	if strings.Contains(normalizedPattern, "**") && matchSegments(strings.Split(normalizedPattern, "/"), strings.Split(normalizedPath, "/")) {
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

func matchSegments(patternSegments []string, pathSegments []string) bool {
	if len(patternSegments) == 0 {
		return len(pathSegments) == 0
	}

	if patternSegments[0] == "**" {
		if matchSegments(patternSegments[1:], pathSegments) {
			return true
		}
		for index := range pathSegments {
			if matchSegments(patternSegments[1:], pathSegments[index+1:]) {
				return true
			}
		}
		return false
	}

	if len(pathSegments) == 0 {
		return false
	}

	matched, err := path.Match(patternSegments[0], pathSegments[0])
	if err != nil || !matched {
		return false
	}

	return matchSegments(patternSegments[1:], pathSegments[1:])
}
