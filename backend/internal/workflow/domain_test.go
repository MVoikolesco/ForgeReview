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

func TestValidateRequiresASequentialLoopConfiguration(t *testing.T) {
	definition := Definition{Key: "loop", Name: "Loop", Nodes: []Node{{Key: "loop", Type: "loop", Name: "Loop", Config: map[string]any{"max_iterations": 1, "concurrency": 2}}}}
	if err := Validate(definition, DefaultCatalog()); err == nil || err.Error() != `loop card "loop" supports only config.concurrency 1` {
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
	if definition.Key != OfficialReviewWorkflowKey || len(definition.Nodes) != 12 || len(definition.Edges) != 12 {
		t.Fatalf("official definition shape = %#v", definition)
	}
	if err := Validate(definition, DefaultCatalog()); err != nil {
		t.Fatalf("official definition validation = %v", err)
	}
	configs := map[string]map[string]any{}
	for _, node := range definition.Nodes {
		configs[node.Key] = node.Config
	}
	if configs["model"]["model_profile"] != "" || configs["model"]["retry_limit"] != 0 || configs["model"]["retry_delay_ms"] != 0 {
		t.Fatalf("model defaults = %#v", configs["model"])
	}
	if configs["publish"]["allow_autonomous_rejection"] != false || configs["publish"]["medium_severity_event"] != "COMMENT" {
		t.Fatalf("publish defaults = %#v", configs["publish"])
	}
}
