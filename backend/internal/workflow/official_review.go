package workflow

const (
	// OfficialReviewWorkflowKey identifies the seeded, administrator-configured
	// Gitea pull-request review pipeline.
	OfficialReviewWorkflowKey     = "official-gitea-pr-review"
	OfficialReviewWorkflowVersion = 1
)

// OfficialReviewDefinition keeps one visible review recipe whose configured
// instances are expanded only by the runtime.
func OfficialReviewDefinition() Definition {
	return PreviousParameterizedReviewRecipeDefinition()
}

// PreviousManualEdgeRoutingDefinition recognizes the last seeded definition
// that carried custom endpoint sides and curvature points. Those JSON fields
// are intentionally ignored now that routing is again delegated to the canvas.
func PreviousManualEdgeRoutingDefinition() Definition {
	definition := PreviousSpacedReviewRecipeDefinition()
	definition.Description = "Pipeline semântica com receita reutilizável, lados configuráveis nas conexões e curvas suaves editáveis por pontos persistidos."
	return definition
}

// PreviousSpacedReviewRecipeDefinition recognizes the seed where the final
// processing cards were moved beyond the enlarged recipe container.
func PreviousSpacedReviewRecipeDefinition() Definition {
	definition := PreviousDirectedReviewRecipeDefinition()
	definition.Description = "Pipeline semântica com receita visual reutilizável, fluxo interno orientado, contexto em barramento separado e etapas finais sem sobreposição."
	for index := range definition.Nodes {
		switch definition.Nodes[index].Key {
		case "merge-reviewers":
			definition.Nodes[index].Position = Position{X: 2600, Y: 300}
		case "consolidate":
			definition.Nodes[index].Position = Position{X: 2880, Y: 300}
		case "format":
			definition.Nodes[index].Position = Position{X: 3160, Y: 300}
		case "publish":
			definition.Nodes[index].Position = Position{X: 3440, Y: 300}
		}
	}
	return definition
}

// PreviousDirectedReviewRecipeDefinition recognizes the seed that corrected
// internal handle direction and made every recipe stage advance to the right,
// before the downstream cards were moved beyond the enlarged container.
func PreviousDirectedReviewRecipeDefinition() Definition {
	definition := PreviousParameterizedReviewRecipeDefinition()
	definition.Description = "Pipeline semântica com uma receita visual reutilizável de revisão, fluxo interno orientado da esquerda para a direita, instâncias configuráveis, contratos versionados e publicação nativa no Gitea."
	for index := range definition.Nodes {
		switch definition.Nodes[index].Key {
		case "review-recipe":
			definition.Nodes[index].Size = &NodeSize{Width: 1040, Height: 410}
		case "review-template":
			definition.Nodes[index].Position = Position{X: 60, Y: 105}
		case "review-model":
			definition.Nodes[index].Position = Position{X: 255, Y: 65}
		case "review-validate":
			definition.Nodes[index].Position = Position{X: 455, Y: 105}
		case "review-confirm":
			definition.Nodes[index].Position = Position{X: 650, Y: 205}
		case "review-filter":
			definition.Nodes[index].Position = Position{X: 845, Y: 105}
		}
	}
	return definition
}

