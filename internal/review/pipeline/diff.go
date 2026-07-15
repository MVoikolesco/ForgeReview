package pipeline

import (
	"fmt"
	"sort"
	"strings"

	"gitea-agents/internal/diff"
)

func filesByPath(files []diff.ChangedFile) map[string]diff.ChangedFile {
	byPath := make(map[string]diff.ChangedFile, len(files))
	for _, file := range files {
		byPath[file.Path] = file
	}
	return byPath
}

func formatFilesDiff(files []diff.ChangedFile) string {
	var b strings.Builder
	for _, file := range files {
		b.WriteString(fmt.Sprintf("===== INICIO ARQUIVO path=%s additions=%d deletions=%d =====\n", file.Path, file.Additions, file.Deletions))
		b.WriteString(file.Patch)
		if !strings.HasSuffix(file.Patch, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString(fmt.Sprintf("===== FIM ARQUIVO path=%s =====\n", file.Path))
	}
	return b.String()
}

func selectFiles(all []diff.ChangedFile, paths []string) []diff.ChangedFile {
	byPath := filesByPath(all)
	selected := make([]diff.ChangedFile, 0, len(paths))
	for _, path := range paths {
		if file, ok := byPath[path]; ok {
			selected = append(selected, file)
		}
	}
	return selected
}

func validAddedLine(file diff.ChangedFile, line int) bool {
	if line <= 0 {
		return false
	}
	for _, added := range addedLines(file.Patch) {
		if added == line {
			return true
		}
	}
	return false
}

func addedLines(patch string) []int {
	var lines []int
	newLine := 0
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "@@") {
			newLine = parseHunkNewStart(line)
			continue
		}
		if newLine <= 0 || line == "" {
			continue
		}
		switch line[0] {
		case '+':
			if strings.HasPrefix(line, "+++") {
				continue
			}
			lines = append(lines, newLine)
			newLine++
		case '-':
			if strings.HasPrefix(line, "---") {
				continue
			}
		default:
			newLine++
		}
	}
	return lines
}

func parseHunkNewStart(header string) int {
	plus := strings.Index(header, "+")
	if plus < 0 {
		return 0
	}
	start := plus + 1
	end := start
	for end < len(header) && header[end] >= '0' && header[end] <= '9' {
		end++
	}
	var n int
	_, _ = fmt.Sscanf(header[start:end], "%d", &n)
	return n
}

func sortedFilePaths(files []diff.ChangedFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	sort.Strings(paths)
	return paths
}
