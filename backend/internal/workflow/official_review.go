package workflow

const (
	// OfficialReviewWorkflowKey identifies the seeded, administrator-configured
	// Gitea pull-request review pipeline.
	OfficialReviewWorkflowKey     = "official-gitea-pr-review"
	OfficialReviewWorkflowVersion = 1
)

// OfficialReviewDefinition is the initial published review pipeline. Connection
// and PR fields intentionally remain empty: administrators select persisted
// Gitea and model-profile records before creating an executable derivative.
func OfficialReviewDefinition() Definition {
	return Definition{
		Key:         OfficialReviewWorkflowKey,
		Name:        "Official Gitea PR Review",
		Description: "Seeded full pull-request review pipeline with one native Gitea review publication.",
		Nodes: []Node{
			{Key: "trigger", Type: "trigger", Name: "Webhook / manual", Position: Position{X: 40, Y: 280}},
			{Key: "fetch", Type: "fetch", Name: "Buscar dados do PR", Position: Position{X: 315, Y: 280}, Config: map[string]any{"integration": "", "owner": "", "repo": "", "pull_request": 0}},
			{Key: "filter", Type: "filter", Name: "Filtrar arquivos", Position: Position{X: 610, Y: 125}, Config: map[string]any{"include_extensions": []string{".go", ".ts", ".tsx", ".php"}, "ignore_generated": true}},
			{Key: "group", Type: "group", Name: "Agrupar arquivos", Position: Position{X: 900, Y: 125}, Config: map[string]any{"max_files": 8, "max_characters": 12000, "group_by_extension": true}},
			{Key: "loop", Type: "loop", Name: "Revisar cada grupo", Position: Position{X: 1190, Y: 125}, Config: map[string]any{"max_iterations": 20, "concurrency": 1, "on_error": "fail"}},
			{Key: "template", Type: "template", Name: "Prompt de review", Position: Position{X: 1480, Y: 125}, Config: map[string]any{"template": "Analise este grupo de arquivos e responda somente uma lista JSON de achados."}},
			{Key: "model", Type: "model", Name: "Modelo de review", Position: Position{X: 1775, Y: 125}, Config: map[string]any{"model_profile": "", "max_tokens": 2000, "retry_limit": 0, "retry_delay_ms": 0}},
			{Key: "validate", Type: "validate", Name: "Validar resposta", Position: Position{X: 2070, Y: 125}, Config: map[string]any{"validate_paths": true}},
			{Key: "response-filter", Type: "response_filter", Name: "Filtrar achados", Position: Position{X: 2365, Y: 125}, Config: map[string]any{"minimum_severity": "medium"}},
			{Key: "consolidate", Type: "consolidate", Name: "Consolidar review", Position: Position{X: 2070, Y: 480}},
			{Key: "format", Type: "format", Name: "Formatar review", Position: Position{X: 2365, Y: 480}},
			{Key: "publish", Type: "publish", Name: "Publicar no Gitea", Position: Position{X: 2660, Y: 480}, Config: map[string]any{"integration": "", "owner": "", "repo": "", "pull_request": 0, "medium_severity_event": "COMMENT", "allow_autonomous_rejection": false}},
		},
		Edges: []Edge{
			{Key: "trigger-fetch", FromNode: "trigger", FromPort: "event", ToNode: "fetch", ToPort: "event"},
			{Key: "fetch-filter", FromNode: "fetch", FromPort: "files", ToNode: "filter", ToPort: "files"},
			{Key: "filter-group", FromNode: "filter", FromPort: "files", ToNode: "group", ToPort: "files"},
			{Key: "group-loop", FromNode: "group", FromPort: "groups", ToNode: "loop", ToPort: "items"},
			{Key: "loop-template", FromNode: "loop", FromPort: "item", ToNode: "template", ToPort: "context"},
			{Key: "template-model", FromNode: "template", FromPort: "prompt", ToNode: "model", ToPort: "prompt"},
			{Key: "model-validate", FromNode: "model", FromPort: "response", ToNode: "validate", ToPort: "response"},
			{Key: "loop-validate", FromNode: "loop", FromPort: "item", ToNode: "validate", ToPort: "files"},
			{Key: "validate-response-filter", FromNode: "validate", FromPort: "valid", ToNode: "response-filter", ToPort: "response"},
			{Key: "loop-consolidate", FromNode: "loop", FromPort: "results", ToNode: "consolidate", ToPort: "comments"},
			{Key: "consolidate-format", FromNode: "consolidate", FromPort: "review", ToNode: "format", ToPort: "review"},
			{Key: "format-publish", FromNode: "format", FromPort: "formatted", ToNode: "publish", ToPort: "formatted_review"},
		},
	}
}
