package review

import "testing"

func TestBuildPullReviewOptionsPreservesReviewForApproval(t *testing.T) {
	finalReview := FinalReview{
		Event:   GiteaEventRequestChanges,
		Status:  "achados",
		Summary: "Resumo da revisão",
		InlineComments: []InlineComment{{
			Severity:       "alta",
			Type:           "bug",
			Path:           "internal/app.go",
			NewPosition:    12,
			DecisionReason: "O fluxo pode falhar",
			Body:           "Corrija este caminho.",
		}},
	}

	options := BuildPullReviewOptions(finalReview, true)
	if options.Event != GiteaEventRequestChanges {
		t.Fatalf("expected request changes event, got %q", options.Event)
	}
	if options.Body != "> status: achados\nResumo da revisão" {
		t.Fatalf("unexpected body: %q", options.Body)
	}
	if len(options.Comments) != 1 || options.Comments[0].Path != "internal/app.go" || options.Comments[0].NewPosition != 12 {
		t.Fatalf("unexpected inline publication: %#v", options.Comments)
	}
}

func TestBuildPullReviewOptionsDowngradesAutonomousDecision(t *testing.T) {
	options := BuildPullReviewOptions(FinalReview{Event: GiteaEventApproved, Summary: "Tudo certo"}, false)
	if options.Event != GiteaEventComment {
		t.Fatalf("expected comment event, got %q", options.Event)
	}
}
