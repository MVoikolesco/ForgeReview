package diff

import "testing"

func TestParse(t *testing.T) {
	raw := `diff --git a/README.md b/README.md
index 111..222 100644
--- a/README.md
+++ b/README.md
@@ -1,2 +1,3 @@
 line one
-old line
+new line
+another line
diff --git a/internal/app.go b/internal/app.go
index 333..444 100644
--- a/internal/app.go
+++ b/internal/app.go
@@ -10,1 +10,1 @@
-fmt.Println("old")
+fmt.Println("new")
`

	files := Parse(raw)

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	if files[0].Path != "README.md" {
		t.Fatalf("expected README.md, got %q", files[0].Path)
	}

	if files[0].Additions != 2 {
		t.Fatalf("expected 2 additions, got %d", files[0].Additions)
	}

	if files[0].Deletions != 1 {
		t.Fatalf("expected 1 deletion, got %d", files[0].Deletions)
	}

	if files[1].Path != "internal/app.go" {
		t.Fatalf("expected internal/app.go, got %q", files[1].Path)
	}

	if files[1].Additions != 1 {
		t.Fatalf("expected 1 addition, got %d", files[1].Additions)
	}

	if files[1].Deletions != 1 {
		t.Fatalf("expected 1 deletion, got %d", files[1].Deletions)
	}
}

func TestParseEmptyDiff(t *testing.T) {
	files := Parse("")

	if len(files) != 0 {
		t.Fatalf("expected no files, got %d", len(files))
	}
}
