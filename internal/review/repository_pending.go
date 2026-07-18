package review

import (
	"context"
	"encoding/json"

	"gitea-agents/internal/queue"
)

// Policy contains the review policy fields enforced by the current processor.
type Policy struct {
	MaxBlockChars         int
	MaxFilesPerBlock      int
	PublishManualReviews  bool
	AllowAutonomousReject bool
}

// Pending contains a generated result and the original job awaiting an
// operator's publication decision.
type Pending struct {
	Job    queue.ReviewJob
	Result Result
}

// SavePending stores or replaces the publication awaiting approval for a review.
func (r *Repository) SavePending(
	ctx context.Context,
	id string,
	job queue.ReviewJob,
	result Result,
) error {
	jobData, err := json.Marshal(job)
	if err != nil {
		return err
	}
	resultData, err := json.Marshal(result)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(
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
	return err
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
	var policy Policy
	var publish, reject int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT pol.max_block_chars, pol.max_files_per_block,
		        pol.publish_manual_reviews, pol.allow_autonomous_rejection
		 FROM review_profiles rp
		 JOIN review_policies pol ON pol.profile_id=rp.id
		 LEFT JOIN repositories rep ON rep.review_profile_id=rp.id
		   AND rep.full_name=? AND rep.is_enabled=1
		 WHERE rp.is_enabled=1 AND (rep.id IS NOT NULL OR rp.is_default=1)
		 ORDER BY rep.id DESC, rp.is_default DESC LIMIT 1`,
		job.Owner+"/"+job.Repository,
	).Scan(
		&policy.MaxBlockChars,
		&policy.MaxFilesPerBlock,
		&publish,
		&reject,
	)
	policy.PublishManualReviews = publish != 0
	policy.AllowAutonomousReject = reject != 0
	return policy, err
}
