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

// StageExecutionLogs returns all persisted execution and retry rows for one review stage.
func (r *Repository) StageExecutionLogs(ctx context.Context, reviewID, stageKey string) ([]StageExecutionLog, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT se.id,se.stage_key,se.attempt,se.status,se.artifact_type,se.metadata_json,
		        se.started_at,se.finished_at,se.duration_ms,se.error_message
		 FROM stage_executions se
		 JOIN pipeline_executions pe ON pe.id=se.pipeline_execution_id
		 WHERE pe.review_id=? AND se.stage_key=?
		 ORDER BY se.started_at,se.id`,
		reviewID,
		stageKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []StageExecutionLog{}
	ids := make([]int64, 0, 8)
	byID := map[int64]int{}
	for rows.Next() {
		var item StageExecutionLog
		var metadata, started, finished, errorText sql.NullString
		if err = rows.Scan(
			&item.ID,
			&item.StageKey,
			&item.Attempt,
			&item.Status,
			&item.ArtifactType,
			&metadata,
			&started,
			&finished,
			&item.DurationMS,
			&errorText,
		); err != nil {
			return nil, err
		}
		item.StartedAt = parseTime(started.String)
		item.FinishedAt = parseTimePtr(finished.String)
		item.Error = errorText.String
		_ = json.Unmarshal([]byte(metadata.String), &item.Metadata)
		byID[item.ID] = len(items)
		ids = append(ids, item.ID)
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return items, nil
	}

	artifactRows, err := r.db.QueryContext(
		ctx,
		`SELECT stage_execution_id,artifact_type,payload_json
		 FROM stage_artifacts
		 WHERE stage_execution_id IN (
		   SELECT se.id FROM stage_executions se
		   JOIN pipeline_executions pe ON pe.id=se.pipeline_execution_id
		   WHERE pe.review_id=? AND se.stage_key=?
		 )
		 ORDER BY id`,
		reviewID,
		stageKey,
	)
	if err != nil {
		return nil, err
	}
	defer artifactRows.Close()
	for artifactRows.Next() {
		var stageExecutionID int64
		var artifactType, payload sql.NullString
		if err = artifactRows.Scan(&stageExecutionID, &artifactType, &payload); err != nil {
			return nil, err
		}
		index, ok := byID[stageExecutionID]
		if !ok {
			continue
		}
		artifact := StageArtifact{Type: artifactType.String}
		if payload.String != "" {
			var value any
			if json.Unmarshal([]byte(payload.String), &value) == nil {
				artifact.Payload = value
			} else {
				artifact.Payload = payload.String
			}
		}
		items[index].Artifacts = append(items[index].Artifacts, artifact)
	}
	return items, artifactRows.Err()
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