// PreviousParameterizedReviewRecipeDefinition recognizes the first official
// seed with one visible recipe and runtime-only reviewer instances.
func PreviousParameterizedReviewRecipeDefinition() Definition {
	reviewers := officialReviewerSpecs()
	instances := make([]map[string]any, 0, len(reviewers))
	for _, reviewer := range reviewers {
		instances = append(instances, map[string]any{
			"key":                     reviewer.Key,
			"name":                    reviewer.Name,
			"enabled":                 true,
			"template":                reviewer.Prompt,
			"review_contract_key":     "review." + reviewer.Key,
			"review_contract_version": 2,
			"model_profile":           "",
			"validator_model_profile": "",
			"minimum_severity":        "medium",
		})
	}
	return Definition{
		Key:         OfficialReviewWorkflowKey,
		Name:        "Official Gitea PR Review",
		Description: "Pipeline semântica com uma receita visual reutilizável de revisão, instâncias configuráveis, contratos versionados, validação independente e publicação nativa no Gitea.",
		Nodes: []Node{
			{Key: "trigger", Type: "trigger", Name: "Webhook Gitea", Position: Position{X: 40, Y: 300}, Config: map[string]any{"mode": "webhook"}},
			{Key: "fetch", Type: "fetch", Name: "Buscar dados do PR", Position: Position{X: 315, Y: 300}, Config: map[string]any{"integration": ""}},
			{Key: "filter", Type: "filter", Name: "Filtrar arquivos", Position: Position{X: 610, Y: 300}, Config: map[string]any{"include_extensions": []string{".go", ".ts", ".tsx", ".php"}, "ignore_generated": true}},
			{Key: "semantic-units", Type: "semantic_units", Name: "Criar unidades semânticas", Position: Position{X: 900, Y: 300}, Config: map[string]any{"max_units": 200, "max_characters": 50000, "context_lines": 4}},
			{Key: "loop", Type: "loop", Name: "Revisar cada unidade", Position: Position{X: 1190, Y: 300}, Config: map[string]any{"max_iterations": 200, "concurrency": 1, "on_error": "fail"}},
			{
				Key: "review-recipe", Type: "subpipeline", Name: "Receita de revisão · 6 instâncias",
				Position: Position{X: 1450, Y: 90},
				Size:     &NodeSize{Width: 670, Height: 470},
				Config: map[string]any{
					"input_ports":  []Port{{Key: "context", Label: "Unidade", Contract: "any", Required: true}},
					"output_ports": []Port{{Key: "comments", Label: "Comentários", Contract: "list", Required: true}},
					"instances":    instances,
				},
			},
			{
				Key: "review-template", Type: "template", Name: "Prompt + contrato",
				ParentKey: "review-recipe", Position: Position{X: 40, Y: 85},
				Config: map[string]any{"template": reviewers[0].Prompt, "review_contract_key": "review.security", "review_contract_version": 2},
			},
			{
				Key: "review-model", Type: "model", Name: "Executar modelo",
				ParentKey: "review-recipe", Position: Position{X: 250, Y: 65},
				Config: map[string]any{"model_profile": "", "max_tokens": 2000, "retry_limit": 0, "retry_delay_ms": 0},
			},
			{
				Key: "review-validate", Type: "validate", Name: "Validar contrato",
				ParentKey: "review-recipe", Position: Position{X: 455, Y: 105},
				Config: map[string]any{"validate_paths": true},
			},
			{
				Key: "review-confirm", Type: "candidate_validator", Name: "Confirmar achados",
				ParentKey: "review-recipe", Position: Position{X: 350, Y: 285},
				Config: map[string]any{"model_profile": "", "max_tokens": 300, "temperature": 0.0, "timeout_seconds": 120},
			},
			{
				Key: "review-filter", Type: "response_filter", Name: "Aplicar filtro",
				ParentKey: "review-recipe", Position: Position{X: 115, Y: 285},
				Config: map[string]any{"minimum_severity": "medium"},
			},
			{Key: "merge-reviewers", Type: "merge", Name: "Unir revisões", Position: Position{X: 2210, Y: 300}, Config: map[string]any{"mode": "all"}},
			{Key: "consolidate", Type: "consolidate", Name: "Consolidar reviewers", Position: Position{X: 2490, Y: 300}},
			{Key: "format", Type: "format", Name: "Formatar review", Position: Position{X: 2770, Y: 300}},
			{Key: "publish", Type: "publish", Name: "Publicar no Gitea", Position: Position{X: 3050, Y: 300}, Config: map[string]any{"integration": "", "medium_severity_event": "COMMENT", "allow_autonomous_rejection": false}},
		},
		Edges: []Edge{
			{Key: "trigger-fetch", FromNode: "trigger", FromPort: "event", ToNode: "fetch", ToPort: "event"},
			{Key: "fetch-filter", FromNode: "fetch", FromPort: "files", ToNode: "filter", ToPort: "files"},
			{Key: "filter-semantic-units", FromNode: "filter", FromPort: "files", ToNode: "semantic-units", ToPort: "files"},
			{Key: "semantic-units-loop", FromNode: "semantic-units", FromPort: "units", ToNode: "loop", ToPort: "items"},
			{Key: "loop-review-recipe", FromNode: "loop", FromPort: "item", ToNode: "review-recipe", ToPort: "entry:context"},
			{Key: "recipe-template", FromNode: "review-recipe", FromPort: "entry:context", ToNode: "review-template", ToPort: "context"},
			{Key: "recipe-validate-files", FromNode: "review-recipe", FromPort: "entry:context", ToNode: "review-validate", ToPort: "files"},
			{Key: "recipe-confirm-files", FromNode: "review-recipe", FromPort: "entry:context", ToNode: "review-confirm", ToPort: "files"},
			{Key: "template-model", FromNode: "review-template", FromPort: "prompt", ToNode: "review-model", ToPort: "prompt"},
			{Key: "model-validate", FromNode: "review-model", FromPort: "response", ToNode: "review-validate", ToPort: "response"},
			{Key: "validate-confirm", FromNode: "review-validate", FromPort: "valid", ToNode: "review-confirm", ToPort: "candidates"},
			{Key: "confirm-filter", FromNode: "review-confirm", FromPort: "confirmed", ToNode: "review-filter", ToPort: "response"},
			{Key: "filter-recipe", FromNode: "review-filter", FromPort: "comments", ToNode: "review-recipe", ToPort: "exit:comments"},
			{Key: "recipe-merge", FromNode: "review-recipe", FromPort: "exit:comments", ToNode: "merge-reviewers", ToPort: "inputs"},
			{Key: "loop-consolidate", FromNode: "loop", FromPort: "results", ToNode: "consolidate", ToPort: "comments"},
			{Key: "consolidate-format", FromNode: "consolidate", FromPort: "review", ToNode: "format", ToPort: "review"},
			{Key: "format-publish", FromNode: "format", FromPort: "formatted", ToNode: "publish", ToPort: "formatted_review"},
			{Key: "fetch-publish-target", FromNode: "fetch", FromPort: "pull_request", ToNode: "publish", ToPort: "pull_request"},
		},
	}
}

