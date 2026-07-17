package gitea

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"gitea-agents/internal/secrets"
)

// ClientResolver selects the configured Gitea instance for a repository. The
// environment client is deliberately kept as the fallback for legacy jobs.
type ClientResolver struct {
	db       *sql.DB
	fallback *Client
}

func NewClientResolver(db *sql.DB, baseURL, token string) *ClientResolver {
	return &ClientResolver{db: db, fallback: NewClient(baseURL, token)}
}

func (r *ClientResolver) Resolve(ctx context.Context, instanceID int64, owner, repo string) (*Client, error) {
	if r.db == nil {
		return r.fallback, nil
	}
	if instanceID == 0 {
		if err := r.db.QueryRowContext(ctx, `SELECT gi.id FROM gitea_instances gi JOIN repositories rep ON rep.gitea_instance_id=gi.id WHERE rep.owner=? AND rep.name=? ORDER BY gi.id LIMIT 1`, owner, repo).Scan(&instanceID); err != nil {
			if err != sql.ErrNoRows {
				return nil, err
			}
			if err := r.db.QueryRowContext(ctx, `SELECT id FROM gitea_instances WHERE is_enabled=1 ORDER BY is_default DESC, id LIMIT 1`).Scan(&instanceID); err != nil {
				if err == sql.ErrNoRows {
					return r.fallback, nil
				}
				return nil, err
			}
		}
	}

	var base, tokenCiphertext, tokenEnv string
	if err := r.db.QueryRowContext(ctx, `SELECT base_url,token_ciphertext,token_env_name FROM gitea_instances WHERE id=?`, instanceID).Scan(&base, &tokenCiphertext, &tokenEnv); err != nil {
		return nil, fmt.Errorf("Gitea instance %d not found: %w", instanceID, err)
	}
	token, err := decryptToken(tokenCiphertext)
	if err != nil {
		token = os.Getenv(tokenEnv)
	}
	if token == "" {
		return nil, fmt.Errorf("Gitea instance %d has no usable token", instanceID)
	}
	return NewClient(base, token), nil
}

func decryptToken(value string) (string, error) {
	return secrets.Decrypt(value, nil)
}
