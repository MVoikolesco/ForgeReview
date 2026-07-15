package pipeline

import (
	"context"

	"gitea-agents/internal/diff"
)

const Version = "2"

type Config struct {
	PlannerEnabled              bool
	ConsolidatorEnabled         bool
	VerifierEnabled             bool
	FormatterEnabled            bool
	PlannerMaxOutputTokens      int
	ReviewerMaxOutputTokens     int
	ConsolidatorMaxOutputTokens int
	VerifierMaxOutputTokens     int
	FormatterMaxOutputTokens    int
	ContextWindow               int
	MaxInputTokens              int
	SafetyMarginTokens          int
	MinimumPublishConfidence    float64
	MaxParallelReviewGroups     int
	MediumSeverityEvent         string
	PartialEvent                string
	MaxGroupChars               int
	MaxFilesPerGroup            int
}

func DefaultConfig() Config {
	return Config{PlannerEnabled: true, ConsolidatorEnabled: true, VerifierEnabled: true, FormatterEnabled: true, PlannerMaxOutputTokens: 2000, ReviewerMaxOutputTokens: 3500, ConsolidatorMaxOutputTokens: 3500, VerifierMaxOutputTokens: 2500, FormatterMaxOutputTokens: 2500, SafetyMarginTokens: 2048, MinimumPublishConfidence: 0.75, MaxParallelReviewGroups: 1, MediumSeverityEvent: "REQUEST_CHANGES", PartialEvent: "COMMENT", MaxGroupChars: 4000, MaxFilesPerGroup: 2}
}

type Input struct {
	Owner             string
	Repository        string
	PullRequestNumber int
	Title             string
	Description       string
	Author            string
	BaseBranch        string
	HeadBranch        string
	Files             []diff.ChangedFile
	Stacks            []string
	GlobalRules       string
}

type ReviewPlan struct {
	PRSummary   string        `json:"pr_summary"`
	RiskLevel   string        `json:"risk_level"`
	RiskAreas   []string      `json:"risk_areas"`
	Groups      []ReviewGroup `json:"groups"`
	Assumptions []string      `json:"assumptions"`
}

type ReviewGroup struct {
	ID             string   `json:"id"`
	Purpose        string   `json:"purpose"`
	Files          []string `json:"files"`
	RelevantStacks []string `json:"relevant_stacks"`
	RiskLevel      string   `json:"risk_level"`
	ReviewFocus    []string `json:"review_focus"`
}

type ReviewFinding struct {
	ID               string   `json:"id"`
	SourceGroupID    string   `json:"source_group_id,omitempty"`
	SourceFindingIDs []string `json:"source_finding_ids,omitempty"`
	File             string   `json:"file"`
	Line             int      `json:"line"`
	EndLine          *int     `json:"end_line,omitempty"`
	Severity         string   `json:"severity"`
	Category         string   `json:"category"`
	Confidence       float64  `json:"confidence"`
	Title            string   `json:"title"`
	DecisionReason   string   `json:"decision_reason"`
	Comment          string   `json:"comment"`
	Evidence         string   `json:"evidence"`
	FailureScenario  string   `json:"failure_scenario"`
	SuggestedFix     string   `json:"suggested_fix"`
	IntroducedByPR   bool     `json:"introduced_by_pr"`
}

type GroupReview struct {
	GroupID       string          `json:"group_id"`
	ReviewedFiles []string        `json:"reviewed_files"`
	Findings      []ReviewFinding `json:"findings"`
	ReviewSummary string          `json:"review_summary"`
}

type DiscardedFinding struct {
	SourceFindingID string `json:"source_finding_id"`
	Reason          string `json:"reason"`
}

type ConsolidatedReview struct {
	Findings          []ReviewFinding    `json:"findings"`
	PRSummary         string             `json:"pr_summary"`
	OverallRisk       string             `json:"overall_risk"`
	DiscardedFindings []DiscardedFinding `json:"discarded_findings"`
}

type VerificationResult struct {
	FindingID          string         `json:"finding_id"`
	Status             string         `json:"status"`
	Confidence         float64        `json:"confidence"`
	VerificationReason string         `json:"verification_reason"`
	AdjustedFinding    *ReviewFinding `json:"adjusted_finding"`
}

type VerificationResponse struct {
	Results []VerificationResult `json:"results"`
}

type FinalResponse struct {
	Comments    []FinalComment `json:"comments"`
	FinalReview FinalReview    `json:"final_review"`
	Metadata    Metadata       `json:"metadata,omitempty"`
}

type FinalComment struct {
	File           string `json:"file"`
	Line           int    `json:"line"`
	Severity       string `json:"severity"`
	Type           string `json:"type,omitempty"`
	DecisionReason string `json:"decision_reason"`
	Comment        string `json:"comment"`
}

type FinalReview struct {
	GiteaEvent   string `json:"gitea_event"`
	Status       string `json:"status"`
	Summary      string `json:"summary"`
	Observations string `json:"observations"`
}

type Metadata struct {
	PipelineVersion      string        `json:"pipeline_version"`
	PlannerUsed          bool          `json:"planner_used"`
	PlannerFallback      bool          `json:"planner_fallback"`
	ConsolidatorFallback bool          `json:"consolidator_fallback"`
	VerifierFallback     bool          `json:"verifier_fallback"`
	FormatterFallback    bool          `json:"formatter_fallback"`
	ReviewGroups         int           `json:"review_groups"`
	SuccessfulGroups     int           `json:"successful_groups"`
	FailedGroups         int           `json:"failed_groups"`
	RawFindings          int           `json:"raw_findings"`
	ConsolidatedFindings int           `json:"consolidated_findings"`
	ConfirmedFindings    int           `json:"confirmed_findings"`
	RejectedFindings     int           `json:"rejected_findings"`
	PartialReview        bool          `json:"partial_review"`
	DiffTruncated        bool          `json:"diff_truncated"`
	ReviewedFiles        int           `json:"reviewed_files"`
	TotalFiles           int           `json:"total_files"`
	StageMetrics         []StageMetric `json:"stage_metrics,omitempty"`
}

type StageMetric struct {
	Stage                  string `json:"stage"`
	DurationMillis         int64  `json:"duration_ms"`
	InputChars             int    `json:"input_chars"`
	EstimatedInputTokens   int    `json:"estimated_input_tokens"`
	RequestedOutputTokens  int    `json:"requested_output_tokens"`
	ActualPromptTokens     int    `json:"actual_prompt_tokens,omitempty"`
	ActualCompletionTokens int    `json:"actual_completion_tokens,omitempty"`
	ResponseChars          int    `json:"response_chars,omitempty"`
	Failed                 bool   `json:"failed,omitempty"`
}

type StageUsage struct {
	PromptTokens     int
	CompletionTokens int
}

type ChatFunc func(ctx context.Context, stage string, prompt string, maxOutputTokens int) (string, StageUsage, error)