// PreviousEmbeddedReviewerSubpipelinesDefinition recognizes the official seed
// where every reviewer was persisted as a separate subpipeline.
func PreviousEmbeddedReviewerSubpipelinesDefinition() Definition {
	definition := Definition{
		Key:         OfficialReviewWorkflowKey,
		Name:        "Official Gitea PR Review",
		Description: "Pipeline semântica com subpipelines editáveis, reviewers especializados, contratos versionados, validação independente, cobertura e publicação nativa no Gitea.",
		Nodes: []Node{
			{Key: "trigger", Type: "trigger", Name: "Webhook Gitea", Position: Position{X: 40, Y: 650}, Config: map[string]any{"mode": "webhook"}},
			{Key: "fetch", Type: "fetch", Name: "Buscar dados do PR", Position: Position{X: 315, Y: 650}, Config: map[string]any{"integration": ""}},
			{Key: "filter", Type: "filter", Name: "Filtrar arquivos", Position: Position{X: 610, Y: 650}, Config: map[string]any{"include_extensions": []string{".go", ".ts", ".tsx", ".php"}, "ignore_generated": true}},
			{Key: "semantic-units", Type: "semantic_units", Name: "Criar unidades semânticas", Position: Position{X: 900, Y: 650}, Config: map[string]any{"max_units": 200, "max_characters": 50000, "context_lines": 4}},
			{Key: "loop", Type: "loop", Name: "Revisar cada unidade", Position: Position{X: 1190, Y: 650}, Config: map[string]any{"max_iterations": 200, "concurrency": 1, "on_error": "fail"}},
		},
		Edges: []Edge{
			{Key: "trigger-fetch", FromNode: "trigger", FromPort: "event", ToNode: "fetch", ToPort: "event"},
			{Key: "fetch-filter", FromNode: "fetch", FromPort: "files", ToNode: "filter", ToPort: "files"},
			{Key: "filter-semantic-units", FromNode: "filter", FromPort: "files", ToNode: "semantic-units", ToPort: "files"},
			{Key: "semantic-units-loop", FromNode: "semantic-units", FromPort: "units", ToNode: "loop", ToPort: "items"},
		},
	}
	for index, reviewer := range officialReviewerSpecs() {
		groupKey := "reviewer-" + reviewer.Key
		groupY := float64(20 + index*220)
		templateKey := "template-" + reviewer.Key
		modelKey := "model-" + reviewer.Key
		validateKey := "validate-" + reviewer.Key
		validatorKey := "candidate-validator-" + reviewer.Key
		filterKey := "response-filter-" + reviewer.Key
		definition.Nodes = append(definition.Nodes,
			Node{
				Key: groupKey, Type: "subpipeline", Name: "Reviewer · " + reviewer.Name,
				Position: Position{X: 1450, Y: groupY},
				Size:     &NodeSize{Width: 900, Height: 190},
				Config: map[string]any{
					"input_ports":  []Port{{Key: "context", Label: "Unidade", Contract: "any", Required: true}},
					"output_ports": []Port{{Key: "comments", Label: "Comentários", Contract: "list", Required: true}},
				},
			},
			Node{
				Key: templateKey, Type: "template", Name: "Tarefa: " + reviewer.Name,
				ParentKey: groupKey, Position: Position{X: 28, Y: 54},
				Config: map[string]any{
					"template":                reviewer.Prompt,
					"review_contract_key":     "review." + reviewer.Key,
					"review_contract_version": 2,
				},
			},
			Node{
				Key: modelKey, Type: "model", Name: "Reviewer: " + reviewer.Name,
				ParentKey: groupKey, Position: Position{X: 198, Y: 54},
				Config: map[string]any{"model_profile": "", "max_tokens": 2000, "retry_limit": 0, "retry_delay_ms": 0},
			},
			Node{
				Key: validateKey, Type: "validate", Name: "Validar contrato: " + reviewer.Name,
				ParentKey: groupKey, Position: Position{X: 368, Y: 54},
				Config: map[string]any{"validate_paths": true},
			},
			Node{
				Key: validatorKey, Type: "candidate_validator", Name: "Confirmar: " + reviewer.Name,
				ParentKey: groupKey, Position: Position{X: 538, Y: 54},
				Config: map[string]any{"model_profile": "", "max_tokens": 300, "temperature": 0.0, "timeout_seconds": 120},
			},
			Node{
				Key: filterKey, Type: "response_filter", Name: "Filtrar: " + reviewer.Name,
				ParentKey: groupKey, Position: Position{X: 708, Y: 54},
				Config: map[string]any{"minimum_severity": "medium"},
			},
		)
		definition.Edges = append(definition.Edges,
			Edge{Key: "loop-" + groupKey, FromNode: "loop", FromPort: "item", ToNode: groupKey, ToPort: "entry:context"},
			Edge{Key: groupKey + "-" + templateKey, FromNode: groupKey, FromPort: "entry:context", ToNode: templateKey, ToPort: "context"},
			Edge{Key: groupKey + "-" + validateKey, FromNode: groupKey, FromPort: "entry:context", ToNode: validateKey, ToPort: "files"},
			Edge{Key: groupKey + "-" + validatorKey, FromNode: groupKey, FromPort: "entry:context", ToNode: validatorKey, ToPort: "files"},
			Edge{Key: templateKey + "-" + modelKey, FromNode: templateKey, FromPort: "prompt", ToNode: modelKey, ToPort: "prompt"},
			Edge{Key: modelKey + "-" + validateKey, FromNode: modelKey, FromPort: "response", ToNode: validateKey, ToPort: "response"},
			Edge{Key: validateKey + "-" + validatorKey, FromNode: validateKey, FromPort: "valid", ToNode: validatorKey, ToPort: "candidates"},
			Edge{Key: validatorKey + "-" + filterKey, FromNode: validatorKey, FromPort: "confirmed", ToNode: filterKey, ToPort: "response"},
			Edge{Key: filterKey + "-" + groupKey, FromNode: filterKey, FromPort: "comments", ToNode: groupKey, ToPort: "exit:comments"},
			Edge{Key: groupKey + "-merge-reviewers", FromNode: groupKey, FromPort: "exit:comments", ToNode: "merge-reviewers", ToPort: "inputs"},
		)
	}
	definition.Nodes = append(definition.Nodes,
		Node{Key: "merge-reviewers", Type: "merge", Name: "Unir reviewers", Position: Position{X: 2430, Y: 650}, Config: map[string]any{"mode": "all"}},
		Node{Key: "consolidate", Type: "consolidate", Name: "Consolidar reviewers", Position: Position{X: 2710, Y: 650}},
		Node{Key: "format", Type: "format", Name: "Formatar review", Position: Position{X: 2990, Y: 650}},
		Node{Key: "publish", Type: "publish", Name: "Publicar no Gitea", Position: Position{X: 3270, Y: 650}, Config: map[string]any{"integration": "", "medium_severity_event": "COMMENT", "allow_autonomous_rejection": false}},
	)
	definition.Edges = append(definition.Edges,
		Edge{Key: "loop-consolidate", FromNode: "loop", FromPort: "results", ToNode: "consolidate", ToPort: "comments"},
		Edge{Key: "consolidate-format", FromNode: "consolidate", FromPort: "review", ToNode: "format", ToPort: "review"},
		Edge{Key: "format-publish", FromNode: "format", FromPort: "formatted", ToNode: "publish", ToPort: "formatted_review"},
		Edge{Key: "fetch-publish-target", FromNode: "fetch", FromPort: "pull_request", ToNode: "publish", ToPort: "pull_request"},
	)
	return definition
}

