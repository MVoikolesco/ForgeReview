package review

import (
	"context"
	"database/sql"
	"encoding/json"

	"gitea-agents/internal/queue"
)

// Policy contains the review policy fields enforced by the current processor.
type Policy struct {
	MaxBlockChars         int
	MaxFilesPerBlock      int
	PublishManualReviews  bool
	AllowAutonomousReject bool
	PlannerEnabled        bool
	ConsolidatorEnabled   bool
	VerifierEnabled       bool
	FormatterEnabled      bool
	PlannerMaxTokens      int
	GroupMaxTokens        int
	ConsolidatorMaxTokens int
	VerifierMaxTokens     int
	FormatterMaxTokens    int
	ContextSafetyTokens   int
	MinimumConfidence     float64
	MaxParallelGroups     int
	MediumSeverityEvent   string
	PartialEvent          string
	ContractMaxAttempts   int
}

// Pending contains a generated result and the original job awaiting an
// operator's publication decision.
type Pending struct {
	Job    queue.ReviewJob
	Result Result
}

// SavePending stores the pending payload and transitions the review atomically.
// It returns false when cancellation won the race.
func (r *Repository) SavePending(
	ctx context.Context,
	id string,
	job queue.ReviewJob,
	result Result,
) (bool, error) {
	jobData, err := json.Marshal(job)
	if err != nil {
		return false, err
	}
	resultData, err := json.Marshal(result)
	if err != nil {
		return false, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	statusResult, err := tx.ExecContext(ctx, `UPDATE reviews SET status=?,updated_at=CURRENT_TIMESTAMP
		WHERE id=? AND status!=?`, StatusAwaitingApproval, id, StatusCancelled)
	if err != nil {
		return false, err
	}
	affected, err := statusResult.RowsAffected()
	if err != nil || affected == 0 {
		return false, err
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO pending_reviews(review_id,job_json,result_json)
		 VALUES(?,?,?)
		 ON CONFLICT(review_id) DO UPDATE SET
			job_json=excluded.job_json,
			result_json=excluded.result_json,
			updated_at=CURRENT_TIMESTAMP`,
		id,
		jobData,
		resultData,
	)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// Pending loads and decodes the publication awaiting approval for a review ID.
func (r *Repository) Pending(ctx context.Context, id string) (Pending, error) {
	var jobData, resultData string
	err := r.db.QueryRowContext(
		ctx,
		`SELECT job_json,result_json FROM pending_reviews WHERE review_id=?`,
		id,
	).Scan(&jobData, &resultData)
	if err != nil {
		return Pending{}, err
	}

	var pending Pending
	if err := json.Unmarshal([]byte(jobData), &pending.Job); err != nil {
		return Pending{}, err
	}
	if err := json.Unmarshal([]byte(resultData), &pending.Result); err != nil {
		return Pending{}, err
	}
	return pending, nil
}

// DeletePending removes a review's pending publication payload.
func (r *Repository) DeletePending(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM pending_reviews WHERE review_id=?`, id)
	return err
}

// Policy resolves the enabled repository-specific policy or default profile
// policy for a review job.
func (r *Repository) Policy(ctx context.Context, job queue.ReviewJob) (Policy, error) {
	profileID, err := r.profileID(ctx, job)
	if err != nil {
		return Policy{}, err
	}
	return r.PolicyForProfile(ctx, profileID)
}

// PolicyForProfile loads publication and semantic limits from the profile used
// by the immutable pipeline version. Stage-specific execution settings come
// from pipeline_stages.
func (r *Repository) PolicyForProfile(ctx context.Context, profileID *int64) (Policy, error) {
	var policy Policy
	var publish, reject, planner, consolidator, verifier, formatter int
	if profileID == nil {
		return policy, sql.ErrNoRows
	}
	err := r.db.QueryRowContext(
		ctx,
		`SELECT pol.max_block_chars, pol.max_files_per_block,
		        pol.publish_manual_reviews, pol.allow_autonomous_rejection,
		        pol.review_planner_enabled, pol.review_consolidator_enabled,
		        pol.review_verifier_enabled, pol.review_formatter_enabled,
		        pol.review_planner_max_output_tokens, pol.review_group_max_output_tokens,
		        pol.review_consolidator_max_output_tokens, pol.review_verifier_max_output_tokens,
		        pol.review_formatter_max_output_tokens, pol.review_context_safety_margin_tokens,
		        pol.review_min_publish_confidence, pol.review_max_parallel_groups,
		        pol.review_medium_severity_event, pol.review_partial_event, pol.review_final_retries
		 FROM review_profiles rp
		 JOIN review_policies pol ON pol.profile_id=rp.id
		 WHERE rp.id=? AND rp.is_enabled=1`,
		*profileID,
	).Scan(
		&policy.MaxBlockChars,
		&policy.MaxFilesPerBlock,
		&publish,
		&reject,
		&planner, &consolidator, &verifier, &formatter,
		&policy.PlannerMaxTokens, &policy.GroupMaxTokens,
		&policy.ConsolidatorMaxTokens, &policy.VerifierMaxTokens,
		&policy.FormatterMaxTokens, &policy.ContextSafetyTokens,
		&policy.MinimumConfidence, &policy.MaxParallelGroups,
		&policy.MediumSeverityEvent, &policy.PartialEvent, &policy.ContractMaxAttempts,
	)
	policy.PublishManualReviews = publish != 0
	policy.AllowAutonomousReject = reject != 0
	policy.PlannerEnabled = planner != 0
	policy.ConsolidatorEnabled = consolidator != 0
	policy.VerifierEnabled = verifier != 0
	policy.FormatterEnabled = formatter != 0
	return policy, err
}
