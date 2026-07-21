package review

import (
	"errors"
	"fmt"
	"strings"
)

var allowedSeverities = []string{"critica", "alta", "media", "baixa"}
var allowedGiteaEvents = []string{"APPROVE", "COMMENT", "REQUEST_CHANGES"}

// validateResult verifies the provider result fields required for persistence
// and Gitea publication.
func validateResult(result Result) error {
	for _, comment := range result.Comments {
		if strings.TrimSpace(comment.File) == "" ||
			comment.Line <= 0 ||
			strings.TrimSpace(comment.Comment) == "" ||
			strings.TrimSpace(comment.DecisionReason) == "" {
			return errors.New("comentário de review inválido")
		}
		if !containsValue(allowedSeverities, comment.Severity) {
			return fmt.Errorf("severidade inválida: %s", comment.Severity)
		}
	}

	if strings.TrimSpace(result.FinalReview.Summary) == "" {
		return errors.New("resumo final vazio")
	}
	if !containsValue(allowedGiteaEvents, result.FinalReview.GiteaEvent) {
		return fmt.Errorf("evento Gitea inválido: %s", result.FinalReview.GiteaEvent)
	}

	return nil
}

// containsValue reports whether value is present in values.
func containsValue(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

// splitDiff groups file diffs by character and file limits. Individual file
// sections larger than maxChars are truncated to preserve the current contract.
func splitDiff(raw string, maxChars, maxFiles int) []string {
	if maxChars <= 0 {
		maxChars = 2000
	}
	if maxFiles <= 0 {
		maxFiles = 2
	}

	files := strings.Split(raw, "diff --git ")
	blocks := []string{}
	current := ""
	fileCount := 0

	for _, file := range files {
		if file == "" {
			continue
		}

		piece := "diff --git " + file
		if len(piece) > maxChars {
			piece = piece[:maxChars]
		}
		if current != "" && (len(current)+len(piece) > maxChars || fileCount >= maxFiles) {
			blocks = append(blocks, current)
			current = ""
			fileCount = 0
		}

		current += piece
		fileCount++
	}

	if current != "" {
		blocks = append(blocks, current)
	}
	return blocks
}

// mergeComments appends unique incoming comments while preserving existing
// order. Uniqueness is based on file, line, and comment body.
func mergeComments(existing, incoming []Comment) []Comment {
	seen := map[string]bool{}
	for _, comment := range existing {
		seen[commentKey(comment)] = true
	}
	for _, comment := range incoming {
		key := commentKey(comment)
		if comment.File != "" && !seen[key] {
			existing = append(existing, comment)
			seen[key] = true
		}
	}
	return existing
}

// commentKey returns the stable deduplication key for one review comment.
func commentKey(comment Comment) string {
	return comment.File + fmt.Sprint(comment.Line) + comment.Comment
}
