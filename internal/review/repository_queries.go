package review

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

const reviewColumns = `id,owner,repository,pull_request,status,source,
	result_json,error_message,created_at,updated_at,started_at,finished_at`

// Get returns one review with its parsed result and ordered execution steps.
func (r *Repository) Get(ctx context.Context, id string) (Review, error) {
	var item Review
	var result, errorMessage, created, updated, started, finished sql.NullString
	err := r.db.QueryRowContext(
		ctx,
		`SELECT `+reviewColumns+` FROM reviews WHERE id=?`,
		id,
	).Scan(
		&item.ID,
		&item.Owner,
		&item.Repository,
		&item.PullRequest,
		&item.Status,
		&item.Source,
		&result,
		&errorMessage,
		&created,
		&updated,
		&started,
		&finished,
	)
	if err != nil {
		return item, err
	}

	populateReview(&item, result, errorMessage, created, updated, started, finished)
	steps, err := r.Steps(ctx, id)
	if err != nil {
		return item, err
	}
	item.Steps = steps
	return item, nil
}

// List returns up to limit reviews ordered by their most recent update. Invalid
// limits fall back to 50 and the maximum accepted limit is 100.
func (r *Repository) List(ctx context.Context, limit int) ([]Review, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := r.db.QueryContext(
		ctx,
		`SELECT `+reviewColumns+` FROM reviews ORDER BY updated_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Review{}
	for rows.Next() {
		var item Review
		var result, errorMessage, created, updated, started, finished sql.NullString
		err := rows.Scan(
			&item.ID,
			&item.Owner,
			&item.Repository,
			&item.PullRequest,
			&item.Status,
			&item.Source,
			&result,
			&errorMessage,
			&created,
			&updated,
			&started,
			&finished,
		)
		if err != nil {
			return nil, err
		}
		populateReview(&item, result, errorMessage, created, updated, started, finished)
		items = append(items, item)
	}

	return items, rows.Err()
}

// Steps returns all execution steps for a review in insertion order.
func (r *Repository) Steps(ctx context.Context, id string) ([]Step, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id,review_id,step,status,message,metadata_json,started_at,
		        finished_at,duration_ms,error_message
		 FROM review_steps WHERE review_id=? ORDER BY id`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Step{}
	for rows.Next() {
		var step Step
		var metadata, started, finished, stepErr sql.NullString
		err := rows.Scan(
			&step.ID,
			&step.ReviewID,
			&step.Step,
			&step.Status,
			&step.Message,
			&metadata,
			&started,
			&finished,
			&step.DurationMS,
			&stepErr,
		)
		if err != nil {
			return nil, err
		}
		step.StartedAt = parseTime(started.String)
		step.FinishedAt = parseTimePtr(finished.String)
		step.Error = stepErr.String
		_ = json.Unmarshal([]byte(metadata.String), &step.Metadata)
		items = append(items, step)
	}

	return items, rows.Err()
}

// populateReview converts nullable database values into a Review value.
func populateReview(
	item *Review,
	result sql.NullString,
	errorMessage sql.NullString,
	created sql.NullString,
	updated sql.NullString,
	started sql.NullString,
	finished sql.NullString,
) {
	item.Error = errorMessage.String
	item.CreatedAt = parseTime(created.String)
	item.UpdatedAt = parseTime(updated.String)
	item.StartedAt = parseTimePtr(started.String)
	item.FinishedAt = parseTimePtr(finished.String)

	if result.String != "" {
		var value Result
		if json.Unmarshal([]byte(result.String), &value) == nil {
			item.Result = &value
		}
	}
}

// parseTime accepts SQLite's timestamp format and RFC3339Nano.
func parseTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	if parsed.IsZero() {
		parsed, _ = time.Parse("2006-01-02 15:04:05", value)
	}
	return parsed
}

// parseTimePtr parses a nullable timestamp and returns nil when absent/invalid.
func parseTimePtr(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed := parseTime(value)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}
