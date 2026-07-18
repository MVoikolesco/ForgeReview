package review

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"gitea-agents/internal/queue"
)

// Repository persists review state, execution steps, results, policies, and
// pending publications in SQLite.
type Repository struct {
	db *sql.DB
}

// NewRepository returns a review repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Create stores a newly received review and its original queue job. It returns
// the SQLite insert error, if any.
func (r *Repository) Create(
	ctx context.Context,
	id string,
	job queue.ReviewJob,
	source string,
) error {
	payload, _ := json.Marshal(job)
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO reviews(id,owner,repository,pull_request,status,source,job_json)
		 VALUES(?,?,?,?,?,?,?)`,
		id,
		job.Owner,
		job.Repository,
		job.PullRequest,
		StatusReceived,
		source,
		payload,
	)
	return err
}

// Job loads and decodes the original ReviewJob stored for a review ID.
func (r *Repository) Job(ctx context.Context, id string) (queue.ReviewJob, error) {
	var payload string
	if err := r.db.QueryRowContext(ctx, `SELECT job_json FROM reviews WHERE id=?`, id).Scan(&payload); err != nil {
		return queue.ReviewJob{}, err
	}

	var job queue.ReviewJob
	if err := json.Unmarshal([]byte(payload), &job); err != nil {
		return job, err
	}
	return job, nil
}

// DefaultPrompt returns the newest active prompt for the enabled default
// profile. It returns sql.ErrNoRows when no prompt is configured.
func (r *Repository) DefaultPrompt(ctx context.Context) (string, error) {
	var content string
	err := r.db.QueryRowContext(
		ctx,
		`SELECT p.content
		 FROM review_prompts p
		 JOIN review_profiles rp ON rp.id=p.profile_id
		 WHERE rp.is_default=1 AND rp.is_enabled=1 AND p.is_active=1
		 ORDER BY p.version DESC, p.id DESC LIMIT 1`,
	).Scan(&content)
	return content, err
}

// SetStatus updates a review status and associated timestamps. Terminal status
// updates also persist message as the review error when it is non-empty.
func (r *Repository) SetStatus(ctx context.Context, id, status, message string) error {
	now := time.Now().UTC()
	switch status {
	case StatusProcessing:
		return r.exec(
			ctx,
			`UPDATE reviews SET status=?,updated_at=?,started_at=COALESCE(started_at,?) WHERE id=?`,
			status,
			now,
			now,
			id,
		)
	case StatusCompleted, StatusFailed, StatusCancelled:
		if message != "" {
			return r.exec(
				ctx,
				`UPDATE reviews SET status=?,error_message=?,updated_at=?,finished_at=? WHERE id=?`,
				status,
				message,
				now,
				now,
				id,
			)
		}
		return r.exec(
			ctx,
			`UPDATE reviews SET status=?,updated_at=?,finished_at=? WHERE id=?`,
			status,
			now,
			now,
			id,
		)
	default:
		if message != "" {
			return r.exec(
				ctx,
				`UPDATE reviews SET status=?,error_message=?,updated_at=? WHERE id=?`,
				status,
				message,
				now,
				id,
			)
		}
		return r.exec(
			ctx,
			`UPDATE reviews SET status=?,updated_at=? WHERE id=?`,
			status,
			now,
			id,
		)
	}
}

// Status returns the persisted status for a review ID.
func (r *Repository) Status(ctx context.Context, id string) (string, error) {
	var status string
	err := r.db.QueryRowContext(ctx, `SELECT status FROM reviews WHERE id=?`, id).Scan(&status)
	return status, err
}

// AddStep appends one execution step and its timing, metadata, and optional
// error to a review.
func (r *Repository) AddStep(
	ctx context.Context,
	id string,
	step string,
	status string,
	message string,
	metadata map[string]any,
	started time.Time,
	finished *time.Time,
	duration int64,
	stepErr string,
) error {
	data, _ := json.Marshal(metadata)
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO review_steps(
			review_id,step,status,message,metadata_json,started_at,finished_at,duration_ms,error_message
		) VALUES(?,?,?,?,?,?,?,?,?)`,
		id,
		step,
		status,
		message,
		data,
		started.UTC(),
		finished,
		duration,
		stepErr,
	)
	return err
}

// SaveResult serializes and stores the final provider-independent review result.
func (r *Repository) SaveResult(ctx context.Context, id string, result Result) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(
		ctx,
		`UPDATE reviews SET result_json=?,updated_at=? WHERE id=?`,
		data,
		time.Now().UTC(),
		id,
	)
	return err
}

// exec executes a repository statement and returns its database error.
func (r *Repository) exec(ctx context.Context, query string, args ...any) error {
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// ErrNotFound aliases sql.ErrNoRows for repository consumers.
var ErrNotFound = sql.ErrNoRows
