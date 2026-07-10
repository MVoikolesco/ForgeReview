package promptconfig

import (
	"gitea-agents/internal/filematch"
)

func matchesAnyFile(patterns []string, files []string) bool {
	return filematch.MatchesAnyFile(patterns, files)
}

func MatchFilePattern(pattern string, filePath string) bool {
	return filematch.Match(pattern, filePath)
}
