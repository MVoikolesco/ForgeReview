package workflow

import (
	"strings"
	"testing"
)

func TestSemanticUnitsSplitHunksAndPreserveObservableContext(t *testing.T) {
	files := []map[string]any{
		{
			"filename":                 "src/auth.go",
			"_forgereview_repository":  "acme/api",
			"_forgereview_base_commit": "abc123",
			"patch":                    "@@ -1,2 +1,4 @@ func Authorize\n package auth\n+import \"net/http\"\n+func Authorize() bool { return false }\n context\n@@ -20,1 +22,2 @@\n old\n+func helper() {}\n",
		},
		{"filename": "src/auth_test.go", "patch": "@@ -1 +1,2 @@\n package auth\n+func TestAuthorize() {}\n"},
		{"filename": "src/auth.schema.json", "patch": "@@ -1 +1 @@\n+{}\n"},
	}
	units, err := buildSemanticUnits([]any{files}, Node{
		Key: "units", Type: "semantic_units",
		Config: map[string]any{"max_units": 20, "max_characters": 50000, "context_lines": 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	authUnits := make([]SemanticUnit, 0)
	for _, unit := range units {
		if unit.Path == "src/auth.go" {
			authUnits = append(authUnits, unit)
		}
	}
	if len(authUnits) != 2 {
		t.Fatalf("auth units = %#v", authUnits)
	}
	first := authUnits[0]
	if first.Symbol != "Authorize" || first.Kind != "symbol" || first.Language != "go" {
		t.Fatalf("first semantic identity = %#v", first)
	}
	if len(first.AddedLines) != 2 || first.AddedLines[0] != 2 || first.AddedLines[1] != 3 {
		t.Fatalf("added lines = %#v", first.AddedLines)
	}
	if len(first.Imports) != 1 || first.Imports[0] != "net/http" {
		t.Fatalf("imports = %#v", first.Imports)
	}
	if len(first.RelatedTests) != 1 || first.RelatedTests[0] != "src/auth_test.go" ||
		len(first.RelatedContracts) != 1 || first.RelatedContracts[0] != "src/auth.schema.json" {
		t.Fatalf("relations = tests %#v contracts %#v", first.RelatedTests, first.RelatedContracts)
	}
	if first.UnitID == "" || first.UnitID != semanticUnitID("acme/api", "abc123", "src/auth.go", "Authorize", 1) {
		t.Fatalf("unit id = %q", first.UnitID)
	}
	if first.file == nil || !strings.HasPrefix(first.file["patch"].(string), "@@ -1,2 +1,4 @@") ||
		strings.Contains(first.file["patch"].(string), "@@ -20") {
		t.Fatalf("scoped observed file = %#v", first.file)
	}
	contexts := map[string]bool{}
	for _, item := range first.AvailableContext {
		contexts[item] = true
	}
	for _, expected := range []string{ContextDiff, ContextFile, ContextSymbol, ContextDependencies, ContextTests, ContextContracts, ContextRepository} {
		if !contexts[expected] {
			t.Fatalf("available contexts = %#v", first.AvailableContext)
		}
	}
}

func TestSemanticUnitsRejectUnboundedOutput(t *testing.T) {
	files := []map[string]any{{"filename": "a.go", "patch": "@@ -1 +1 @@\n+a\n@@ -3 +3 @@\n+b"}}
	_, err := buildSemanticUnits([]any{files}, Node{Key: "units", Config: map[string]any{"max_units": 1}})
	if err == nil || !strings.Contains(err.Error(), "max_units") {
		t.Fatalf("bounded unit error = %v", err)
	}
}

func TestSemanticUnitFeedsObservedFileValidation(t *testing.T) {
	unit := SemanticUnit{file: map[string]any{"filename": "a.go", "patch": "@@ -4 +4 @@\n+added"}}
	files, err := workflowFiles([]any{unit})
	if err != nil || len(files) != 1 || !lineIsAddedInPatch(files[0], 4) {
		t.Fatalf("semantic observed files = %#v, %v", files, err)
	}
}