type officialReviewerSpec struct {
	Key    string
	Name   string
	Prompt string
}

func officialReviewerSpecs() []officialReviewerSpec {
	return []officialReviewerSpec{
		{Key: "security", Name: "Segurança", Prompt: "Atue somente como reviewer de segurança desta unidade semântica. Procure bypass concreto de autenticação, autorização ou isolamento de dados introduzido pela alteração. Produza apenas CandidateFinding sustentados por evidência observável; quando faltar contexto, declare required_context. Não confirme nem publique achados. Responda somente uma lista JSON."},
		{Key: "correctness", Name: "Corretude", Prompt: "Atue somente como reviewer de corretude desta unidade semântica. Procure comportamento incorreto reproduzível, limites quebrados e tratamento de erro defeituoso introduzidos pela alteração. Produza apenas CandidateFinding sustentados por evidência observável; quando faltar contexto, declare required_context. Não confirme nem publique achados. Responda somente uma lista JSON."},
		{Key: "contracts", Name: "Contratos", Prompt: "Atue somente como reviewer de contratos desta unidade semântica. Procure quebra concreta de API, formato persistido, schema ou integração introduzida pela alteração. Não trate contrato ausente no diff como inexistente; quando não for observável, declare required_context. Não confirme nem publique achados. Responda somente uma lista JSON."},
		{Key: "performance", Name: "Performance", Prompt: "Atue somente como reviewer de performance desta unidade semântica. Procure regressão material em caminho frequente, trabalho repetido evitável ou crescimento não limitado introduzido pela alteração. Produza apenas CandidateFinding sustentados por evidência observável; quando faltar contexto, declare required_context. Não confirme nem publique achados. Responda somente uma lista JSON."},
		{Key: "architecture", Name: "Arquitetura", Prompt: "Atue somente como reviewer de arquitetura desta unidade semântica. Procure violação funcional de fronteira, dependência indevida ou efeito externo introduzido no lugar errado. Produza apenas CandidateFinding sustentados por evidência observável; quando faltar contexto, declare required_context. Não confirme nem publique achados. Responda somente uma lista JSON."},
		{Key: "observability", Name: "Observabilidade", Prompt: "Atue somente como reviewer de observabilidade desta unidade semântica. Procure falha operacional nova que fique silenciosa ou perca logs, métricas ou contexto indispensável ao diagnóstico. Produza apenas CandidateFinding sustentados por evidência observável; quando faltar contexto, declare required_context. Não confirme nem publique achados. Responda somente uma lista JSON."},
	}
}

