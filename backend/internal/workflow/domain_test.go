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
