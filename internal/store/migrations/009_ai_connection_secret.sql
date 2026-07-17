-- SQLite cannot drop a column in all supported deployments. Rebuild the table
-- while retaining ids and dependent foreign-key relationships.
CREATE TABLE ai_connections_new (
    id INTEGER PRIMARY KEY,
    provider_id INTEGER NOT NULL REFERENCES ai_providers (id),
    name TEXT NOT NULL,
    base_url TEXT NOT NULL DEFAULT '',
    api_key_ciphertext TEXT NOT NULL DEFAULT '',
    requires_auth INTEGER NOT NULL DEFAULT 0,
    organization_id TEXT NOT NULL DEFAULT '',
    project_id TEXT NOT NULL DEFAULT '',
    is_default INTEGER NOT NULL DEFAULT 0,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    http_referer TEXT NOT NULL DEFAULT '',
    app_title TEXT NOT NULL DEFAULT '',
    UNIQUE (provider_id, name)
);
-- Legacy rows have no ciphertext to prove their authentication state. SQLite
-- cannot safely reproduce Go's parsed net.IP loopback classifier, so every
-- such row is fail-closed until an operator supplies a key and re-enables it.
INSERT INTO ai_connections_new(id,provider_id,name,base_url,requires_auth,organization_id,project_id,is_default,is_enabled,created_at,updated_at,http_referer,app_title)
SELECT c.id,c.provider_id,c.name,c.base_url,1,
       c.organization_id,c.project_id,c.is_default,0,
       c.created_at,c.updated_at,c.http_referer,c.app_title
FROM ai_connections c;
DROP TABLE ai_connections;
ALTER TABLE ai_connections_new RENAME TO ai_connections;
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_connections_default_provider ON ai_connections (provider_id) WHERE is_default = 1;
