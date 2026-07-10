package diff

import (
	"bufio"
	"strings"
)

type ChangedFile struct {
	Path      string
	Additions int
	Deletions int
	Patch     string
}

func Parse(raw string) []ChangedFile {
	if raw == "" {
		return nil
	}

	var files []ChangedFile
	var current *ChangedFile
	var patch strings.Builder

	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 1024), 1024*1024*10)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				current.Patch = patch.String()
				files = append(files, *current)
				patch.Reset()
			}

			current = &ChangedFile{
				Path: parsePathFromDiffHeader(line),
			}
		}

		if current == nil {
			continue
		}

		patch.WriteString(line)
		patch.WriteByte('\n')

		if strings.HasPrefix(line, "+++ b/") {
			current.Path = strings.TrimPrefix(line, "+++ b/")
			continue
		}

		if isAddedLine(line) {
			current.Additions++
			continue
		}

		if isDeletedLine(line) {
			current.Deletions++
		}
	}

	if current != nil {
		current.Patch = patch.String()
		files = append(files, *current)
	}

	return files
}

func parsePathFromDiffHeader(line string) string {
	parts := strings.Fields(line)
	if len(parts) < 4 {
		return ""
	}

	return strings.TrimPrefix(parts[3], "b/")
}

func isAddedLine(line string) bool {
	return strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++")
}

func isDeletedLine(line string) bool {
	return strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---")
}
