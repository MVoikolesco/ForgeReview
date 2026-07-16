package redis

import "testing"

func TestDecodeJobIncludesPullRequestMetadata(t *testing.T) {
	job, err := decodeJob(map[string]any{
		"owner":              "acme",
		"repo":               "portal",
		"pr_number":          "42",
		"requested_reviewer": "ia-reviewer",
		"sender":             "marcio",
		"manual":             "false",
		"title":              "Corrige login",
		"description":        "Descricao",
		"author":             "ana",
		"base_branch":        "main",
		"head_branch":        "feature/login",
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.Title != "Corrige login" || job.Description != "Descricao" || job.Author != "ana" || job.BaseBranch != "main" || job.HeadBranch != "feature/login" {
		t.Fatalf("unexpected metadata %#v", job)
	}
}
