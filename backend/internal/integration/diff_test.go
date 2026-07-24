package integration

import (
	"strings"
	"testing"
)

func TestAttachUnifiedDiffPatchesMatchesNestedAndQuotedPaths(t *testing.T) {
	files := []map[string]any{
		{"filename": "b/service.go"},
		{"filename": "docs/read me.md"},
	}
	diff := "diff --git a/b/service.go b/b/service.go\n--- a/b/service.go\n+++ b/b/service.go\n@@ -1 +1,2 @@\n package b\n+var enabled = true\n" +
		"diff --git \"a/docs/read me.md\" \"b/docs/read me.md\"\n--- \"a/docs/read me.md\"\n+++ \"b/docs/read me.md\"\n@@ -1 +1 @@\n-old\n+new"

	if count := attachUnifiedDiffPatches(files, diff); count != 2 {
		t.Fatalf("reviewable files = %d, want 2", count)
	}
	if patch, _ := files[0]["patch"].(string); !strings.Contains(patch, "+var enabled = true") {
		t.Fatalf("nested path patch = %q", patch)
	}
	if patch, _ := files[1]["patch"].(string); !strings.Contains(patch, `+++ "b/docs/read me.md"`) || !strings.Contains(patch, "+new") {
		t.Fatalf("quoted path patch = %q", patch)
	}
}

func TestAttachUnifiedDiffPatchesPreservesProviderPatch(t *testing.T) {
	files := []map[string]any{{"filename": "main.go", "patch": "provider patch"}}
	if count := attachUnifiedDiffPatches(files, ""); count != 1 {
		t.Fatalf("reviewable files = %d, want 1", count)
	}
	if files[0]["patch"] != "provider patch" {
		t.Fatalf("provider patch was overwritten: %#v", files[0])
	}
}
