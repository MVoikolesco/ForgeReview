package gitea

import (
	"context"
	"database/sql"
	"fmt"

	"gitea-agents/internal/security"
)

// Resolver selects the Gitea client configured for a review job. It uses a
// fallback client when no database mapping or default instance exists.
type Resolver struct {
	db       *sql.DB
	fallback *Client
}

// NewResolver returns a resolver backed by db and the supplied legacy fallback
// client. A nil database always resolves to fallback.
func NewResolver(db *sql.DB, fallback *Client) *Resolver {
	return &Resolver{db: db, fallback: fallback}
}

// Resolve returns the client for instanceID, the owner/repo mapping, or the
// default enabled instance. It returns fallback when no configured instance
// exists and an error when a selected instance has no usable token.
func (r *Resolver) Resolve(
	ctx context.Context,
	instanceID int64,
	owner string,
	repo string,
) (*Client, error) {
	if r.db == nil {
		return r.fallback, nil
	}

	if instanceID == 0 {
		err := r.db.QueryRowContext(
			ctx,
			`SELECT gi.id
			 FROM gitea_instances gi
			 JOIN repositories rep ON rep.gitea_instance_id=gi.id
			 WHERE rep.owner=? AND rep.name=? AND gi.is_enabled=1
			 ORDER BY rep.id LIMIT 1`,
			owner,
			repo,
		).Scan(&instanceID)
		if err == sql.ErrNoRows {
			err = r.db.QueryRowContext(
				ctx,
				`SELECT id FROM gitea_instances
				 WHERE is_enabled=1
				 ORDER BY is_default DESC, id LIMIT 1`,
			).Scan(&instanceID)
		}
		if err == sql.ErrNoRows {
			return r.fallback, nil
		}
		if err != nil {
			return nil, err
		}
	}

	var baseURL, ciphertext string
	err := r.db.QueryRowContext(
		ctx,
		"SELECT base_url,token_ciphertext FROM gitea_instances WHERE id=? AND is_enabled=1",
		instanceID,
	).Scan(&baseURL, &ciphertext)
	if err != nil {
		return nil, fmt.Errorf("Gitea instance %d not found: %w", instanceID, err)
	}

	token, err := security.Decrypt(ciphertext)
	if err != nil || token == "" {
		return nil, fmt.Errorf("Gitea instance %d has no usable token", instanceID)
	}

	return New(baseURL, token), nil
}
