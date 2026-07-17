package review

import (
	"strings"

	"gitea-agents/internal/gitea"
)

func BuildPullReviewOptions(finalReview FinalReview, allowAutonomousReject bool) gitea.CreatePullReviewOptions {
	event := finalReview.Event
	if !allowAutonomousReject && (event == GiteaEventApproved || event == GiteaEventRequestChanges) {
		event = GiteaEventComment
	}
	if event == GiteaEventRequestChanges && !hasBlockingComment(finalReview.InlineComments) {
		event = GiteaEventComment
	}

	body := strings.TrimSpace(finalReview.ReviewBody())
	if commentsWithoutPosition := finalReview.CommentsWithoutPosition(); len(commentsWithoutPosition) > 0 {
		if body != "" {
			body += "\n\n"
		}
		body += "Comentarios sem linha especifica:\n"
		for _, comment := range commentsWithoutPosition {
			body += comment.SummaryLine() + "\n"
		}
		body = strings.TrimSpace(body)
	}

	options := gitea.CreatePullReviewOptions{Event: event, Body: body}
	for _, comment := range finalReview.InlineComments {
		if comment.Path == "" || comment.Body == "" || comment.NewPosition <= 0 {
			continue
		}
		options.Comments = append(options.Comments, gitea.CreatePullReviewComment{
			Body:        formatInlineReviewComment(comment),
			NewPosition: comment.NewPosition,
			Path:        comment.Path,
		})
	}
	return options
}

func formatInlineReviewComment(comment InlineComment) string {
	severity := strings.TrimSpace(comment.Severity)
	if severity == "" {
		severity = "nao informada"
	}
	commentType := strings.TrimSpace(comment.Type)
	if commentType == "" {
		commentType = "semantica"
	}
	body := "> severity: " + severity + "\n> tipo: " + commentType
	if reason := strings.TrimSpace(comment.DecisionReason); reason != "" {
		body += "\n\n" + reason
	}
	if commentBody := strings.TrimSpace(comment.Body); commentBody != "" {
		body += "\n\n" + commentBody
	}
	return strings.TrimSpace(body)
}

func hasBlockingComment(comments []InlineComment) bool {
	for _, comment := range comments {
		if strings.EqualFold(comment.Severity, "alta") || strings.EqualFold(comment.Severity, "media") {
			return true
		}
		reason := strings.ToLower(comment.DecisionReason)
		if isExplicitlyNonBlockingReason(reason) {
			continue
		}
		if strings.Contains(reason, "bloque") || strings.Contains(reason, "request_changes") {
			return true
		}
	}
	return false
}

func isExplicitlyNonBlockingReason(reason string) bool {
	reason = strings.ReplaceAll(reason, "\u00e3", "a")
	return strings.Contains(reason, "nao bloque") || strings.Contains(reason, "nao-bloque") || strings.Contains(reason, "non-block")
}
