package workflow

import "testing"

func TestValidateAcceptsTypedGraph(t *testing.T) {
	catalog := DefaultCatalog()
	definition := Definition{
		Key: "review", Name: "Review",
		Nodes: []Node{{Key: "start", Type: "trigger", Name: "Evento"}, {Key: "fetch", Type: "fetch", Name: "Dados"}},
		Edges: []Edge{{Key: "event", FromNode: "start", FromPort: "event", ToNode: "fetch", ToPort: "event"}},
	}
	if err := Validate(definition, catalog); err != nil {
		t.Fatalf("expected valid graph: %v", err)
	}
}

func TestValidateRejectsSecretConfigurationAndDuplicateEdgeKeys(t *testing.T) {
	definition := Definition{Key: "safe", Name: "Safe", Nodes: []Node{{Key: "trigger", Type: "trigger", Name: "Trigger", Config: map[string]any{"token": "must-not-persist"}}}}
	if err := Validate(definition, DefaultCatalog()); err == nil {
		t.Fatal("expected secret configuration rejection")
	}
	definition.Nodes[0].Config = map[string]any{}
	definition.Nodes = append(definition.Nodes, Node{Key: "log", Type: "log", Name: "Log", Config: map[string]any{}})
	definition.Edges = []Edge{
		{Key: "same", FromNode: "trigger", FromPort: "event", ToNode: "log", ToPort: "input"},
		{Key: "same", FromNode: "trigger", FromPort: "event", ToNode: "log", ToPort: "input"},
	}
	if err := Validate(definition, DefaultCatalog()); err == nil {
		t.Fatal("expected duplicate edge rejection")
	}
}

func TestValidateRejectsInvalidCardsAndContracts(t *testing.T) {
	catalog := DefaultCatalog()
	unknown := Definition{Key: "review", Name: "Review", Nodes: []Node{{Key: "unknown", Type: "script", Name: "Script"}}}
	if err := Validate(unknown, catalog); err == nil {
		t.Fatal("expected unknown card type to be rejected")
	}
	incompatible := Definition{
		Key: "review", Name: "Review",
		Nodes: []Node{{Key: "start", Type: "trigger", Name: "Evento"}, {Key: "model", Type: "model", Name: "Modelo"}},
		Edges: []Edge{{Key: "invalid", FromNode: "start", FromPort: "event", ToNode: "model", ToPort: "prompt"}},
	}
	if err := Validate(incompatible, catalog); err == nil {
		t.Fatal("expected incompatible ports to be rejected")
	}
}

func TestValidateBoundsLoopParallelism(t *testing.T) {
	definition := Definition{Key: "loop", Name: "Loop", Nodes: []Node{{Key: "loop", Type: "loop", Name: "Loop", Config: map[string]any{"max_iterations": 1, "concurrency": 5}}}}
	if err := Validate(definition, DefaultCatalog()); err == nil || err.Error() != `loop card "loop" config.concurrency must not exceed 4` {
		t.Fatalf("loop validation error = %v", err)
	}
}

func TestValidateBoundsModelCorrectiveRetryConfiguration(t *testing.T) {
	definition := Definition{Key: "retry", Name: "Retry", Nodes: []Node{{Key: "model", Type: "model", Name: "Model", Config: map[string]any{"retry_limit": 4}}}}
	if err := Validate(definition, DefaultCatalog()); err == nil || err.Error() != `model card "model" config.retry_limit must be between 0 and 3` {
		t.Fatalf("retry limit validation error = %v", err)
	}
	definition.Nodes[0].Config = map[string]any{"retry_limit": 0, "retry_delay_ms": 60001}
	if err := Validate(definition, DefaultCatalog()); err == nil || err.Error() != `model card "model" config.retry_delay_ms must be between 0 and 60000` {
		t.Fatalf("retry delay validation error = %v", err)
	}
	definition.Nodes[0].Config = map[string]any{"model_profile": "reviewer", "max_tokens": 128001}
	if err := Validate(definition, DefaultCatalog()); err == nil || err.Error() != `model card "model" config.max_tokens must be between 1 and 128000` {
		t.Fatalf("max tokens validation error = %v", err)
	}
}

func TestValidateRequiresPinnedSubpipelineAndRejectsMisplacedPublishPolicy(t *testing.T) {
	definition := Definition{Key: "unsupported", Name: "Unsupported", Nodes: []Node{{Key: "child", Type: "workflow", Name: "Child"}}}
	if err := Validate(definition, DefaultCatalog()); err == nil || err.Error() != `workflow card "child" requires positive config.workflow_version_id` {
		t.Fatalf("subpipeline pin validation error = %v", err)
	}
	definition.Nodes[0] = Node{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"medium_severity_event": "COMMENT"}}
	if err := Validate(definition, DefaultCatalog()); err == nil || err.Error() != `fetch card "fetch" does not support config.medium_severity_event; configure it on publish` {
		t.Fatalf("misplaced policy validation error = %v", err)
	}
	definition.Nodes[0] = Node{Key: "publish", Type: "publish", Name: "Publish", Config: map[string]any{"medium_severity_event": "APPROVE"}}
	if err := Validate(definition, DefaultCatalog()); err == nil || err.Error() != `publish card "publish" config.medium_severity_event must be "COMMENT" or "REQUEST_CHANGES"` {
		t.Fatalf("publish policy validation error = %v", err)
	}
}

