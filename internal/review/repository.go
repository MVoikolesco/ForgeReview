package review

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"gitea-agents/internal/queue"
	"time"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, id string, job queue.ReviewJob, source string) error {
	payload, _ := json.Marshal(job)
	_, err := r.db.ExecContext(ctx, `INSERT INTO reviews(id,owner,repository,pull_request,status,source,job_json) VALUES(?,?,?,?,?,?,?)`, id, job.Owner, job.Repository, job.PullRequest, StatusReceived, source, payload)
	return err
}
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

func (r *Repository) DefaultPrompt(ctx context.Context) (string, error) {
	var content string
	err := r.db.QueryRowContext(ctx, `SELECT p.content FROM review_prompts p JOIN review_profiles rp ON rp.id=p.profile_id WHERE rp.is_default=1 AND rp.is_enabled=1 AND p.is_active=1 ORDER BY p.version DESC, p.id DESC LIMIT 1`).Scan(&content)
	return content, err
}
func (r *Repository) SetStatus(ctx context.Context, id, status, message string) error {
	now := time.Now().UTC()
	var q string
	switch status {
	case StatusProcessing:
		q = `UPDATE reviews SET status=?,updated_at=?,started_at=COALESCE(started_at,?) WHERE id=?`
		return r.exec(ctx, q, status, now, now, id)
	case StatusCompleted, StatusFailed, StatusCancelled:
		q = `UPDATE reviews SET status=?,updated_at=?,finished_at=? WHERE id=?`
		if message != "" {
			q = `UPDATE reviews SET status=?,error_message=?,updated_at=?,finished_at=? WHERE id=?`
			return r.exec(ctx, q, status, message, now, now, id)
		}
		return r.exec(ctx, q, status, now, now, id)
	default:
		q = `UPDATE reviews SET status=?,updated_at=? WHERE id=?`
		if message != "" {
			q = `UPDATE reviews SET status=?,error_message=?,updated_at=? WHERE id=?`
			return r.exec(ctx, q, status, message, now, id)
		}
		return r.exec(ctx, q, status, now, id)
	}
}

func (r *Repository) Status(ctx context.Context, id string) (string, error) {
	var status string
	err := r.db.QueryRowContext(ctx, `SELECT status FROM reviews WHERE id=?`, id).Scan(&status)
	return status, err
}

type Policy struct {
	MaxBlockChars         int
	MaxFilesPerBlock      int
	PublishManualReviews  bool
	AllowAutonomousReject bool
}

type Pending struct {
	Job    queue.ReviewJob
	Result Result
}

func (r *Repository) SavePending(ctx context.Context, id string, job queue.ReviewJob, result Result) error {
	jobData, err := json.Marshal(job)
	if err != nil {
		return err
	}
	resultData, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO pending_reviews(review_id,job_json,result_json) VALUES(?,?,?) ON CONFLICT(review_id) DO UPDATE SET job_json=excluded.job_json,result_json=excluded.result_json,updated_at=CURRENT_TIMESTAMP`, id, jobData, resultData)
	return err
}

func (r *Repository) Pending(ctx context.Context, id string) (Pending, error) {
	var jobData, resultData string
	if err := r.db.QueryRowContext(ctx, `SELECT job_json,result_json FROM pending_reviews WHERE review_id=?`, id).Scan(&jobData, &resultData); err != nil {
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

func (r *Repository) DeletePending(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM pending_reviews WHERE review_id=?`, id)
	return err
}