// PreviousCompactSpecializedReviewDefinition recognizes the last official
// seed before reviewer lanes became first-class subpipeline containers.
func PreviousCompactSpecializedReviewDefinition() Definition {
	definition := Definition{
		Key:         OfficialReviewWorkflowKey,
		Name:        "Official Gitea PR Review",
		Description: "Pipeline semântica com reviewers especializados, contratos versionados, validação independente, cobertura e publicação nativa no Gitea.",
		Nodes: []Node{
			{Key: "trigger", Type: "trigger", Name: "Webhook Gitea", Position: Position{X: 40, Y: 650}, Config: map[string]any{"mode": "webhook"}},
			{Key: "fetch", Type: "fetch", Name: "Buscar dados do PR", Position: Position{X: 315, Y: 650}, Config: map[string]any{"integration": ""}},
			{Key: "filter", Type: "filter", Name: "Filtrar arquivos", Position: Position{X: 610, Y: 650}, Config: map[string]any{"include_extensions": []string{".go", ".ts", ".tsx", ".php"}, "ignore_generated": true}},
			{Key: "semantic-units", Type: "semantic_units", Name: "Criar unidades semânticas", Position: Position{X: 900, Y: 650}, Config: map[string]any{"max_units": 200, "max_characters": 50000, "context_lines": 4}},
			{Key: "loop", Type: "loop", Name: "Revisar cada unidade", Position: Position{X: 1190, Y: 650}, Config: map[string]any{"max_iterations": 200, "concurrency": 1, "on_error": "fail"}},
		},
		Edges: []Edge{
			{Key: "trigger-fetch", FromNode: "trigger", FromPort: "event", ToNode: "fetch", ToPort: "event"},
			{Key: "fetch-filter", FromNode: "fetch", FromPort: "files", ToNode: "filter", ToPort: "files"},
			{Key: "filter-semantic-units", FromNode: "filter", FromPort: "files", ToNode: "semantic-units", ToPort: "files"},
			{Key: "semantic-units-loop", FromNode: "semantic-units", FromPort: "units", ToNode: "loop", ToPort: "items"},
		},
	}
	for index, reviewer := range officialReviewerSpecs() {
		y := float64(40 + index*220)
		templateKey := "template-" + reviewer.Key
		modelKey := "model-" + reviewer.Key
		validateKey := "validate-" + reviewer.Key
		validatorKey := "candidate-validator-" + reviewer.Key
		filterKey := "response-filter-" + reviewer.Key
		definition.Nodes = append(definition.Nodes,
			Node{Key: templateKey, Type: "template", Name: "Tarefa: " + reviewer.Name, Position: Position{X: 1480, Y: y}, Config: map[string]any{"template": reviewer.Prompt, "review_contract_key": "review." + reviewer.Key, "review_contract_version": 2}},
			Node{Key: modelKey, Type: "model", Name: "Reviewer: " + reviewer.Name, Position: Position{X: 1660, Y: y}, Config: map[string]any{"model_profile": "", "max_tokens": 2000, "retry_limit": 0, "retry_delay_ms": 0}},
			Node{Key: validateKey, Type: "validate", Name: "Validar contrato: " + reviewer.Name, Position: Position{X: 1840, Y: y}, Config: map[string]any{"validate_paths": true}},
			Node{Key: validatorKey, Type: "candidate_validator", Name: "Confirmar: " + reviewer.Name, Position: Position{X: 2020, Y: y}, Config: map[string]any{"model_profile": "", "max_tokens": 300, "temperature": 0.0, "timeout_seconds": 120}},
			Node{Key: filterKey, Type: "response_filter", Name: "Filtrar: " + reviewer.Name, Position: Position{X: 2200, Y: y}, Config: map[string]any{"minimum_severity": "medium"}},
		)
		definition.Edges = append(definition.Edges,
			Edge{Key: "loop-" + templateKey, FromNode: "loop", FromPort: "item", ToNode: templateKey, ToPort: "context"},
			Edge{Key: templateKey + "-" + modelKey, FromNode: templateKey, FromPort: "prompt", ToNode: modelKey, ToPort: "prompt"},
			Edge{Key: modelKey + "-" + validateKey, FromNode: modelKey, FromPort: "response", ToNode: validateKey, ToPort: "response"},
			Edge{Key: "loop-" + validateKey, FromNode: "loop", FromPort: "item", ToNode: validateKey, ToPort: "files"},
			Edge{Key: validateKey + "-" + validatorKey, FromNode: validateKey, FromPort: "valid", ToNode: validatorKey, ToPort: "candidates"},
			Edge{Key: "loop-" + validatorKey, FromNode: "loop", FromPort: "item", ToNode: validatorKey, ToPort: "files"},
			Edge{Key: validatorKey + "-" + filterKey, FromNode: validatorKey, FromPort: "confirmed", ToNode: filterKey, ToPort: "response"},
		)
	}
	definition.Nodes = append(definition.Nodes,
		Node{Key: "consolidate", Type: "consolidate", Name: "Consolidar reviewers", Position: Position{X: 2440, Y: 650}},
		Node{Key: "format", Type: "format", Name: "Formatar review", Position: Position{X: 2735, Y: 650}},
		Node{Key: "publish", Type: "publish", Name: "Publicar no Gitea", Position: Position{X: 3030, Y: 650}, Config: map[string]any{"integration": "", "medium_severity_event": "COMMENT", "allow_autonomous_rejection": false}},
	)
	definition.Edges = append(definition.Edges,
		Edge{Key: "loop-consolidate", FromNode: "loop", FromPort: "results", ToNode: "consolidate", ToPort: "comments"},
		Edge{Key: "consolidate-format", FromNode: "consolidate", FromPort: "review", ToNode: "format", ToPort: "review"},
		Edge{Key: "format-publish", FromNode: "format", FromPort: "formatted", ToNode: "publish", ToPort: "formatted_review"},
		Edge{Key: "fetch-publish-target", FromNode: "fetch", FromPort: "pull_request", ToNode: "publish", ToPort: "pull_request"},
	)
	return definition
}

