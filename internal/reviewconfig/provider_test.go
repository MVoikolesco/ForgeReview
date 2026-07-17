package reviewconfig

import (
	"context"
	"fmt"
	"gitea-agents/internal/config"
	"gitea-agents/internal/secrets"
	"gitea-agents/internal/store"
	"testing"
)

func TestSQLiteProviderSelectsRepositoryProfile(t *testing.T) {
	t.Setenv("GITEA_TOKEN_ENCRYPTION_KEY", "01234567890123456789012345678901")
	s, e := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Initialize(context.Background()); e != nil {
		t.Fatal(e)
	}
	key, _ := secrets.Encrypt("test-key", secrets.ConnectionKeyAAD(2))
	sql := fmt.Sprintf(`INSERT INTO ai_providers(id,name,display_name,base_url,auth_type) VALUES(1,'ollama','Ollama','http://ollama','none'),(2,'openrouter','OpenRouter','https://openrouter.ai/api/v1','bearer');
INSERT INTO ai_connections(id,provider_id,name,base_url,api_key_ciphertext) VALUES(1,1,'local','http://localhost:11434',''),(2,2,'cloud','https://openrouter.ai/api/v1',%q);
INSERT INTO ai_models(id,connection_id,provider_model_name,display_name) VALUES(1,1,'qwen','Qwen'),(2,2,'openai/gpt','GPT');
INSERT INTO model_parameters(model_id,temperature,top_p,keep_alive,timeout_seconds) VALUES(1,0.1,0.9,'5m',60),(2,0.2,0.8,'',60);
INSERT INTO review_profiles(id,name,model_id,is_default) VALUES(1,'default',1,1),(2,'repo',2,0);
INSERT INTO review_policies(profile_id,max_block_chars,max_files_per_block,review_concurrency) VALUES(1,4000,2,1),(2,8000,4,1);
INSERT INTO repositories(owner,name,full_name,review_profile_id) VALUES('acme','portal','acme/portal',2);`, key)
	if _, e = s.DB.Exec(sql); e != nil {
		t.Fatal(e)
	}
	p := SQLiteProvider{Store: s}
	c, e := p.GetConfig(context.Background(), "acme/portal")
	if e != nil {
		t.Fatal(e)
	}
	if c.Provider.Name != "openrouter" || c.Model.Name != "openai/gpt" {
		t.Fatalf("unexpected repository config: %+v", c)
	}
	if !c.Pipeline.PlannerEnabled || c.Pipeline.PlannerMaxOutputTokens != 2000 || c.Pipeline.GroupMaxOutputTokens != 3500 || c.Pipeline.MinimumPublishConfidence != 0.75 {
		t.Fatalf("unexpected pipeline defaults: %+v", c.Pipeline)
	}
	c, e = p.GetConfig(context.Background(), "other/repo")
	if e != nil {
		t.Fatal(e)
	}
	if c.Provider.Name != "ollama" {
		t.Fatalf("expected default Ollama, got %s", c.Provider.Name)
	}
}