func (r *Repository) Policy(ctx context.Context, job queue.ReviewJob) (Policy, error) {
	var p Policy
	var publish, reject int
	err := r.db.QueryRowContext(ctx, `
		SELECT pol.max_block_chars, pol.max_files_per_block,
		       pol.publish_manual_reviews, pol.allow_autonomous_rejection
		FROM review_profiles rp
		JOIN review_policies pol ON pol.profile_id=rp.id
		LEFT JOIN repositories rep ON rep.review_profile_id=rp.id
		  AND rep.full_name=? AND rep.is_enabled=1
		WHERE rp.is_enabled=1 AND (rep.id IS NOT NULL OR rp.is_default=1)
		ORDER BY rep.id DESC, rp.is_default DESC LIMIT 1`, job.Owner+"/"+job.Repository).
		Scan(&p.MaxBlockChars, &p.MaxFilesPerBlock, &publish, &reject)
	p.PublishManualReviews = publish != 0
	p.AllowAutonomousReject = reject != 0
	return p, err
}
func (r *Repository) exec(ctx context.Context, q string, args ...any) error {
	_, err := r.db.ExecContext(ctx, q, args...)
	return err
}
func (r *Repository) AddStep(ctx context.Context, id, step, status, message string, metadata map[string]any, started time.Time, finished *time.Time, duration int64, stepErr string) error {
	data, _ := json.Marshal(metadata)
	_, err := r.db.ExecContext(ctx, `INSERT INTO review_steps(review_id,step,status,message,metadata_json,started_at,finished_at,duration_ms,error_message) VALUES(?,?,?,?,?,?,?,?,?)`, id, step, status, message, data, started.UTC(), finished, duration, stepErr)
	return err
}
func (r *Repository) SaveResult(ctx context.Context, id string, result Result) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE reviews SET result_json=?,updated_at=? WHERE id=?`, data, time.Now().UTC(), id)
	return err
}
func (r *Repository) Get(ctx context.Context, id string) (Review, error) {
	var x Review
	var result, errorMessage, created, updated, started, finished sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id,owner,repository,pull_request,status,source,result_json,error_message,created_at,updated_at,started_at,finished_at FROM reviews WHERE id=?`, id).Scan(&x.ID, &x.Owner, &x.Repository, &x.PullRequest, &x.Status, &x.Source, &result, &errorMessage, &created, &updated, &started, &finished)
	if err != nil {
		return x, err
	}
	x.Error = errorMessage.String
	x.CreatedAt = parseTime(created.String)
	x.UpdatedAt = parseTime(updated.String)
	x.StartedAt = parseTimePtr(started.String)
	x.FinishedAt = parseTimePtr(finished.String)
	if result.String != "" {
		var value Result
		if json.Unmarshal([]byte(result.String), &value) == nil {
			x.Result = &value
		}
	}
	steps, err := r.Steps(ctx, id)
	if err != nil {
		return x, err
	}
	x.Steps = steps
	return x, nil
}
func (r *Repository) List(ctx context.Context, limit int) ([]Review, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,owner,repository,pull_request,status,source,result_json,error_message,created_at,updated_at,started_at,finished_at FROM reviews ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Review{}
	for rows.Next() {
		var x Review
		var result, errorMessage, created, updated, started, finished sql.NullString
		if err := rows.Scan(&x.ID, &x.Owner, &x.Repository, &x.PullRequest, &x.Status, &x.Source, &result, &errorMessage, &created, &updated, &started, &finished); err != nil {
			return nil, err
		}
		x.Error = errorMessage.String
		x.CreatedAt = parseTime(created.String)
		x.UpdatedAt = parseTime(updated.String)
		x.StartedAt = parseTimePtr(started.String)
		x.FinishedAt = parseTimePtr(finished.String)
		if result.String != "" {
			var v Result
			if json.Unmarshal([]byte(result.String), &v) == nil {
				x.Result = &v
			}
		}
		items = append(items, x)
	}
	return items, rows.Err()
}
func (r *Repository) Steps(ctx context.Context, id string) ([]Step, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,review_id,step,status,message,metadata_json,started_at,finished_at,duration_ms,error_message FROM review_steps WHERE review_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Step{}
	for rows.Next() {
		var s Step
		var metadata, started, finished, stepErr sql.NullString
		if err := rows.Scan(&s.ID, &s.ReviewID, &s.Step, &s.Status, &s.Message, &metadata, &started, &finished, &s.DurationMS, &stepErr); err != nil {
			return nil, err
		}
		s.StartedAt = parseTime(started.String)
		s.FinishedAt = parseTimePtr(finished.String)
		s.Error = stepErr.String
		_ = json.Unmarshal([]byte(metadata.String), &s.Metadata)
		items = append(items, s)
	}
	return items, rows.Err()
}
func parseTime(v string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, v)
	if t.IsZero() {
		t, _ = time.Parse("2006-01-02 15:04:05", v)
	}
	return t
}
func parseTimePtr(v string) *time.Time {
	if v == "" {
		return nil
	}
	t := parseTime(v)
	if t.IsZero() {
		return nil
	}
	return &t
}

var ErrNotFound = sql.ErrNoRows
var _ = fmt.Sprintf
