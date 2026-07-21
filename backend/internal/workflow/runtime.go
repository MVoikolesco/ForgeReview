package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"forgereview/backend/internal/integration"
)

type Token struct {
	NodeKey  string `json:"node_key"`
	PortKey  string `json:"port_key"`
	Contract string `json:"contract"`
	Value    any    `json:"value"`
}

type NodeRun struct {
	NodeKey    string           `json:"node_key"`
	Status     string           `json:"status"`
	Error      string           `json:"error,omitempty"`
	DurationMS int64            `json:"duration_ms"`
	Inputs     map[string][]any `json:"inputs"`
	Outputs    []Token          `json:"outputs"`
}

type RunReport struct {
	Status string    `json:"status"`
	Runs   []NodeRun `json:"runs"`
}

// Execution is the worker-only execution payload. Its input is not included in
// the public execution-status response.
type Execution struct {
	ID        int64
	VersionID int64
	Input     map[string]any
}

// Adapters are the explicit boundary for controlled external card execution.
// A nil field leaves its card type unavailable.
type Adapters struct {
	Integrations integration.Lookup
	Gitea        integration.GiteaPullRequestReader
	GiteaWriter  integration.GiteaReviewWriter
	OpenAI       integration.ChatClient
	Ollama       integration.ChatClient
	Publications PublicationLedger
	Execution    ExecutionContext
	Dispatcher   ExecutionDispatcher
}

// ExecutionContext identifies the durable execution currently being run.
// Publication cards require it so their idempotency key is stable across
// duplicate dispatches of the same execution.
type ExecutionContext struct {
	ID        int64
	VersionID int64
}

// PublicationLedger persists the state transition before an external effect.
type PublicationLedger interface {
	BeginPublication(context.Context, PublicationAttempt) (PublicationAttempt, bool, error)
	CompletePublication(context.Context, string, integration.PublicationReceipt) error
	RetryPublication(context.Context, string, error) error
}

type PublicationAttempt struct {
	IdempotencyKey string
	ExecutionID    int64
	VersionID      int64
	NodeKey        string
	Status         string
	Receipt        integration.PublicationReceipt
}

// ExecutionDispatcher accepts durable execution IDs for asynchronous workers.
type ExecutionDispatcher interface {
	Enqueue(context.Context, int64) error
}

// Run preserves local-card execution without external adapters.
func Run(ctx context.Context, definition Definition, catalog Catalog, input map[string]any) (RunReport, error) {
	return RunWithAdapters(ctx, definition, catalog, input, Adapters{})
}

// RunWithAdapters executes registered cards. Fetch and model cards require a
// configured active integration, an injected adapter, and an environment-backed
// secret resolver; local card behavior is unchanged.
func RunWithAdapters(ctx context.Context, definition Definition, catalog Catalog, input map[string]any, adapters Adapters) (RunReport, error) {
	if err := Validate(definition, catalog); err != nil {
		return RunReport{}, err
	}
	nodes := make(map[string]Node, len(definition.Nodes))
	inboxes := map[string]map[string][]Token{}
	incoming := map[string]map[string]int{}
	edges := map[string][]Edge{}
	for _, node := range definition.Nodes {
		nodes[node.Key] = node
		inboxes[node.Key] = map[string][]Token{}
		incoming[node.Key] = map[string]int{}
	}
	for _, edge := range definition.Edges {
		edges[edge.FromNode] = append(edges[edge.FromNode], edge)
		incoming[edge.ToNode][edge.ToPort]++
	}
	ready := make([]string, 0)
	for _, node := range definition.Nodes {
		card, _ := catalog.Get(node.Type)
		if len(card.Inputs) == 0 {
			ready = append(ready, node.Key)
		}
	}
	run := map[string]bool{}
	report := RunReport{Status: "completed"}
	for len(ready) > 0 {
		if err := ctx.Err(); err != nil {
			return RunReport{Status: "cancelled", Runs: report.Runs}, err
		}
		nodeKey := ready[0]
		ready = ready[1:]
		if run[nodeKey] {
			continue
		}
		node := nodes[nodeKey]
		if !inputsReady(node, inboxes[nodeKey], incoming[nodeKey], catalog) {
			continue
		}
		run[nodeKey] = true
		started := time.Now()
		inputs := values(inboxes[nodeKey])
		outputs, err := execute(ctx, node, inputs, input, adapters)
		nodeRun := NodeRun{NodeKey: nodeKey, Status: "completed", DurationMS: time.Since(started).Milliseconds(), Inputs: inputs}
		if err != nil {
			nodeRun.Status, nodeRun.Error = "failed", err.Error()
			report.Status = "failed"
			report.Runs = append(report.Runs, nodeRun)
			return report, err
		}
		card, _ := catalog.Get(node.Type)
		for _, port := range card.Outputs {
			value, present := outputs[port.Key]
			if !present {
				continue
			}
			portKey := port.Key
			token := Token{NodeKey: nodeKey, PortKey: portKey, Contract: port.Contract, Value: value}
			nodeRun.Outputs = append(nodeRun.Outputs, token)
			for _, edge := range edges[nodeKey] {
				if edge.FromPort != portKey {
					continue
				}
				inboxes[edge.ToNode][edge.ToPort] = append(inboxes[edge.ToNode][edge.ToPort], token)
				if !run[edge.ToNode] && inputsReady(nodes[edge.ToNode], inboxes[edge.ToNode], incoming[edge.ToNode], catalog) {
					ready = append(ready, edge.ToNode)
				}
			}
		}
		report.Runs = append(report.Runs, nodeRun)
	}
	if len(run) != len(nodes) {
		return RunReport{Status: "blocked", Runs: report.Runs}, fmt.Errorf("workflow has blocked nodes")
	}
	return report, nil
}