func TestCatalogMarksMergeAsCollectAllAndSubpipelineAvailable(t *testing.T) {
	catalog := DefaultCatalog()
	merge, _ := catalog.Get("merge")
	if len(merge.Inputs) != 1 || !merge.Inputs[0].CollectAll {
		t.Fatalf("merge input = %#v", merge.Inputs)
	}
	workflowCard, _ := catalog.Get("workflow")
	if !workflowCard.Available || workflowCard.UnavailableReason != "" {
		t.Fatalf("workflow availability = %#v", workflowCard)
	}
}

func TestValidateRequiresCacheConfiguration(t *testing.T) {
	definition := Definition{Key: "cache", Name: "Cache", Nodes: []Node{{Key: "cache", Type: "cache", Name: "Cache", Config: map[string]any{"key": "review:42", "mode": "write", "ttl_seconds": 60}}}}
	if err := Validate(definition, DefaultCatalog()); err != nil {
		t.Fatalf("valid cache configuration: %v", err)
	}
	definition.Nodes[0].Config = map[string]any{"key": "review:42", "mode": "invalid"}
	if err := Validate(definition, DefaultCatalog()); err == nil || err.Error() != `cache card "cache" config.mode must be "read", "write", or "delete"` {
		t.Fatalf("cache mode validation error = %v", err)
	}
	definition.Nodes[0].Config = map[string]any{"key": "review:42", "mode": "write", "ttl_seconds": 0}
	if err := Validate(definition, DefaultCatalog()); err == nil || err.Error() != `cache card "cache" config.ttl_seconds must be between 1 and 86400 for write mode` {
		t.Fatalf("cache TTL validation error = %v", err)
	}
}

func TestValidateRequiresExplicitTypedErrorRoute(t *testing.T) {
	definition := Definition{Key: "errors", Name: "Errors", Nodes: []Node{{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"on_error": "route"}}}}
	if err := Validate(definition, DefaultCatalog()); err == nil || err.Error() != `node "template" config.on_error "route" requires an explicit error edge` {
		t.Fatalf("route validation error = %v", err)
	}
	definition.Nodes = append(definition.Nodes, Node{Key: "control", Type: "error_control", Name: "Control"})
	definition.Edges = []Edge{{Key: "error", FromNode: "template", FromPort: "error", ToNode: "control", ToPort: "error"}}
	if err := Validate(definition, DefaultCatalog()); err != nil {
		t.Fatalf("explicit typed route should validate: %v", err)
	}
}

func TestOfficialReviewDefinitionIsAValidFullGraphWithSafePublicationDefaults(t *testing.T) {
	definition := OfficialReviewDefinition()
	if definition.Key != OfficialReviewWorkflowKey || len(definition.Nodes) != 15 || len(definition.Edges) != 18 {
		t.Fatalf("official definition shape = %#v", definition)
	}
	if err := Validate(definition, DefaultCatalog()); err != nil {
		t.Fatalf("official definition validation = %v", err)
	}
	configs := map[string]map[string]any{}
	hasSemanticUnits := false
	nodeTypes := map[string]int{}
	for _, node := range definition.Nodes {
		configs[node.Key] = node.Config
		hasSemanticUnits = hasSemanticUnits || node.Type == "semantic_units"
		nodeTypes[node.Type]++
	}
	if !hasSemanticUnits {
		t.Fatalf("official definition must create semantic units")
	}
	if configs["trigger"]["mode"] != "webhook" {
		t.Fatalf("trigger defaults = %#v", configs["trigger"])
	}
	if nodeTypes["subpipeline"] != 1 || nodeTypes["template"] != 1 || nodeTypes["model"] != 1 || nodeTypes["validate"] != 1 || nodeTypes["candidate_validator"] != 1 || nodeTypes["response_filter"] != 1 {
		t.Fatalf("reusable reviewer recipe node counts = %#v", nodeTypes)
	}
	groupNode := Node{Key: "review-recipe", Config: configs["review-recipe"]}
	instances, err := configuredSubpipelineInstances(groupNode)
	if err != nil || len(instances) != 6 {
		t.Fatalf("review recipe instances = %#v, %v", instances, err)
	}
	for index, category := range []string{"security", "correctness", "contracts", "performance", "architecture", "observability"} {
		instance := instances[index]
		if instance.Key != category || !instance.Enabled || instance.ReviewContractKey != "review."+category || instance.ReviewContractVersion != 2 || instance.Template == "" {
			t.Fatalf("%s instance defaults = %#v", category, instance)
		}
	}
	model := configs["review-model"]
	validator := configs["review-confirm"]
	validate := configs["review-validate"]
	if model["model_profile"] != "" || model["retry_limit"] != 0 || model["retry_delay_ms"] != 0 {
		t.Fatalf("recipe model defaults = %#v", model)
	}
	if validator["model_profile"] != "" || model["review_checklist"] != nil || validator["review_checklist"] != nil || validate["response_schema"] != nil {
		t.Fatalf("contract must not be duplicated downstream: model=%#v validator=%#v validate=%#v", model, validator, validate)
	}
	if configs["publish"]["allow_autonomous_rejection"] != false || configs["publish"]["medium_severity_event"] != "COMMENT" {
		t.Fatalf("publish defaults = %#v", configs["publish"])
	}
}
