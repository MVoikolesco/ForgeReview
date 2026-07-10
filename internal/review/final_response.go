package review

import (
	"fmt"
	"strconv"
	"strings"

	"gitea-agents/internal/diff"
)

const (
	GiteaEventApproved       = "APPROVED"
	GiteaEventComment        = "COMMENT"
	GiteaEventRequestChanges = "REQUEST_CHANGES"
)

type FinalReview struct {
	InlineComments []InlineComment
	Event          string
	Status         string
	Summary        string
	Observations   string
	Raw            string
	Structured     bool
}

type InlineComment struct {
	Severity        string
	Path            string
	NewPosition     int
	Reference       string
	Title           string
	Body            string
	DecisionReason  string
}

func ParseFinalReviewResponse(content string) FinalReview {
	parsed := FinalReview{Raw: strings.TrimSpace(content)}
	var current *InlineComment
	section := ""

	flushComment := func() {
		if current == nil {
			return
		}
		if current.Path != "" || current.Body != "" || current.Title != "" {
			parsed.InlineComments = append(parsed.InlineComments, *current)
		}
		current = nil
	}

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		switch trimmed {
		case "COMENTARIOS_INLINE:":
			flushComment()
			section = "comments"
			parsed.Structured = true
			continue
		case "REVISAO_FINAL:":
			flushComment()
			section = "final"
			parsed.Structured = true
			continue
		}

		switch section {
		case "comments":
			if trimmed == "- nenhum" {
				flushComment()
				continue
			}
			if strings.HasPrefix(trimmed, "- SEVERIDADE:") {
				flushComment()
				current = &InlineComment{}
				current.Severity = strings.TrimSpace(strings.TrimPrefix(trimmed, "- SEVERIDADE:"))
				continue
			}
			if current == nil {
				continue
			}
			key, value, ok := splitKeyValue(trimmed)
			if !ok {
				continue
			}
			applyInlineCommentField(current, key, value)
		case "final":
			key, value, ok := splitKeyValue(trimmed)
			if !ok {
				continue
			}
			switch strings.ToUpper(key) {
			case "EVENTO_GITEA":
				parsed.Event = normalizeGiteaReviewEvent(value)
			case "STATUS":
				parsed.Status = value
			case "RESUMO":
				parsed.Summary = value
			case "OBSERVACOES":
				parsed.Observations = value
			}
		}
	}
	flushComment()

	if parsed.Event == "" {
		parsed.Event = eventFromStatus(parsed.Status)
	}
	if parsed.Event == "" {
		parsed.Event = GiteaEventComment
	}
	if parsed.Summary == "" {
		parsed.Summary = fallbackSummary(parsed)
	}

	return parsed
}

func (r FinalReview) ReviewBody() string {
	var builder strings.Builder
	if r.Summary != "" {
		builder.WriteString(r.Summary)
	}
	if r.Observations != "" {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(r.Observations)
	}
	if builder.Len() == 0 && r.Raw != "" {
		builder.WriteString(r.Raw)
	}
	if builder.Len() == 0 {
		builder.WriteString("Review automatizado concluido.")
	}

	return builder.String()
}

func (r FinalReview) CommentsWithoutPosition() []InlineComment {
	var comments []InlineComment
	for _, comment := range r.InlineComments {
		if comment.Path == "" || comment.Body == "" {
			continue
		}
		if comment.NewPosition > 0 {
			continue
		}
		comments = append(comments, comment)
	}

	return comments
}

func (c InlineComment) SummaryLine() string {
	title := c.Title
	if title == "" {
		title = c.Reference
	}
	if title == "" {
		title = "Comentario de review"
	}
	if c.Path == "" {
		return fmt.Sprintf("- %s: %s", title, c.Body)
	}

	return fmt.Sprintf("- %s: %s - %s", c.Path, title, c.Body)
}

func ResolveFinalReviewCommentPositions(finalReview FinalReview, files []diff.ChangedFile) FinalReview {
	patchesByPath := make(map[string]string, len(files))
	for _, file := range files {
		patchesByPath[file.Path] = file.Patch
	}

	for index := range finalReview.InlineComments {
		comment := &finalReview.InlineComments[index]
		if comment.Path == "" || comment.Reference == "" {
			comment.NewPosition = 0
			continue
		}

		line := findAddedLineByReference(patchesByPath[comment.Path], comment.Reference)
		comment.NewPosition = line
	}

	return finalReview
}

func findAddedLineByReference(patch string, reference string) int {
	normalizedReference := normalizeReference(reference)
	if patch == "" || normalizedReference == "" {
		return 0
	}

	newLine := 0
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "@@") {
			newLine = parseHunkNewStart(line)
			continue
		}
		if newLine <= 0 || len(line) == 0 {
			continue
		}

		switch line[0] {
		case '+':
			if strings.HasPrefix(line, "+++") {
				continue
			}
			if strings.Contains(normalizeReference(line[1:]), normalizedReference) {
				return newLine
			}
			newLine++
		case '-':
			if strings.HasPrefix(line, "---") {
				continue
			}
		default:
			newLine++
		}
	}

	return 0
}

func parseHunkNewStart(header string) int {
	plusIndex := strings.Index(header, "+")
	if plusIndex < 0 {
		return 0
	}

	start := plusIndex + 1
	end := start
	for end < len(header) && header[end] >= '0' && header[end] <= '9' {
		end++
	}
	if start == end {
		return 0
	}

	line, err := strconv.Atoi(header[start:end])
	if err != nil {
		return 0
	}

	return line
}

func normalizeReference(value string) string {
	value = strings.Trim(value, "` ")
	return strings.Join(strings.Fields(value), " ")
}

func splitKeyValue(line string) (string, string, bool) {
	index := strings.Index(line, ":")
	if index < 0 {
		return "", "", false
	}

	key := strings.TrimSpace(line[:index])
	value := strings.TrimSpace(line[index+1:])
	return key, value, key != ""
}

func applyInlineCommentField(comment *InlineComment, key string, value string) {
	switch strings.ToUpper(key) {
	case "SEVERIDADE":
		comment.Severity = value
	case "PATH", "ARQUIVO":
		comment.Path = value
	case "NEW_POSITION", "LINHA_REFERENCIA":
		comment.NewPosition = parsePosition(value)
	case "TRECHO_REFERENCIA":
		comment.Reference = value
	case "TITULO":
		comment.Title = value
	case "BODY", "COMENTARIO_PR":
		comment.Body = value
	case "MOTIVO_DECISAO":
		comment.DecisionReason = value
	}
}

func parsePosition(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return 0
	}

	return parsed
}

func normalizeGiteaReviewEvent(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case GiteaEventApproved:
		return GiteaEventApproved
	case GiteaEventRequestChanges:
		return GiteaEventRequestChanges
	case GiteaEventComment:
		return GiteaEventComment
	}

	return ""
}

func eventFromStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "aprovado":
		return GiteaEventApproved
	case "reprovado":
		return GiteaEventRequestChanges
	case "aprovado_com_observacao", "comentario":
		return GiteaEventComment
	}

	return ""
}

func fallbackSummary(parsed FinalReview) string {
	if !parsed.Structured && parsed.Raw != "" {
		return parsed.Raw
	}
	switch parsed.Event {
	case GiteaEventApproved:
		return "Review automatizado aprovado."
	case GiteaEventRequestChanges:
		return "Review automatizado solicitou ajustes."
	default:
		return "Review automatizado concluido."
	}
}