func inputsReady(node Node, inbox map[string][]Token, incoming map[string]int, catalog Catalog) bool {
	card, _ := catalog.Get(node.Type)
	for _, port := range card.Inputs {
		if port.Required && len(inbox[port.Key]) == 0 {
			return false
		}
		if port.CollectAll && len(inbox[port.Key]) < incoming[port.Key] {
			return false
		}
	}
	return true
}

func values(inbox map[string][]Token) map[string][]any {
	result := map[string][]any{}
	for key, tokens := range inbox {
		for _, token := range tokens {
			result[key] = append(result[key], token.Value)
		}
	}
	return result
}

func execute(ctx context.Context, node Node, inputs map[string][]any, input map[string]any, adapters Adapters) (map[string]any, error) {
	first := func() any {
		for _, values := range inputs {
			if len(values) > 0 {
				return values[0]
			}
		}
		return nil
	}
	switch node.Type {
	case "trigger":
		if value, ok := node.Config["event"]; ok {
			return map[string]any{"event": value}, nil
		}
		return map[string]any{"event": input}, nil
	case "transform", "log":
		return map[string]any{"output": first()}, nil
	case "variable", "cache":
		return map[string]any{"value": first()}, nil
	case "filter":
		files, err := filterFiles(inputs["files"], node.Config)
		if err != nil {
			return nil, fmt.Errorf("filter card %q: %w", node.Key, err)
		}
		return map[string]any{"files": files}, nil
	case "group":
		groups, err := groupFiles(inputs["files"], node.Config)
		if err != nil {
			return nil, fmt.Errorf("group card %q: %w", node.Key, err)
		}
		return map[string]any{"groups": groups}, nil
	case "template":
		template, _ := node.Config["template"].(string)
		if template == "" {
			return nil, fmt.Errorf("template card %q requires config.template", node.Key)
		}
		return map[string]any{"prompt": template}, nil
	case "condition":
		expected, configured := node.Config["equals"]
		if configured && fmt.Sprint(first()) == fmt.Sprint(expected) {
			return map[string]any{"true": first()}, nil
		}
		if !configured && truthy(first()) {
			return map[string]any{"true": first()}, nil
		}
		return map[string]any{"false": first()}, nil
	case "merge":
		return map[string]any{"output": inputs}, nil
	case "validate":
		value, portKey := validateResponse(inputs["response"], inputs["files"], node.Config)
		return map[string]any{portKey: value}, nil
	case "response_filter":
		comments, err := filterFindings(inputs["response"], node.Config)
		if err != nil {
			return nil, fmt.Errorf("response_filter card %q: %w", node.Key, err)
		}
		return map[string]any{"comments": comments}, nil
	case "consolidate":
		review, err := consolidateFindings(inputs["comments"])
		if err != nil {
			return nil, fmt.Errorf("consolidate card %q: %w", node.Key, err)
		}
		return map[string]any{"review": review}, nil
	case "format":
		formatted, err := formatReview(inputs["review"])
		if err != nil {
			return nil, fmt.Errorf("format card %q: %w", node.Key, err)
		}
		return map[string]any{"formatted": formatted}, nil
	case "publish":
		return publishReview(ctx, node, inputs, adapters)
	case "fetch":
		item, err := configuredIntegration(ctx, node, adapters, "fetch")
		if err != nil {
			return nil, err
		}
		if item.Type != integration.TypeGitea {
			return nil, fmt.Errorf("fetch card %q integration %q must be type %q", node.Key, item.Key, integration.TypeGitea)
		}
		if adapters.Gitea == nil {
			return nil, fmt.Errorf("fetch card %q requires an injected Gitea adapter", node.Key)
		}
		secret, err := resolveSecret(item, adapters, "fetch", node.Key)
		if err != nil {
			return nil, err
		}
		request, err := pullRequestRequest(node)
		if err != nil {
			return nil, err
		}
		pullRequest, err := adapters.Gitea.ReadPullRequest(ctx, item, secret, request)
		if err != nil {
			return nil, fmt.Errorf("fetch card %q: %w", node.Key, err)
		}
		return map[string]any{"pull_request": pullRequest, "files": pullRequest.Files}, nil
	case "model":
		item, err := configuredIntegration(ctx, node, adapters, "model")
		if err != nil {
			return nil, err
		}
		var client integration.ChatClient
		switch item.Type {
		case integration.TypeOpenAI:
			client = adapters.OpenAI
		case integration.TypeOllama:
			client = adapters.Ollama
		default:
			return nil, fmt.Errorf("model card %q integration %q must be type %q or %q", node.Key, item.Key, integration.TypeOpenAI, integration.TypeOllama)
		}
		if client == nil {
			return nil, fmt.Errorf("model card %q requires an injected %s adapter", node.Key, item.Type)
		}
		secret, err := resolveSecret(item, adapters, "model", node.Key)
		if err != nil {
			return nil, err
		}
		prompt, ok := firstForPort(inputs, "prompt").(string)
		if !ok || prompt == "" {
			return nil, fmt.Errorf("model card %q requires a prompt input", node.Key)
		}
		response, err := client.Chat(ctx, item, secret, prompt)
		if err != nil {
			return nil, fmt.Errorf("model card %q: %w", node.Key, err)
		}
		return map[string]any{"response": response}, nil
	default:
		return nil, fmt.Errorf("card type %q has no configured executor", node.Type)
	}
}