// PreviousSpecializedReviewDefinition recognizes the first specialized seed,
// before its reviewer lanes received the compact visual layout.
func PreviousSpecializedReviewDefinition() Definition {
	definition := PreviousCompactSpecializedReviewDefinition()
	for index := range definition.Nodes {
		switch definition.Nodes[index].Type {
		case "model":
			definition.Nodes[index].Position.X = 1775
		case "validate":
			definition.Nodes[index].Position.X = 2070
		case "candidate_validator":
			definition.Nodes[index].Position.X = 2365
		case "response_filter":
			definition.Nodes[index].Position.X = 2660
		}
		switch definition.Nodes[index].Key {
		case "consolidate":
			definition.Nodes[index].Position.X = 2955
		case "format":
			definition.Nodes[index].Position.X = 3250
		case "publish":
			definition.Nodes[index].Position.X = 3545
		}
	}
	return definition
}

// PreviousSemanticReviewDefinition recognizes the single-reviewer semantic
// seed so startup can append the specialized graph without overwriting user
// customizations.
func PreviousSemanticReviewDefinition() Definition {
	definition := PreviousContractReviewDefinition()
	definition.Description = "Seeded semantic pull-request review pipeline with versioned contracts, coverage, and one native Gitea review publication."
	for index := range definition.Nodes {
		switch definition.Nodes[index].Key {
		case "group":
			definition.Nodes[index].Type = "semantic_units"
			definition.Nodes[index].Name = "Criar unidades semânticas"
			definition.Nodes[index].Config = map[string]any{"max_units": 200, "max_characters": 50000, "context_lines": 4}
		case "loop":
			definition.Nodes[index].Name = "Revisar cada unidade"
			definition.Nodes[index].Config["max_iterations"] = 200
		case "template":
			definition.Nodes[index].Config["template"] = "Analise somente esta unidade semântica e proponha CandidateFinding sustentados por evidência concreta em suas linhas alteradas e contexto disponível. Declare required_context quando a unidade não permitir confirmar a alegação. Não confirme nem publique achados. Responda somente uma lista JSON de candidatos."
			definition.Nodes[index].Config["review_contract_version"] = 2
		}
	}
	for index := range definition.Edges {
		if definition.Edges[index].Key == "group-loop" {
			definition.Edges[index].FromPort = "units"
		}
	}
	return definition
}

