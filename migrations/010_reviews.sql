CREATE TABLE IF NOT EXISTS reviews (
    id TEXT PRIMARY KEY,
    owner TEXT NOT NULL,
    repository TEXT NOT NULL,
    pull_request INTEGER NOT NULL,
    status TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'webhook',
    job_json TEXT NOT NULL DEFAULT '{}',
    result_json TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TEXT,
    finished_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_reviews_updated_at ON reviews(updated_at DESC);
CREATE TABLE IF NOT EXISTS review_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    review_id TEXT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    step TEXT NOT NULL,
    status TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TEXT,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_review_steps_review ON review_steps(review_id, id);
