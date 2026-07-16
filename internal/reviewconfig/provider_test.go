package reviewconfig

import (
	"context"
	"gitea-agents/internal/config"
	"gitea-agents/internal/store"
	"testing"
)

func TestSQLiteProviderSelectsRepositoryProfile(t *testing.T) {
	s, e := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Initialize(context.Background()); e != nil {
		t.Fatal(e)
	}
	sql := `INSERT INTO ai_providers(id,name,display_name,base_url,auth_type) VALUES(1,'ollama','Ollama','http://ollama','none'),(2,'openrouter','OpenRouter','https://openrouter.ai/api/v1','bearer');
INSERT INTO ai_connections(id,provider_id,name,base_url,api_key_env_name) VALUES(1,1,'local','http://ollama',''),(2,2,'cloud','https://openrouter.ai/api/v1','OPENROUTER_API_KEY');
INSERT INTO ai_models(id,connection_id,provider_model_name,display_name) VALUES(1,1,'qwen','Qwen'),(2,2,'openai/gpt','GPT');
INSERT INTO model_parameters(model_id,temperature,top_p,keep_alive,timeout_seconds) VALUES(1,0.1,0.9,'5m',60),(2,0.2,0.8,'',60);
INSERT INTO review_profiles(id,name,model_id,is_default) VALUES(1,'default',1,1),(2,'repo',2,0);
INSERT INTO review_policies(profile_id,max_block_chars,max_files_per_block,review_concurrency) VALUES(1,4000,2,1),(2,8000,4,1);
INSERT INTO repositories(owner,name,full_name,review_profile_id) VALUES('acme','portal','acme/portal',2);`
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