// PreviousContractReviewDefinition recognizes the contract-backed seed that
// still grouped files only by size and extension.
func PreviousContractReviewDefinition() Definition {
	definition := officialReviewDefinitionV1()
	definition.Description = "Seeded dynamic pull-request review pipeline whose Template selects an immutable backend contract."
	for index := range definition.Nodes {
		switch definition.Nodes[index].Key {
		case "template":
			definition.Nodes[index].Config["template"] = "Analise este grupo de arquivos e proponha somente CandidateFinding sustentados por evidência concreta em linhas alteradas. Não confirme nem publique achados. Responda somente uma lista JSON de candidatos."
			definition.Nodes[index].Config["review_contract_key"] = OfficialPullRequestContractKey
			definition.Nodes[index].Config["review_contract_version"] = 1
		case "response-filter":
			definition.Nodes[index].Position.X = 2660
		}
	}
	definition.Nodes = append(definition.Nodes, Node{
		Key: "candidate-validator", Type: "candidate_validator", Name: "Validar candidatos",
		Position: Position{X: 2365, Y: 125},
		Config:   map[string]any{"model_profile": "", "max_tokens": 300, "temperature": 0.0, "timeout_seconds": 120},
	})
	edges := make([]Edge, 0, len(definition.Edges)+2)
	for _, edge := range definition.Edges {
		if edge.Key == "validate-response-filter" {
			continue
		}
		edges = append(edges, edge)
	}
	edges = append(edges,
		Edge{Key: "validate-candidate-validator", FromNode: "validate", FromPort: "valid", ToNode: "candidate-validator", ToPort: "candidates"},
		Edge{Key: "loop-candidate-validator", FromNode: "loop", FromPort: "item", ToNode: "candidate-validator", ToPort: "files"},
		Edge{Key: "candidate-validator-response-filter", FromNode: "candidate-validator", FromPort: "confirmed", ToNode: "response-filter", ToPort: "response"},
	)
	definition.Edges = edges
	return definition
}

