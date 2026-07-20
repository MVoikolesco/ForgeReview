package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitea-agents/internal/config"
	databasepkg "gitea-agents/internal/database"

	"github.com/gin-gonic/gin"
)

func TestReviewSettingsReturnsPublishedPipelineCatalog(t *testing.T) {
	db, err := databasepkg.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err = db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = db.SQL.ExecContext(ctx, `
		INSERT INTO ai_connections(provider_id,name,base_url,is_enabled)
		VALUES((SELECT id FROM ai_providers WHERE name='ollama'),'Local','http://localhost:11434',1);
		INSERT INTO ai_models(connection_id,provider_model_name,display_name,is_enabled)
		VALUES(last_insert_rowid(),'qwen-test','Qwen Test',1);
		INSERT INTO review_profiles(name,description,model_id,is_default,is_enabled)
		VALUES('Default','Perfil principal',last_insert_rowid(),1,1);
		INSERT INTO review_policies(profile_id,max_block_chars,max_files_per_block)
		VALUES(last_insert_rowid(),4000,2);
		INSERT INTO review_prompts(profile_id,name,prompt_type,content,version,is_active)
		VALUES((SELECT id FROM review_profiles WHERE name='Default'),'Antigo','review','ignorar',1,0);
		INSERT INTO review_prompts(profile_id,name,prompt_type,content,version,is_active)
		VALUES((SELECT id FROM review_profiles WHERE name='Default'),'Ativo','review','revisar',2,1);`); err != nil {
		t.Fatal(err)
	}
	if err = db.Seed(ctx); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAdminHandler(db.SQL, config.Config{}, nil, nil, nil)
	router.GET("/review/settings", handler.reviewSettings)
	request := httptest.NewRequest(http.MethodGet, "/review/settings", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	var payload reviewSettingsResponse
	if err = json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Pipelines) != 2 || len(payload.Pipelines[0].Stages) != 7 || len(payload.StageCatalog) != 9 {
		t.Fatalf("unexpected settings payload: pipelines=%d stages=%d catalog=%d", len(payload.Pipelines), len(payload.Pipelines[0].Stages), len(payload.StageCatalog))
	}
	if len(payload.Profiles) != 1 || payload.Profiles[0].Prompt == nil || payload.Profiles[0].Prompt.Name != "Ativo" {
		t.Fatalf("unexpected profile settings: %#v", payload.Profiles)
	}
}