func TestSQLiteProviderUsesDefaultProviderChainWhenProfileHasNoModel(t *testing.T) {
	t.Setenv("GITEA_TOKEN_ENCRYPTION_KEY", "01234567890123456789012345678901")
	s, e := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Initialize(context.Background()); e != nil {
		t.Fatal(e)
	}
	key, _ := secrets.Encrypt("test-key", secrets.ConnectionKeyAAD(2))
	sql := fmt.Sprintf(`INSERT INTO ai_providers(id,name,display_name,base_url,auth_type,is_default) VALUES(1,'ollama','Ollama','http://ollama','none',1),(2,'openrouter','OpenRouter','https://openrouter.ai/api/v1','bearer',0);
INSERT INTO ai_connections(id,provider_id,name,base_url,api_key_ciphertext,is_default) VALUES(1,1,'local','http://localhost:11434','',1),(2,2,'cloud','https://openrouter.ai/api/v1',%q,1);
INSERT INTO ai_models(id,connection_id,provider_model_name,display_name,is_default) VALUES(1,1,'qwen','Qwen',1),(2,2,'openai/gpt','GPT',1);
INSERT INTO model_parameters(model_id,temperature,top_p,keep_alive,timeout_seconds) VALUES(1,0.1,0.9,'5m',60),(2,0.2,0.8,'',60);
INSERT INTO review_profiles(id,name,model_id,is_default) VALUES(1,'default',NULL,1);
INSERT INTO review_policies(profile_id,max_block_chars,max_files_per_block,review_concurrency) VALUES(1,4000,2,1);`, key)
	if _, e = s.DB.Exec(sql); e != nil {
		t.Fatal(e)
	}
	p := SQLiteProvider{Store: s}
	c, e := p.GetConfig(context.Background(), "other/repo")
	if e != nil {
		t.Fatal(e)
	}
	if c.Provider.Name != "ollama" || c.Model.Name != "qwen" || c.Connection.Name != "local" {
		t.Fatalf("unexpected default chain config: %+v", c)
	}
	if _, e = s.DB.Exec("UPDATE ai_providers SET is_default=0; UPDATE ai_providers SET is_default=1 WHERE name='openrouter'"); e != nil {
		t.Fatal(e)
	}
	c, e = p.GetConfig(context.Background(), "other/repo")
	if e != nil {
		t.Fatal(e)
	}
	if c.Provider.Name != "openrouter" || c.Model.Name != "openai/gpt" || c.Connection.Name != "cloud" {
		t.Fatalf("unexpected switched config: %+v", c)
	}
}

func TestSQLiteProviderUsesOllamaCloudCiphertextAndFailsClosed(t *testing.T) {
	t.Setenv("GITEA_TOKEN_ENCRYPTION_KEY", "01234567890123456789012345678901")
	s, err := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	ciphertext, err := secrets.Encrypt("cloud-key", secrets.ConnectionKeyAAD(1))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(fmt.Sprintf(`
INSERT INTO ai_providers(id,name,display_name,base_url,auth_type,is_default) VALUES(1,'ollama','Ollama','http://ollama','none',1);
INSERT INTO ai_connections(id,provider_id,name,base_url,api_key_ciphertext,requires_auth,is_default) VALUES(1,1,'Ollama Cloud','https://ollama.com',%q,1,1);
INSERT INTO ai_models(id,connection_id,provider_model_name,display_name,is_default) VALUES(1,1,'cloud','Cloud',1);
INSERT INTO model_parameters(model_id,timeout_seconds) VALUES(1,60);
INSERT INTO review_profiles(id,name,model_id,is_default) VALUES(1,'default',1,1);
INSERT INTO review_policies(profile_id,max_block_chars,max_files_per_block,review_concurrency) VALUES(1,4000,2,1);`, ciphertext))
	if err != nil {
		t.Fatal(err)
	}
	p := SQLiteProvider{Store: s}
	c, err := p.GetConfig(context.Background(), "acme/repo")
	if err != nil || c.Connection.APIKey != "cloud-key" {
		t.Fatalf("config=%+v err=%v", c, err)
	}
	if _, err = s.DB.Exec("UPDATE ai_connections SET api_key_ciphertext='invalid'"); err != nil {
		t.Fatal(err)
	}
	if _, err = p.GetConfig(context.Background(), "acme/repo"); err == nil {
		t.Fatal("invalid cloud ciphertext was accepted")
	}
	if _, err = s.DB.Exec("UPDATE ai_connections SET requires_auth=0,api_key_ciphertext=''"); err != nil {
		t.Fatal(err)
	}
	if _, err = p.GetConfig(context.Background(), "acme/repo"); err == nil {
		t.Fatal("remote Ollama connection without ciphertext was accepted")
	}
}
