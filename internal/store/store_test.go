package store

import (
	"context"
	"gitea-agents/internal/config"
	"testing"
)

func TestMigrationsAndSeedAreIdempotent(t *testing.T) {
	s, e := Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Initialize(context.Background()); e != nil {
		t.Fatal(e)
	}
	c := config.Config{}
	if e = s.Seed(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	if e = s.Seed(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = s.DB.QueryRow("SELECT count(*) FROM ai_providers").Scan(&n); e != nil || n != 4 {
		t.Fatalf("providers=%d err=%v", n, e)
	}
	if e = s.DB.QueryRow("SELECT count(*) FROM ai_providers WHERE is_default=1 AND name='ollama'").Scan(&n); e != nil || n != 1 {
		t.Fatalf("default providers=%d err=%v", n, e)
	}
	rows, e := s.DB.Query("PRAGMA table_info(ai_connections)")
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	var ciphertext, legacy bool
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var def any
		if e = rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); e != nil {
			t.Fatal(e)
		}
		ciphertext = ciphertext || name == "api_key_ciphertext"
		legacy = legacy || name == "api_key"+"_env_name"
	}
	if !ciphertext || legacy {
		t.Fatalf("ai_connections secret columns ciphertext=%t legacy=%t", ciphertext, legacy)
	}
}

func TestAIConnectionSecretMigrationPreservesExistingRelationships(t *testing.T) {
	s, err := Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_, err = s.DB.Exec(`
CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE ai_providers (id INTEGER PRIMARY KEY, name TEXT NOT NULL, auth_type TEXT NOT NULL);
CREATE TABLE ai_connections (id INTEGER PRIMARY KEY, provider_id INTEGER NOT NULL REFERENCES ai_providers(id), name TEXT NOT NULL, base_url TEXT NOT NULL DEFAULT '', api_key_env_name TEXT NOT NULL DEFAULT '', organization_id TEXT NOT NULL DEFAULT '', project_id TEXT NOT NULL DEFAULT '', is_default INTEGER NOT NULL DEFAULT 0, is_enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, http_referer TEXT NOT NULL DEFAULT '', app_title TEXT NOT NULL DEFAULT '', UNIQUE(provider_id, name));
CREATE TABLE ai_models (id INTEGER PRIMARY KEY, connection_id INTEGER NOT NULL REFERENCES ai_connections(id));
CREATE TABLE model_parameters (id INTEGER PRIMARY KEY, model_id INTEGER NOT NULL REFERENCES ai_models(id));
CREATE TABLE review_profiles (id INTEGER PRIMARY KEY, model_id INTEGER REFERENCES ai_models(id));
	INSERT INTO ai_providers VALUES(1, 'ollama', 'none');
INSERT INTO ai_connections(id, provider_id, name) VALUES(7, 1, 'local');
INSERT INTO ai_models VALUES(11, 7);
INSERT INTO model_parameters VALUES(13, 11);
INSERT INTO review_profiles VALUES(17, 11);
INSERT INTO schema_migrations(name) VALUES ('001_admin_schema.sql'), ('002_integrity_indexes.sql'), ('003_review_execution_policy.sql'), ('004_openrouter_attribution.sql'), ('005_fix_openrouter_output_tokens.sql'), ('006_review_pipeline_policy.sql'), ('007_provider_default_config.sql'), ('008_gitea_token_ciphertext.sql');`)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	var connectionID, modelID, parameterID, profileID int
	if err = s.DB.QueryRow(`SELECT c.id, m.id, mp.id, rp.id FROM ai_connections c JOIN ai_models m ON m.connection_id=c.id JOIN model_parameters mp ON mp.model_id=m.id JOIN review_profiles rp ON rp.model_id=m.id`).Scan(&connectionID, &modelID, &parameterID, &profileID); err != nil {
		t.Fatal(err)
	}
	if connectionID != 7 || modelID != 11 || parameterID != 13 || profileID != 17 {
		t.Fatalf("relationships changed: connection=%d model=%d parameter=%d profile=%d", connectionID, modelID, parameterID, profileID)
	}
	rows, err := s.DB.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("migration left an invalid foreign-key relationship")
	}
}

func TestAIConnectionSecretMigrationFailsClosedForLegacyConnections(t *testing.T) {
	s, err := Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.DB.Exec(`
CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE ai_providers (id INTEGER PRIMARY KEY, name TEXT NOT NULL, auth_type TEXT NOT NULL);
CREATE TABLE ai_connections (id INTEGER PRIMARY KEY, provider_id INTEGER NOT NULL REFERENCES ai_providers(id), name TEXT NOT NULL, base_url TEXT NOT NULL DEFAULT '', api_key_env_name TEXT NOT NULL DEFAULT '', organization_id TEXT NOT NULL DEFAULT '', project_id TEXT NOT NULL DEFAULT '', is_default INTEGER NOT NULL DEFAULT 0, is_enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, http_referer TEXT NOT NULL DEFAULT '', app_title TEXT NOT NULL DEFAULT '');
INSERT INTO ai_providers VALUES(1, 'ollama', 'none'),(2, 'openrouter', 'bearer'),(3, 'google_gemini', 'api_key'),(4, 'groq', 'bearer');
INSERT INTO ai_connections(id,provider_id,name,base_url,is_enabled) VALUES(1,1,'Ollama local','http://localhost:11434',1),(2,1,'Ollama spoofed loopback','http://127.0.0.1.evil',1),(3,2,'OpenRouter','https://openrouter.ai/api/v1',1),(4,3,'Gemini','https://generativelanguage.googleapis.com',1),(5,4,'Groq','https://api.groq.com/openai/v1',1);
INSERT INTO schema_migrations(name) VALUES ('001_admin_schema.sql'), ('002_integrity_indexes.sql'), ('003_review_execution_policy.sql'), ('004_openrouter_attribution.sql'), ('005_fix_openrouter_output_tokens.sql'), ('006_review_pipeline_policy.sql'), ('007_provider_default_config.sql'), ('008_gitea_token_ciphertext.sql');`)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ id, requiresAuth, enabled int }{{1, 1, 0}, {2, 1, 0}, {3, 1, 0}, {4, 1, 0}, {5, 1, 0}} {
		var requiresAuth, enabled int
		if err = s.DB.QueryRow("SELECT requires_auth,is_enabled FROM ai_connections WHERE id=?", test.id).Scan(&requiresAuth, &enabled); err != nil || requiresAuth != test.requiresAuth || enabled != test.enabled {
			t.Fatalf("connection=%d requires_auth=%d enabled=%d err=%v", test.id, requiresAuth, enabled, err)
		}
	}
}
