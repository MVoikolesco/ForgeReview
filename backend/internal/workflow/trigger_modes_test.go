package workflow

import (
	"context"
	"encoding/json"
	"testing"

	"forgereview/backend/internal/integration"
)

type targetReader struct {
	request integration.PullRequestRequest
}

func (r *targetReader) ReadPullRequest(_ context.Context, _ integration.Integration, _ string, request integration.PullRequestRequest) (integration.PullRequest, error) {
	r.request = request
	return integration.PullRequest{Metadata: map[string]any{"title": "safe"}}, nil
}

func TestRunFromTriggerIsolatesOtherTriggerBranches(t *testing.T) {
	definition := Definition{Key: "isolation", Name: "Isolation", Nodes: []Node{
		{Key: "manual", Type: "trigger", Name: "Manual", Config: map[string]any{"mode": "manual"}},
		{Key: "api", Type: "trigger", Name: "API", Config: map[string]any{"mode": "api"}},
		{Key: "manual-log", Type: "log", Name: "Manual log"},
		{Key: "api-log", Type: "log", Name: "API log"},
	}, Edges: []Edge{
		{Key: "manual-edge", FromNode: "manual", FromPort: "event", ToNode: "manual-log", ToPort: "input"},
		{Key: "api-edge", FromNode: "api", FromPort: "event", ToNode: "api-log", ToPort: "input"},
	}}
	report, err := RunFromTriggerWithAdapters(context.Background(), definition, DefaultCatalog(), "api", map[string]any{"value": "selected"}, Adapters{})
	if err != nil || len(report.Runs) != 2 || report.Runs[0].NodeKey != "api" || report.Runs[1].NodeKey != "api-log" {
		t.Fatalf("isolated report = %#v, %v", report, err)
	}
}

func TestRunFromTriggerIgnoresInactiveInputsAtConvergedNode(t *testing.T) {
	definition := Definition{Key: "converged", Name: "Converged", Nodes: []Node{
		{Key: "manual", Type: "trigger", Name: "Manual", Config: map[string]any{"mode": "manual"}},
		{Key: "api", Type: "trigger", Name: "API", Config: map[string]any{"mode": "api"}},
		{Key: "merged", Type: "merge", Name: "Merged"},
	}, Edges: []Edge{
		{Key: "manual-merge", FromNode: "manual", FromPort: "event", ToNode: "merged", ToPort: "inputs"},
		{Key: "api-merge", FromNode: "api", FromPort: "event", ToNode: "merged", ToPort: "inputs"},
	}}
	catalog := NewCatalog(
		CardType{Key: "trigger", Inputs: []Port{}, Outputs: []Port{{Key: "event", Contract: "any"}}, Available: true},
		CardType{Key: "merge", Inputs: []Port{{Key: "inputs", Contract: "any", Required: true, CollectAll: true}}, Outputs: []Port{{Key: "output", Contract: "any"}}, Available: true},
	)
	report, err := RunFromTriggerWithAdapters(context.Background(), definition, catalog, "api", map[string]any{"value": "selected"}, Adapters{})
	if err != nil || report.Status != "completed" || len(report.Runs) != 2 || report.Runs[1].NodeKey != "merged" {
		t.Fatalf("converged isolated report = %#v, %v", report, err)
	}
}

func TestFetchAndPublishPreferDynamicPullRequestTarget(t *testing.T) {
	config, _ := json.Marshal(map[string]string{"base_url": "https://gitea.example"})
	item := encryptedIntegration(t, integration.Integration{Key: "gitea", Name: "Gitea", Type: integration.TypeGitea, Config: config, Status: integration.StatusActive}, "token")
	reader := &targetReader{}
	fetch := Node{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"integration": "gitea", "owner": "legacy", "repo": "legacy", "pull_request": 1}}
	outputs, err := execute(context.Background(), fetch, map[string][]any{"event": {map[string]any{"pull_request": map[string]any{"owner": "acme", "repo": "api", "number": 42}}}}, nil, Adapters{Integrations: memoryIntegrations{"gitea": item}, Secrets: testSecrets(t), Gitea: reader}, rootScope)
	if err != nil || reader.request.Owner != "acme" || reader.request.Repo != "api" || reader.request.Number != 42 {
		t.Fatalf("dynamic fetch target = %#v, outputs = %#v, err = %v", reader.request, outputs, err)
	}
	publisher := &recordingPublisher{}
	ledger := &recordingPublicationLedger{}
	publish := Node{Key: "publish", Type: "publish", Name: "Publish", Config: map[string]any{"integration": "gitea", "owner": "legacy", "repo": "legacy", "pull_request": 1}}
	_, err = publishReview(context.Background(), publish, map[string][]any{"formatted_review": {FormattedReview{}}, "pull_request": {outputs["pull_request"]}}, Adapters{Integrations: memoryIntegrations{"gitea": item}, Secrets: testSecrets(t), GiteaWriter: publisher, Publications: ledger, Execution: ExecutionContext{ID: 1, VersionID: 1}}, rootScope)
	if err != nil || len(publisher.requests) != 1 || publisher.requests[0].Owner != "acme" || publisher.requests[0].Repo != "api" || publisher.requests[0].Number != 42 {
		t.Fatalf("dynamic publish target = %#v, err = %v", publisher.requests, err)
	}
}

func TestTriggerModeDefaultsToManualAndRejectsUnknownMode(t *testing.T) {
	if mode, err := TriggerMode(Node{Key: "legacy", Type: "trigger"}); err != nil || mode != "manual" {
		t.Fatalf("legacy mode = %q, %v", mode, err)
	}
	if _, err := TriggerMode(Node{Key: "bad", Type: "trigger", Config: map[string]any{"mode": "cron"}}); err == nil {
		t.Fatal("unknown trigger mode should fail")
	}
}
