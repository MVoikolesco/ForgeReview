package review

import (
	"encoding/json"
	"testing"
)

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

func TestRuleProjectsOnlyFiltersFromSuccessfulLogicalBranches(t *testing.T) {
	schema := `{"type":"object","required":["files","mode"],"properties":{"mode":{"type":"string"},"files":{"type":"array","items":{"type":"object","required":["path"],"properties":{"path":{"type":"string"}}}}}}`
	payload := map[string]any{"mode": "active", "files": []map[string]any{{"path": "web.tsx"}, {"path": "api.go"}}}
	tsxFilter := Rule{Operator: RuleExists, Scope: &CollectionScope{Kind: CollectionFilter, Path: "files", Rule: &Rule{Operator: RuleMatches, Path: "path", Value: `\.tsx$`}}, Value: true}
	goFilter := Rule{Operator: RuleExists, Scope: &CollectionScope{Kind: CollectionFilter, Path: "files", Rule: &Rule{Operator: RuleMatches, Path: "path", Value: `\.go$`}}, Value: true}

	tests := []struct {
		name string
		rule Rule
	}{
		{name: "nonmatching any branch", rule: Rule{Operator: RuleAny, Rules: []Rule{{Operator: RuleAll, Rules: []Rule{tsxFilter, {Operator: RuleEquals, Path: "mode", Value: "inactive"}}}, goFilter}}},
		{name: "inverted branch", rule: Rule{Operator: RuleAll, Rules: []Rule{{Operator: RuleNot, Rules: []Rule{{Operator: RuleAll, Rules: []Rule{tsxFilter, {Operator: RuleEquals, Path: "mode", Value: "inactive"}}}}}, goFilter}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matched, projected, err := EvaluateRuleArtifact(test.rule, schema, payload)
			if err != nil || !matched {
				t.Fatalf("rule did not match: matched=%v err=%v", matched, err)
			}
			body, _ := json.Marshal(projected)
			if string(body) != `{"files":[{"path":"api.go"}],"mode":"active"}` {
				t.Fatalf("inactive filter scope changed projection: %s", body)
			}
		})
	}
}