func publishReview(ctx context.Context, node Node, inputs map[string][]any, adapters Adapters) (map[string]any, error) {
	item, err := configuredIntegration(ctx, node, adapters, "publish")
	if err != nil {
		return nil, err
	}
	if item.Type != integration.TypeGitea {
		return nil, fmt.Errorf("publish card %q integration %q must be type %q", node.Key, item.Key, integration.TypeGitea)
	}
	if adapters.GiteaWriter == nil {
		return nil, fmt.Errorf("publish card %q requires an injected Gitea writer adapter", node.Key)
	}
	if adapters.Publications == nil || adapters.Execution.ID < 1 || adapters.Execution.VersionID < 1 {
		return nil, fmt.Errorf("publish card %q requires a durable execution context and publication ledger", node.Key)
	}
	formatted, ok := firstForPort(inputs, "formatted_review").(FormattedReview)
	if !ok {
		return nil, fmt.Errorf("publish card %q requires a formatted_review input", node.Key)
	}
	request, err := pullRequestRequestForCard(node, "publish")
	if err != nil {
		return nil, err
	}
	key := publicationKey(adapters.Execution, node.Key)
	attempt, shouldPublish, err := adapters.Publications.BeginPublication(ctx, PublicationAttempt{IdempotencyKey: key, ExecutionID: adapters.Execution.ID, VersionID: adapters.Execution.VersionID, NodeKey: node.Key})
	if err != nil {
		return nil, fmt.Errorf("publish card %q could not persist idempotency state: %w", node.Key, err)
	}
	if !shouldPublish {
		return map[string]any{"receipt": attempt.Receipt}, nil
	}
	secret, err := resolveSecret(item, adapters, "publish", node.Key)
	if err != nil {
		_ = adapters.Publications.RetryPublication(ctx, key, err)
		return nil, err
	}
	receipt, err := adapters.GiteaWriter.PublishReview(ctx, item, secret, integration.GiteaReviewRequest{Owner: request.Owner, Repo: request.Repo, Number: request.Number, Body: formattedReviewBody(formatted), IdempotencyKey: key})
	if err != nil {
		_ = adapters.Publications.RetryPublication(ctx, key, err)
		return nil, fmt.Errorf("publish card %q: %w", node.Key, err)
	}
	receipt.Status, receipt.IdempotencyKey = "completed", key
	if err = adapters.Publications.CompletePublication(ctx, key, receipt); err != nil {
		return nil, fmt.Errorf("publish card %q could not complete idempotency state: %w", node.Key, err)
	}
	return map[string]any{"receipt": receipt}, nil
}

