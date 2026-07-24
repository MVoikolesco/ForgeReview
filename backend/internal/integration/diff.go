package integration

import (
	"strconv"
	"strings"
)

// attachUnifiedDiffPatches enriches the file metadata returned by Gitea's
// /files endpoint with the corresponding block from the canonical .diff
// response. Gitea does not consistently include a patch field in /files.
func attachUnifiedDiffPatches(files []map[string]any, diff string) int {
	patches := unifiedDiffPatches(diff)
	reviewable := 0
	for _, file := range files {
		if !hasReviewableContent(file) {
			filename, _ := file["filename"].(string)
			patch := patches[filename]
			if patch == "" {
				if previous, _ := file["previous_filename"].(string); previous != "" {
					patch = patches[previous]
				}
			}
			if patch != "" {
				file["patch"] = patch
			}
		}
		if hasReviewableContent(file) {
			reviewable++
		}
	}
	return reviewable
}

func hasReviewableContent(file map[string]any) bool {
	for _, key := range []string{"patch", "content", "diff"} {
		if value, ok := file[key].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func unifiedDiffPatches(diff string) map[string]string {
	normalized := strings.ReplaceAll(diff, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	starts := make([]int, 0)
	for index, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			starts = append(starts, index)
		}
	}
	if len(starts) == 0 && strings.TrimSpace(normalized) != "" {
		starts = append(starts, 0)
	}

	patches := make(map[string]string, len(starts))
	for index, start := range starts {
		end := len(lines)
		if index+1 < len(starts) {
			end = starts[index+1]
		}
		block := strings.TrimRight(strings.Join(lines[start:end], "\n"), "\n")
		if filename := unifiedDiffFilename(lines[start:end]); filename != "" {
			patches[filename] = block
		}
	}
	return patches
}

func unifiedDiffFilename(lines []string) string {
	for _, prefix := range []string{"+++ ", "--- "} {
		for _, line := range lines {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			filename := decodeUnifiedDiffPath(strings.TrimPrefix(line, prefix))
			if filename != "" && filename != "/dev/null" {
				return filename
			}
		}
	}
	return ""
}

func decodeUnifiedDiffPath(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, `"`) {
		if decoded, err := strconv.Unquote(value); err == nil {
			value = decoded
		}
	} else if tab := strings.IndexByte(value, '\t'); tab >= 0 {
		value = value[:tab]
	}
	if strings.HasPrefix(value, "a/") || strings.HasPrefix(value, "b/") {
		value = value[2:]
	}
	return value
}
