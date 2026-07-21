package review

import "testing"

func TestRuleValidationAndEvaluation(t *testing.T) {
	schema := `{
		"type":"object",
		"required":["score","status","name","tags","findings"],
		"properties":{
			"score":{"type":"number"},
			"status":{"type":"string"},
			"name":{"type":"string"},
			"tags":{"type":"array","items":{"type":"string"}},
			"meta":{"type":"object","properties":{"enabled":{"type":"boolean"}}},
			"findings":{"type":"array","items":{"type":"object","required":["id","severity","confidence"],"properties":{"id":{"type":"string"},"severity":{"type":"string"},"confidence":{"type":"number"}}}}
		}
	}`
	payload := map[string]any{
		"score": 9, "status": "ready", "name": "release-17", "tags": []string{"stable", "backend"},
		"meta": map[string]any{"enabled": true},
		"findings": []map[string]any{
			{"id": "one", "severity": "high", "confidence": .95},
			{"id": "two", "severity": "low", "confidence": .4},
		},
	}
	rule := Rule{Operator: RuleAll, Rules: []Rule{
		{Operator: RuleEquals, Path: "status", Value: "ready"},
		{Operator: RuleContains, Path: "tags", Value: "stable"},
		{Operator: RuleIn, Path: "status", Value: []any{"ready", "done"}},
		{Operator: RuleGT, Path: "score", Value: 8},
		{Operator: RuleGTE, Path: "score", Value: 9},
		{Operator: RuleLT, Path: "score", Value: 10},
		{Operator: RuleLTE, Path: "score", Value: 9},
		{Operator: RuleMatches, Path: "name", Value: `^release-[0-9]+$`},
		{Operator: RuleExists, Path: "meta.enabled", Value: true},
		{Operator: RuleAny, Rules: []Rule{{Operator: RuleEquals, Path: "status", Value: "missing"}, {Operator: RuleEquals, Path: "status", Value: "ready"}}},
		{Operator: RuleNot, Rules: []Rule{{Operator: RuleEquals, Path: "status", Value: "blocked"}}},
		{Operator: RuleEquals, Scope: &CollectionScope{Kind: CollectionAny, Path: "findings", Rule: &Rule{Operator: RuleGT, Path: "confidence", Value: .9}}, Value: true},
		{Operator: RuleEquals, Scope: &CollectionScope{Kind: CollectionAll, Path: "findings", Rule: &Rule{Operator: RuleExists, Path: "id", Value: true}}, Value: true},
		{Operator: RuleGTE, Scope: &CollectionScope{Kind: CollectionCount, Path: "findings", Rule: &Rule{Operator: RuleEquals, Path: "severity", Value: "high"}}, Value: 1},
		{Operator: RuleExists, Scope: &CollectionScope{Kind: CollectionFilter, Path: "findings", Rule: &Rule{Operator: RuleEquals, Path: "severity", Value: "low"}}, Value: true},
	}}

	matched, err := EvaluateRule(rule, schema, payload)
	if err != nil || !matched {
		t.Fatalf("expected rule to match: matched=%v err=%v", matched, err)
	}
}

func TestRuleRejectsUnknownSchemaPathInvalidRegexAndPayload(t *testing.T) {
	schema := `{"type":"object","required":["status"],"properties":{"status":{"type":"string"}}}`
	for _, rule := range []Rule{
		{Operator: RuleEquals, Path: "missing", Value: true},
		{Operator: RuleMatches, Path: "status", Value: "["},
		{Operator: "execute", Path: "status", Value: "ready"},
	} {
		if err := ValidateRule(rule, schema); err == nil {
			t.Fatalf("expected invalid rule to be rejected: %#v", rule)
		}
	}
	_, err := EvaluateRule(Rule{Operator: RuleEquals, Path: "status", Value: "ready"}, schema, map[string]any{})
	if err == nil {
		t.Fatal("expected payload missing a required contract field to be rejected")
	}
}
