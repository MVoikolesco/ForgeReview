package workflow

import (
	"strings"
	"testing"
)

func TestResponseContractsAreStableAndCompile(t *testing.T) {
	contracts := ResponseContracts()
	if len(contracts) != 6 || contracts[0].Key != "review.findings.v1" || contracts[1].Key != "review.candidate-findings.v1" ||
		contracts[2].Key != "review.candidate-findings.v2" || contracts[2].Version != 2 || contracts[0].Editable {
		t.Fatalf("unexpected contracts: %#v", contracts)
	}
	for _, contract := range contracts {
		if _, err := validateResponseSchema(contract.Schema); err != nil {
			t.Fatalf("contract %q does not compile: %v", contract.Key, err)
		}
	}
}

func TestValidateResponseUsesConfiguredSchema(t *testing.T) {
	config := map[string]any{"response_schema": map[string]any{
		"type":       "object",
		"required":   []any{"summary"},
		"properties": map[string]any{"summary": map[string]any{"type": "string"}},
	}}
	value, port := validateResponse([]any{`{"summary":"ok"}`}, nil, config)
	if port != "valid" || value.(map[string]any)["summary"] != "ok" {
		t.Fatalf("valid response = %#v, %q", value, port)
	}
	value, port = validateResponse([]any{`{"summary":1}`}, nil, config)
	failure, ok := value.(ValidationFailure)
	if port != "invalid" || !ok || failure.Code != "schema_mismatch" {
		t.Fatalf("invalid response = %#v, %q", value, port)
	}
}

func TestResponseSchemaRejectsExternalReferencesAndExcessiveDepth(t *testing.T) {
	if _, err := validateResponseSchema(map[string]any{"$ref": "https://example.com/schema.json"}); err == nil || !strings.Contains(err.Error(), "external") {
		t.Fatalf("external ref error = %v", err)
	}
	var schema any = map[string]any{"type": "string"}
	for range maxResponseSchemaDepth + 1 {
		schema = map[string]any{"not": schema}
	}
	if _, err := validateResponseSchema(schema); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("depth error = %v", err)
	}
}

func TestWorkflowValidationRejectsInvalidConfiguredResponseSchema(t *testing.T) {
	definition := Definition{
		Key: "schema", Name: "Schema",
		Nodes: []Node{{
			Key: "validate", Name: "Validate", Type: "validate",
			Config: map[string]any{"response_schema": map[string]any{"type": "not-a-type"}},
		}},
	}
	if err := Validate(definition, DefaultCatalog()); err == nil || !strings.Contains(err.Error(), "response_schema") {
		t.Fatalf("validation error = %v", err)
	}
}
