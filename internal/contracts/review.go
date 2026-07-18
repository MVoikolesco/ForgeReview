package contracts

// Comment describes one inline review finding produced by an AI provider.
type Comment struct {
	File           string `json:"file"`
	Line           int    `json:"line"`
	Severity       string `json:"severity"`
	DecisionReason string `json:"decision_reason"`
	Comment        string `json:"comment"`
}

// FinalReview describes the overall decision and message published to Gitea.
type FinalReview struct {
	GiteaEvent   string `json:"gitea_event"`
	Status       string `json:"status"`
	Summary      string `json:"summary"`
	Observations string `json:"observations"`
}

// Result is the provider-independent review contract persisted by the backend
// and sent to the Gitea integration.
type Result struct {
	ReviewID    string         `json:"review_id,omitempty"`
	Provider    string         `json:"provider,omitempty"`
	Model       string         `json:"model,omitempty"`
	Agent       string         `json:"agent,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Comments    []Comment      `json:"comments"`
	FinalReview FinalReview    `json:"final_review"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}