func publicationKey(execution ExecutionContext, nodeKey string) string {
	return fmt.Sprintf("forgereview:publication:%d:%d:%s", execution.ID, execution.VersionID, nodeKey)
}

func formattedReviewBody(review FormattedReview) string {
	lines := []string{fmt.Sprintf("## ForgeReview: %d finding(s)", review.Summary.Total)}
	for _, finding := range review.Findings {
		lines = append(lines, fmt.Sprintf("- **%s** `%s:%d` — %s", strings.ToUpper(finding.Severity), finding.Path, finding.Line, finding.Comment))
	}
	return strings.Join(lines, "\n")
}

func configuredIntegration(ctx context.Context, node Node, adapters Adapters, card string) (integration.Integration, error) {
	key, _ := node.Config["integration"].(string)
	if key == "" {
		return integration.Integration{}, fmt.Errorf("%s card %q requires config.integration", card, node.Key)
	}
	if adapters.Integrations == nil {
		return integration.Integration{}, fmt.Errorf("%s card %q requires an injected integration lookup", card, node.Key)
	}
	item, err := adapters.Integrations.Integration(ctx, key)
	if err != nil {
		return integration.Integration{}, fmt.Errorf("%s card %q integration %q is not configured: %w", card, node.Key, key, err)
	}
	if item.Status != integration.StatusActive {
		return integration.Integration{}, fmt.Errorf("%s card %q integration %q is not active", card, node.Key, key)
	}
	return item, nil
}

func resolveSecret(item integration.Integration, adapters Adapters, card, nodeKey string) (string, error) {
	secret, err := (integration.EnvironmentSecrets{}).Resolve(item.SecretReference)
	if err != nil {
		return "", fmt.Errorf("%s card %q could not resolve integration %q secret: %w", card, nodeKey, item.Key, err)
	}
	return secret, nil
}

func pullRequestRequest(node Node) (integration.PullRequestRequest, error) {
	return pullRequestRequestForCard(node, "fetch")
}

func pullRequestRequestForCard(node Node, card string) (integration.PullRequestRequest, error) {
	owner, _ := node.Config["owner"].(string)
	repo, _ := node.Config["repo"].(string)
	number, ok := integer(node.Config["pull_request"])
	if owner == "" || repo == "" || !ok || number < 1 {
		return integration.PullRequestRequest{}, fmt.Errorf("%s card %q requires config.owner, config.repo, and positive config.pull_request", card, node.Key)
	}
	return integration.PullRequestRequest{Owner: owner, Repo: repo, Number: number}, nil
}

func integer(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case float64:
		return int(number), number == float64(int(number))
	default:
		return 0, false
	}
}

func firstForPort(inputs map[string][]any, key string) any {
	if values := inputs[key]; len(values) > 0 {
		return values[0]
	}
	return nil
}

func truthy(value any) bool {
	return value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" && fmt.Sprint(value) != "false"
}