// officialReviewDefinitionV1 is retained only to recognize and append-only
// upgrade the untouched official seed that published model findings directly.
func officialReviewDefinitionV1() Definition {
	return Definition{
		Key:         OfficialReviewWorkflowKey,
		Name:        "Official Gitea PR Review",
		Description: "Seeded full pull-request review pipeline with one native Gitea review publication.",
		Nodes: []Node{
			{Key: "trigger", Type: "trigger", Name: "Webhook Gitea", Position: Position{X: 40, Y: 280}, Config: map[string]any{"mode": "webhook"}},
			{Key: "fetch", Type: "fetch", Name: "Buscar dados do PR", Position: Position{X: 315, Y: 280}, Config: map[string]any{"integration": ""}},
			{Key: "filter", Type: "filter", Name: "Filtrar arquivos", Position: Position{X: 610, Y: 125}, Config: map[string]any{"include_extensions": []string{".go", ".ts", ".tsx", ".php"}, "ignore_generated": true}},
			{Key: "group", Type: "group", Name: "Agrupar arquivos", Position: Position{X: 900, Y: 125}, Config: map[string]any{"max_files": 8, "max_characters": 12000, "group_by_extension": true}},
			{Key: "loop", Type: "loop", Name: "Revisar cada grupo", Position: Position{X: 1190, Y: 125}, Config: map[string]any{"max_iterations": 20, "concurrency": 1, "on_error": "fail"}},
			{Key: "template", Type: "template", Name: "Prompt de review", Position: Position{X: 1480, Y: 125}, Config: map[string]any{"template": "Analise este grupo de arquivos e responda somente uma lista JSON de achados."}},
			{Key: "model", Type: "model", Name: "Modelo de review", Position: Position{X: 1775, Y: 125}, Config: map[string]any{"model_profile": "", "max_tokens": 2000, "retry_limit": 0, "retry_delay_ms": 0}},
			{Key: "validate", Type: "validate", Name: "Validar resposta", Position: Position{X: 2070, Y: 125}, Config: map[string]any{"validate_paths": true}},
			{Key: "response-filter", Type: "response_filter", Name: "Filtrar achados", Position: Position{X: 2365, Y: 125}, Config: map[string]any{"minimum_severity": "medium"}},
			{Key: "consolidate", Type: "consolidate", Name: "Consolidar review", Position: Position{X: 2070, Y: 480}},
			{Key: "format", Type: "format", Name: "Formatar review", Position: Position{X: 2365, Y: 480}},
			{Key: "publish", Type: "publish", Name: "Publicar no Gitea", Position: Position{X: 2660, Y: 480}, Config: map[string]any{"integration": "", "medium_severity_event": "COMMENT", "allow_autonomous_rejection": false}},
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
			{Key: "fetch-publish-target", FromNode: "fetch", FromPort: "pull_request", ToNode: "publish", ToPort: "pull_request"},
		},
	}
}

// PreviousOfficialReviewDefinition identifies only the untouched seed shipped
// before typed trigger modes. It is used for an append-only startup upgrade.
func PreviousOfficialReviewDefinition() Definition {
	definition := officialReviewDefinitionV1()
	definition.Nodes[0].Name = "Webhook / manual"
	definition.Nodes[0].Config = nil
	return definition
}

func PreviousVerifiableReviewDefinition() Definition {
	return officialReviewDefinitionV1()
}

func PreviousCandidateReviewDefinition() Definition {
	definition := PreviousChecklistReviewDefinition()
	for index := range definition.Nodes {
		if definition.Nodes[index].Key == "model" || definition.Nodes[index].Key == "candidate-validator" {
			delete(definition.Nodes[index].Config, "review_checklist")
		}
	}
	return definition
}

// PreviousChecklistReviewDefinition recognizes the last seed where schema and
// checklist snapshots were duplicated across downstream cards.
func PreviousChecklistReviewDefinition() Definition {
	definition := PreviousContractReviewDefinition()
	checklist := officialReviewChecklistSnapshot()
	for index := range definition.Nodes {
		switch definition.Nodes[index].Key {
		case "template":
			delete(definition.Nodes[index].Config, "review_contract_key")
			delete(definition.Nodes[index].Config, "review_contract_version")
		case "model", "candidate-validator":
			definition.Nodes[index].Config["review_checklist"] = checklist
		case "validate":
			definition.Nodes[index].Config["response_contract_key"] = "review.candidate-findings.v1"
			definition.Nodes[index].Config["response_schema"] = responseContractSchema("review.candidate-findings.v1")
		}
	}
	definition.Description = "Seeded verifiable pull-request review pipeline with independent candidate validation and one native Gitea review publication."
	return definition
}
