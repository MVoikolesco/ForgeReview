package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"forgereview/backend/internal/integration"
)

type Token struct {
	NodeKey  string `json:"node_key"`
	PortKey  string `json:"port_key"`
	Contract string `json:"contract"`
	ScopeKey string `json:"scope_key"`
	Value    any    `json:"value"`
}

// ErrorToken intentionally carries only stable execution context. It never
// includes provider bodies, prompts, credentials, or an underlying error text.
type ErrorToken struct {
	Code     string `json:"code"`
	NodeKey  string `json:"node_key"`
	ScopeKey string `json:"scope_key"`
}

type NodeRun struct {
	NodeKey    string           `json:"node_key"`
	ScopeKey   string           `json:"scope_key"`
	Status     string           `json:"status"`
	Error      string           `json:"error,omitempty"`
	DurationMS int64            `json:"duration_ms"`
	Inputs     map[string][]any `json:"inputs"`
	Outputs    []Token          `json:"outputs"`
	Metadata   map[string]any   `json:"metadata,omitempty"`
}

type RunReport struct {
	Status string    `json:"status"`
	Runs   []NodeRun `json:"runs"`
}

// ExecutionFailure keeps the public error intentionally generic while allowing
// the durable worker to classify a wrapped provider failure for retry policy.
type ExecutionFailure struct {
	NodeKey string
	Cause   error
}

func (e *ExecutionFailure) Error() string { return fmt.Sprintf("card %q execution failed", e.NodeKey) }
func (e *ExecutionFailure) Unwrap() error { return e.Cause }

// ProgressObserver is the minimal runtime boundary for durable live feedback.
// A node is observed once as running and again with its terminal state.
type ProgressObserver interface {
	ObserveNode(context.Context, NodeRun) error
}

type ProgressObserverFunc func(context.Context, NodeRun) error

func (observe ProgressObserverFunc) ObserveNode(ctx context.Context, run NodeRun) error {
	return observe(ctx, run)
}

// ExecutionTelemetry accumulates only safe provider accounting fields. It must
// never contain prompts, responses, provider payloads, or credentials.
type ExecutionTelemetry struct {
	mu         sync.Mutex
	startedAt  time.Time
	models     []string
	prompt     int
	completion int
	total      int
	lastModel  string
	lastUsage  integration.TokenUsage
}

type TelemetrySnapshot struct {
	ElapsedMS  int64
	Models     []string
	Prompt     int
	Completion int
	Total      int
}

func NewExecutionTelemetry(startedAt time.Time) *ExecutionTelemetry {
	return &ExecutionTelemetry{startedAt: startedAt}
}

func (telemetry *ExecutionTelemetry) Record(result integration.ChatResult) {
	if telemetry == nil {
		return
	}
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	if model := strings.TrimSpace(result.Model); model != "" {
		seen := false
		for _, existing := range telemetry.models {
			seen = seen || existing == model
		}
		if !seen {
			telemetry.models = append(telemetry.models, model)
		}
	}
	telemetry.prompt += positiveInt(result.Usage.Prompt)
	telemetry.completion += positiveInt(result.Usage.Completion)
	telemetry.total += positiveInt(result.Usage.Total)
	telemetry.lastModel = strings.TrimSpace(result.Model)
	telemetry.lastUsage = result.Usage
}

func (telemetry *ExecutionTelemetry) LastCallMetadata() map[string]any {
	if telemetry == nil {
		return nil
	}
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	metadata := map[string]any{}
	if telemetry.lastModel != "" {
		metadata["model"] = telemetry.lastModel
	}
	if telemetry.lastUsage.Prompt > 0 {
		metadata["prompt_tokens"] = telemetry.lastUsage.Prompt
	}
	if telemetry.lastUsage.Completion > 0 {
		metadata["completion_tokens"] = telemetry.lastUsage.Completion
	}
	if telemetry.lastUsage.Total > 0 {
		metadata["total_tokens"] = telemetry.lastUsage.Total
	}
	return metadata
}

func (telemetry *ExecutionTelemetry) Snapshot() TelemetrySnapshot {
	if telemetry == nil {
		return TelemetrySnapshot{}
	}
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	total := telemetry.total
	if total == 0 && (telemetry.prompt > 0 || telemetry.completion > 0) {
		total = telemetry.prompt + telemetry.completion
	}
	return TelemetrySnapshot{ElapsedMS: time.Since(telemetry.startedAt).Milliseconds(), Models: append([]string(nil), telemetry.models...), Prompt: telemetry.prompt, Completion: telemetry.completion, Total: total}
}

func positiveInt(value int) int {
	if value > 0 {
		return value
	}
	return 0
}

// Execution is the worker-only execution payload. Its input is not included in
// the public execution-status response.
type Execution struct {
	ID             int64
	VersionID      int64
	TriggerNodeKey string
	Input          map[string]any
}

// Adapters are the explicit boundary for controlled external card execution.
// A nil field leaves its card type unavailable.
type Adapters struct {
	Integrations  integration.Lookup
	ModelProfiles integration.ModelProfileLookup
	Secrets       integration.SecretManager
	Gitea         integration.GiteaPullRequestReader
	GiteaWriter   integration.GiteaReviewWriter
	OpenAI        integration.ChatClient
	Ollama        integration.ChatClient
	Publications  PublicationLedger
	Execution     ExecutionContext
	Dispatcher    ExecutionDispatcher
	Cache         Cache
	Progress      ProgressObserver
	Logs          ExecutionLogWriter
	// Cancellation is queried at card boundaries. Publication intentionally
	// begins atomically in the ledger and is never cancelled once begun.
	Cancellation func(context.Context) (bool, error)
	Telemetry    *ExecutionTelemetry
}

// ExecutionLogWriter persists safe, operator-facing lifecycle records. Entries
// deliberately exclude token values, prompts, provider responses and secrets.
type ExecutionLogWriter interface {
	WriteExecutionLog(context.Context, ExecutionLogEntry) error
}

type ExecutionLogEntry struct {
	ExecutionID int64          `json:"execution_id"`
	VersionID   int64          `json:"version_id"`
	NodeKey     string         `json:"node_key"`
	NodeName    string         `json:"node_name"`
	NodeType    string         `json:"node_type"`
	ScopeKey    string         `json:"scope_key,omitempty"`
	Event       string         `json:"event"`
	Status      string         `json:"status"`
	DurationMS  int64          `json:"duration_ms,omitempty"`
	Facts       map[string]any `json:"facts,omitempty"`
	Error       string         `json:"error,omitempty"`
	Inputs      any            `json:"inputs,omitempty"`
	Outputs     any            `json:"outputs,omitempty"`
	OccurredAt  time.Time      `json:"occurred_at"`
}

// ExecutionLogDiagnostic keeps request/response evidence useful to an
// operator, while removing credential-bearing fields before it leaves the
// sensitive execution store or reaches the project log file.
func ExecutionLogDiagnostic(value any) any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "[não foi possível serializar o diagnóstico]"
	}
	var copied any
	if json.Unmarshal(raw, &copied) != nil {
		return "[não foi possível ler o diagnóstico]"
	}
	return redactExecutionLogValue(copied)
}

func redactExecutionLogValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, child := range item {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "authorization") || strings.Contains(lower, "credential") || strings.Contains(lower, "cookie") || strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") {
				result[key] = "[REDACTED]"
				continue
			}
			result[key] = redactExecutionLogValue(child)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for i := range item {
			result[i] = redactExecutionLogValue(item[i])
		}
		return result
	default:
		return value
	}
}

// SafeExecutionLogFacts selects the few runtime facts useful for diagnosis.
// It is intentionally allowlisted so payloads, prompts and provider bodies
// cannot be written to the physical log or returned by the status API.
func SafeExecutionLogFacts(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	allowed := map[string]bool{"attempt_count": true, "retry_limit": true, "retry_delay_ms": true, "completed_iterations": true, "failed_iterations": true, "max_iterations": true, "concurrency": true, "model": true, "validation_error": true, "error_code": true, "error_policy": true, "error_action": true, "error_scope": true}
	result := map[string]any{}
	for key := range allowed {
		value, ok := metadata[key]
		if !ok {
			continue
		}
		switch value.(type) {
		case string, float64, bool, int, int64:
			result[key] = value
		}
	}
	for _, key := range []string{"validation_attempts", "provider_calls"} {
		values, ok := metadata[key].([]any)
		if !ok {
			continue
		}
		attempts := make([]map[string]any, 0, len(values))
		for _, raw := range values {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			attempt := map[string]any{}
			if value, ok := item["attempt"].(float64); ok {
				attempt["attempt"] = int(value)
			}
			if value, ok := item["status"].(string); ok {
				attempt["status"] = value
			}
			if len(attempt) > 0 {
				attempts = append(attempts, attempt)
			}
		}
		if len(attempts) > 0 {
			result[key] = attempts
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
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

// Cache is the explicit boundary for the ephemeral cache card. Values are
// JSON-encoded by the runner so implementations never receive Go objects.
type Cache interface {
	Get(context.Context, string) ([]byte, bool, error)
	Set(context.Context, string, []byte, time.Duration) error
	Delete(context.Context, string) error
}

// Run preserves local-card execution without external adapters.
func Run(ctx context.Context, definition Definition, catalog Catalog, input map[string]any) (RunReport, error) {
	return RunWithAdapters(ctx, definition, catalog, input, Adapters{})
}

// RunWithAdapters executes registered cards. Fetch and model cards require a
// configured active integration, an injected adapter, and an encrypted-secret
// resolver; local card behavior is unchanged.
func RunWithAdapters(ctx context.Context, definition Definition, catalog Catalog, input map[string]any, adapters Adapters) (RunReport, error) {
	trigger := ""
	for _, node := range definition.Nodes {
		if node.Type == "trigger" {
			trigger = node.Key
			break
		}
	}
	return RunFromTriggerWithAdapters(ctx, definition, catalog, trigger, input, adapters)
}

// RunFromTriggerWithAdapters activates only the selected trigger and nodes
// reachable from it. Other zero-input triggers and their branches stay idle.
func RunFromTriggerWithAdapters(ctx context.Context, definition Definition, catalog Catalog, triggerNodeKey string, input map[string]any, adapters Adapters) (RunReport, error) {
	if err := Validate(definition, catalog); err != nil {
		return RunReport{}, err
	}
	if triggerNodeKey == "" {
		for _, node := range definition.Nodes {
			if node.Type == "trigger" {
				triggerNodeKey = node.Key
				break
			}
		}
	}
	active := map[string]bool{}
	if triggerNodeKey == "" {
		for _, node := range definition.Nodes {
			active[node.Key] = true
		}
	} else {
		selected, ok := nodeFor(definition.Nodes, triggerNodeKey)
		if !ok || selected.Type != "trigger" {
			return RunReport{}, fmt.Errorf("selected trigger %q does not exist", triggerNodeKey)
		}
		active = reachableNodes(definition, triggerNodeKey)
	}
	if adapters.Telemetry == nil {
		adapters.Telemetry = NewExecutionTelemetry(time.Now())
	}
	runner := newScopedRunner(definition, catalog, input, adapters, active)
	ready := make([]nodeScope, 0)
	for _, node := range definition.Nodes {
		if !active[node.Key] || (triggerNodeKey != "" && node.Type == "trigger" && node.Key != triggerNodeKey) {
			continue
		}
		card, _ := catalog.Get(node.Type)
		if len(card.Inputs) == 0 || (node.Type == "cache" && node.Config["mode"] != "write") {
			ready = append(ready, nodeScope{NodeKey: node.Key, ScopeKey: rootScope})
		}
	}
	for len(ready) > 0 {
		if err := ctx.Err(); err != nil {
			return RunReport{Status: "cancelled", Runs: runner.report.Runs}, err
		}
		current := ready[0]
		ready = ready[1:]
		if err := runner.runNode(ctx, current, &ready); err != nil {
			if runner.report.Status != "cancelled" {
				runner.report.Status = "failed"
			}
			return runner.report, err
		}
	}
	for _, node := range definition.Nodes {
		if active[node.Key] && !runner.scopeOnly[node.Key] && !runner.ran(nodeScope{NodeKey: node.Key, ScopeKey: rootScope}) {
			return RunReport{Status: "blocked", Runs: runner.report.Runs}, fmt.Errorf("workflow has blocked nodes")
		}
	}
	return runner.report, nil
}

func reachableNodes(definition Definition, start string) map[string]bool {
	edges := map[string][]string{}
	for _, edge := range definition.Edges {
		edges[edge.FromNode] = append(edges[edge.FromNode], edge.ToNode)
	}
	active, queue := map[string]bool{}, []string{start}
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if active[key] {
			continue
		}
		active[key] = true
		queue = append(queue, edges[key]...)
	}
	return active
}

const rootScope = "root"

type nodeScope struct {
	NodeKey  string
	ScopeKey string
}

type scopedRunner struct {
	nodes     map[string]Node
	inboxes   map[string]map[string]map[string][]Token
	incoming  map[string]map[string]int
	edges     map[string][]Edge
	run       map[nodeScope]bool
	scopeOnly map[string]bool
	catalog   Catalog
	input     map[string]any
	adapters  Adapters
	report    RunReport
	results   map[string][]any
	active    map[string]bool
	limits    *executionLimits
}

type executionLimits struct {
	global   chan struct{}
	external sync.Mutex
}

func newScopedRunner(definition Definition, catalog Catalog, input map[string]any, adapters Adapters, active map[string]bool) *scopedRunner {
	runner := &scopedRunner{
		nodes:     make(map[string]Node, len(definition.Nodes)),
		inboxes:   map[string]map[string]map[string][]Token{},
		incoming:  map[string]map[string]int{},
		edges:     map[string][]Edge{},
		run:       map[nodeScope]bool{},
		scopeOnly: loopScopedNodes(definition),
		catalog:   catalog,
		input:     input,
		adapters:  adapters,
		report:    RunReport{Status: "completed"},
		results:   map[string][]any{},
		active:    active,
		limits:    &executionLimits{global: make(chan struct{}, 8)},
	}
	for _, node := range definition.Nodes {
		runner.nodes[node.Key] = node
		runner.incoming[node.Key] = map[string]int{}
	}
	for _, edge := range definition.Edges {
		// Inactive trigger branches must not contribute required or collecting
		// input counts when multiple trigger branches converge downstream.
		if !active[edge.FromNode] || !active[edge.ToNode] {
			continue
		}
		runner.edges[edge.FromNode] = append(runner.edges[edge.FromNode], edge)
		runner.incoming[edge.ToNode][edge.ToPort]++
	}
	return runner
}

func (r *scopedRunner) child() *scopedRunner {
	return &scopedRunner{nodes: r.nodes, inboxes: map[string]map[string]map[string][]Token{}, incoming: r.incoming, edges: r.edges, run: map[nodeScope]bool{}, scopeOnly: r.scopeOnly, catalog: r.catalog, input: r.input, adapters: r.adapters, report: RunReport{Status: "completed"}, results: map[string][]any{}, active: r.active, limits: r.limits}
}

func loopScopedNodes(definition Definition) map[string]bool {
	edges := map[string][]Edge{}
	rootBoundaries := map[string]bool{}
	for _, edge := range definition.Edges {
		edges[edge.FromNode] = append(edges[edge.FromNode], edge)
		from, _ := nodeFor(definition.Nodes, edge.FromNode)
		if from.Type == "loop" && edge.FromPort == "results" {
			rootBoundaries[edge.ToNode] = true
		}
	}
	result, queue := map[string]bool{}, []string{}
	for _, node := range definition.Nodes {
		if node.Type != "loop" {
			continue
		}
		for _, edge := range edges[node.Key] {
			if edge.FromPort == "item" {
				queue = append(queue, edge.ToNode)
			}
		}
	}
	for len(queue) > 0 {
		nodeKey := queue[0]
		queue = queue[1:]
		if result[nodeKey] {
			continue
		}
		result[nodeKey] = true
		for _, edge := range edges[nodeKey] {
			if !rootBoundaries[edge.ToNode] {
				queue = append(queue, edge.ToNode)
			}
		}
	}
	return result
}

func nodeFor(nodes []Node, key string) (Node, bool) {
	for _, node := range nodes {
		if node.Key == key {
			return node, true
		}
	}
	return Node{}, false
}

func (r *scopedRunner) ran(current nodeScope) bool { return r.run[current] }

func (r *scopedRunner) inbox(current nodeScope) map[string][]Token {
	if r.inboxes[current.ScopeKey] == nil {
		r.inboxes[current.ScopeKey] = map[string]map[string][]Token{}
	}
	if r.inboxes[current.ScopeKey][current.NodeKey] == nil {
		r.inboxes[current.ScopeKey][current.NodeKey] = map[string][]Token{}
	}
	return r.inboxes[current.ScopeKey][current.NodeKey]
}

func (r *scopedRunner) route(token Token, edge Edge, scopeKey string, ready *[]nodeScope) {
	target := nodeScope{NodeKey: edge.ToNode, ScopeKey: scopeKey}
	inbox := r.inbox(target)
	inbox[edge.ToPort] = append(inbox[edge.ToPort], token)
	if !r.ran(target) && inputsReady(r.nodes[target.NodeKey], inbox, r.incoming[target.NodeKey], r.catalog) {
		*ready = append(*ready, target)
	}
}

func (r *scopedRunner) runNode(ctx context.Context, current nodeScope, ready *[]nodeScope) error {
	node := r.nodes[current.NodeKey]
	inbox := r.inbox(current)
	if r.ran(current) || !inputsReady(node, inbox, r.incoming[node.Key], r.catalog) {
		return nil
	}
	r.run[current] = true
	if err := r.acquire(ctx, r.nodes[current.NodeKey].Type); err != nil {
		if errors.Is(err, context.Canceled) {
			r.report.Status = "cancelled"
		}
		return err
	}
	defer r.release(r.nodes[current.NodeKey].Type)
	if r.adapters.Cancellation != nil {
		cancelled, err := r.adapters.Cancellation(ctx)
		if err != nil {
			return err
		}
		if cancelled {
			r.report.Status = "cancelled"
			return context.Canceled
		}
	}
	started := time.Now()
	inputs := values(inbox)
	nodeRun := NodeRun{NodeKey: node.Key, ScopeKey: current.ScopeKey, Status: "running", Inputs: inputs}
	if err := r.observe(ctx, nodeRun); err != nil {
		return fmt.Errorf("persist node progress: %w", err)
	}
	var outputs map[string]any
	var err error
	if node.Type == "loop" {
		if current.ScopeKey != rootScope {
			err = fmt.Errorf("loop card %q does not support nested scopes", node.Key)
		} else {
			outputs, nodeRun.Metadata, err = r.runLoop(ctx, node, inputs, &nodeRun)
		}
	} else if node.Type == "validate" {
		outputs, nodeRun.Metadata, err = r.runValidate(ctx, node, current, inbox, inputs)
	} else {
		outputs, err = execute(ctx, node, inputs, r.input, r.adapters, current.ScopeKey)
		if node.Type == "model" && err == nil {
			nodeRun.Metadata = map[string]any{"attempt_count": 1, "provider_calls": []any{map[string]any{"attempt": 1, "status": "completed"}}}
			for key, value := range r.adapters.Telemetry.LastCallMetadata() {
				nodeRun.Metadata[key] = value
			}
		}
	}
	nodeRun.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			nodeRun.Status = "cancelled"
			r.report.Status = "cancelled"
			r.report.Runs = append(r.report.Runs, nodeRun)
			if observeErr := r.observe(ctx, nodeRun); observeErr != nil {
				return fmt.Errorf("persist node progress: %w", observeErr)
			}
			return err
		}
		return r.handleFailure(ctx, node, current, nodeRun, err, ready)
	}
	nodeRun.Status = "completed"
	card, _ := r.catalog.Get(node.Type)
	for _, port := range card.Outputs {
		value, present := outputs[port.Key]
		if !present {
			continue
		}
		token := Token{NodeKey: node.Key, PortKey: port.Key, Contract: port.Contract, ScopeKey: current.ScopeKey, Value: value}
		nodeRun.Outputs = append(nodeRun.Outputs, token)
		for _, edge := range r.edges[node.Key] {
			if edge.FromPort == port.Key {
				r.route(token, edge, current.ScopeKey, ready)
			}
		}
	}
	if current.ScopeKey != rootScope && len(r.edges[node.Key]) == 0 {
		for _, output := range nodeRun.Outputs {
			r.results[current.ScopeKey] = append(r.results[current.ScopeKey], output.Value)
		}
	}
	r.report.Runs = append(r.report.Runs, nodeRun)
	return r.observe(ctx, nodeRun)
}

func (r *scopedRunner) acquire(ctx context.Context, cardType string) error {
	select {
	case r.limits.global <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	if cardType == "fetch" || cardType == "model" || cardType == "publish" {
		// sync.Mutex has no context-aware acquisition. Waiting here is bounded by
		// the in-flight provider call; it avoids a cancellation-path goroutine
		// acquiring the mutex after its caller has returned and wedging all later
		// provider calls. Provider calls are deliberately serialized.
		r.limits.external.Lock()
		if err := ctx.Err(); err != nil {
			r.limits.external.Unlock()
			<-r.limits.global
			return err
		}
	}
	return nil
}

func (r *scopedRunner) release(cardType string) {
	if cardType == "fetch" || cardType == "model" || cardType == "publish" {
		r.limits.external.Unlock()
	}
	<-r.limits.global
}

func (r *scopedRunner) observe(ctx context.Context, run NodeRun) error {
	if r.adapters.Progress == nil {
		return nil
	}
	return r.adapters.Progress.ObserveNode(ctx, run)
}

func (r *scopedRunner) handleFailure(ctx context.Context, node Node, current nodeScope, nodeRun NodeRun, cause error, ready *[]nodeScope) error {
	policy := "fail"
	if node.Type != "error_control" {
		policy = errorPolicyForNode(node)
	}
	if nodeRun.Metadata == nil {
		nodeRun.Metadata = map[string]any{}
	}
	nodeRun.Metadata["error_policy"] = policy
	nodeRun.Metadata["error_code"] = "execution_failed"
	nodeRun.Metadata["error_scope"] = current.ScopeKey
	nodeRun.Error = fmt.Sprintf("%v", ExecutionLogDiagnostic(cause.Error()))

	switch policy {
	case "continue":
		nodeRun.Status = "failed"
		nodeRun.Metadata["error_action"] = "continued"
		r.report.Runs = append(r.report.Runs, nodeRun)
		return r.observe(ctx, nodeRun)
	case "partial":
		nodeRun.Status = "partial"
		nodeRun.Metadata["error_action"] = "partial"
		r.report.Runs = append(r.report.Runs, nodeRun)
		return r.observe(ctx, nodeRun)
	case "route":
		nodeRun.Status = "failed"
		nodeRun.Metadata["error_action"] = "routed"
		token := Token{NodeKey: node.Key, PortKey: "error", Contract: "error", ScopeKey: current.ScopeKey, Value: ErrorToken{Code: "execution_failed", NodeKey: node.Key, ScopeKey: current.ScopeKey}}
		nodeRun.Outputs = append(nodeRun.Outputs, token)
		for _, edge := range r.edges[node.Key] {
			if edge.FromPort == "error" {
				r.route(token, edge, current.ScopeKey, ready)
			}
		}
		r.report.Runs = append(r.report.Runs, nodeRun)
		return r.observe(ctx, nodeRun)
	default:
		nodeRun.Status = "failed"
		r.report.Runs = append(r.report.Runs, nodeRun)
		if err := r.observe(ctx, nodeRun); err != nil {
			return fmt.Errorf("persist node progress: %w", err)
		}
		return &ExecutionFailure{NodeKey: node.Key, Cause: cause}
	}
}

const (
	maxModelRetryLimit    = 3
	maxModelRetryDelay    = 60000
	defaultModelMaxTokens = 2000
	maxModelMaxTokens     = 128000
)

type modelSettings struct {
	RetryLimit   int
	RetryDelayMS int
	MaxTokens    int
}

const maxCacheTTLSeconds = 86400

type cacheSettings struct {
	Key        string
	Mode       string
	TTLSeconds int
}

func cacheSettingsFor(node Node) (cacheSettings, error) {
	key, _ := node.Config["key"].(string)
	if strings.TrimSpace(key) == "" {
		return cacheSettings{}, fmt.Errorf("cache card %q requires config.key", node.Key)
	}
	mode, _ := node.Config["mode"].(string)
	if mode != "read" && mode != "write" && mode != "delete" {
		return cacheSettings{}, fmt.Errorf("cache card %q config.mode must be \"read\", \"write\", or \"delete\"", node.Key)
	}
	settings := cacheSettings{Key: key, Mode: mode}
	if mode == "write" {
		ttl, ok := integer(node.Config["ttl_seconds"])
		if !ok || ttl < 1 || ttl > maxCacheTTLSeconds {
			return cacheSettings{}, fmt.Errorf("cache card %q config.ttl_seconds must be between 1 and %d for write mode", node.Key, maxCacheTTLSeconds)
		}
		settings.TTLSeconds = ttl
	}
	return settings, nil
}

func modelSettingsFor(node Node) (modelSettings, error) {
	settings := modelSettings{MaxTokens: defaultModelMaxTokens}
	if value, exists := node.Config["max_tokens"]; exists {
		limit, ok := integer(value)
		if !ok || limit < 1 || limit > maxModelMaxTokens {
			return modelSettings{}, fmt.Errorf("model card %q config.max_tokens must be between 1 and %d", node.Key, maxModelMaxTokens)
		}
		settings.MaxTokens = limit
	}
	if value, exists := node.Config["retry_limit"]; exists {
		limit, ok := integer(value)
		if !ok || limit < 0 || limit > maxModelRetryLimit {
			return modelSettings{}, fmt.Errorf("model card %q config.retry_limit must be between 0 and %d", node.Key, maxModelRetryLimit)
		}
		settings.RetryLimit = limit
	}
	if value, exists := node.Config["retry_delay_ms"]; exists {
		delay, ok := integer(value)
		if !ok || delay < 0 || delay > maxModelRetryDelay {
			return modelSettings{}, fmt.Errorf("model card %q config.retry_delay_ms must be between 0 and %d", node.Key, maxModelRetryDelay)
		}
		settings.RetryDelayMS = delay
	}
	return settings, nil
}

// runValidate performs bounded corrective calls only for the direct model ->
// validate response edge in the same scope. It creates no graph back edge.
func (r *scopedRunner) runValidate(ctx context.Context, node Node, current nodeScope, inbox map[string][]Token, inputs map[string][]any) (map[string]any, map[string]any, error) {
	value, portKey := validateResponse(inputs["response"], inputs["files"], node.Config)
	validationAttempts := []any{map[string]any{"attempt": 1, "status": portKey}}
	providerCalls := []any{}
	metadata := map[string]any{"attempt_count": 1, "validation_attempts": validationAttempts, "provider_calls": providerCalls}
	if detail := validationFailureDetail(value); detail != "" {
		metadata["validation_error"] = detail
	}
	if portKey == "valid" {
		return map[string]any{portKey: value}, metadata, nil
	}
	model, prompt, settings, eligible := r.correctiveModel(current, inbox)
	if !eligible || settings.RetryLimit == 0 {
		return map[string]any{portKey: value}, metadata, nil
	}
	metadata["retry_limit"] = settings.RetryLimit
	metadata["retry_delay_ms"] = settings.RetryDelayMS
	for retry := 1; retry <= settings.RetryLimit; retry++ {
		if settings.RetryDelayMS > 0 {
			timer := time.NewTimer(time.Duration(settings.RetryDelayMS) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, metadata, ctx.Err()
			case <-timer.C:
			}
		}
		response, err := modelResponse(ctx, model, repairPrompt(prompt, validationFailureDetail(value)), r.adapters)
		call := map[string]any{"attempt": retry + 1, "status": "completed"}
		if err != nil {
			call["status"] = "failed"
			providerCalls = append(providerCalls, call)
			metadata["provider_calls"] = providerCalls
			return nil, metadata, err
		}
		providerCalls = append(providerCalls, call)
		value, portKey = validateResponse([]any{response.Content}, inputs["files"], node.Config)
		if detail := validationFailureDetail(value); detail != "" {
			metadata["validation_error"] = detail
		} else {
			delete(metadata, "validation_error")
		}
		validationAttempts = append(validationAttempts, map[string]any{"attempt": retry + 1, "status": portKey})
		metadata["attempt_count"] = len(validationAttempts)
		metadata["validation_attempts"] = validationAttempts
		metadata["provider_calls"] = providerCalls
		if portKey == "valid" {
			return map[string]any{portKey: value}, metadata, nil
		}
	}
	return map[string]any{portKey: value}, metadata, nil
}

func validationFailureDetail(value any) string {
	failure, ok := value.(ValidationFailure)
	if !ok || len(failure.Errors) == 0 {
		return ""
	}
	return strings.Join(failure.Errors, "; ")
}

func (r *scopedRunner) correctiveModel(current nodeScope, inbox map[string][]Token) (Node, string, modelSettings, bool) {
	responses := inbox["response"]
	if len(responses) != 1 || responses[0].ScopeKey != current.ScopeKey || responses[0].PortKey != "response" {
		return Node{}, "", modelSettings{}, false
	}
	model, exists := r.nodes[responses[0].NodeKey]
	if !exists || model.Type != "model" {
		return Node{}, "", modelSettings{}, false
	}
	settings, err := modelSettingsFor(model)
	if err != nil {
		return Node{}, "", modelSettings{}, false
	}
	prompt, ok := firstForPort(values(r.inbox(nodeScope{NodeKey: model.Key, ScopeKey: current.ScopeKey})), "prompt").(string)
	if !ok || prompt == "" {
		return Node{}, "", modelSettings{}, false
	}
	return model, prompt, settings, true
}

func repairPrompt(prompt, validationError string) string {
	message := strings.TrimSpace(prompt) + "\n\nA resposta anterior foi REJEITADA pelo validador."
	if strings.TrimSpace(validationError) != "" {
		message += " Motivo exato: " + validationError + "."
	}
	return message + "\n\nCorrija a resposta agora. Retorne SOMENTE JSON puro: sem markdown, sem bloco ```json e sem texto antes/depois.\n" +
		"O formato obrigatório é uma lista JSON, por exemplo:\n" +
		`[{"path":"arquivo.ext","line":12,"comment":"explique o problema encontrado","severity":"low"}]` +
		"\nCada item precisa ter: path (arquivo existente), line (inteiro positivo), comment (texto não vazio) e severity (low, medium, high ou critical)."
}

func (r *scopedRunner) runLoop(ctx context.Context, node Node, inputs map[string][]any, nodeRun *NodeRun) (map[string]any, map[string]any, error) {
	settings, err := loopSettingsFor(node)
	if err != nil {
		return nil, nil, err
	}
	items, err := loopItems(inputs["items"])
	if err != nil {
		return nil, nil, fmt.Errorf("loop card %q: %w", node.Key, err)
	}
	if len(items) > settings.MaxIterations {
		return nil, nil, fmt.Errorf("loop card %q received %d items, exceeding config.max_iterations %d", node.Key, len(items), settings.MaxIterations)
	}
	metadata := map[string]any{"max_iterations": settings.MaxIterations, "concurrency": settings.Concurrency, "on_error": settings.OnError}
	type outcome struct {
		report  []NodeRun
		results []any
		err     error
	}
	outcomes := make([]outcome, len(items))
	semaphore := make(chan struct{}, settings.Concurrency)
	var wait sync.WaitGroup
	for index, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, metadata, err
		}
		scopeKey := fmt.Sprintf("%s:%06d", node.Key, index+1)
		itemToken := Token{NodeKey: node.Key, PortKey: "item", Contract: "any", ScopeKey: scopeKey, Value: item}
		nodeRun.Outputs = append(nodeRun.Outputs, itemToken)
		run := func(index int, scopeKey string, token Token) {
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				outcomes[index].err = ctx.Err()
				return
			}
			defer func() { <-semaphore }()
			child := r.child() // every iteration owns its maps and report; no scoped map is shared.
			ready := make([]nodeScope, 0)
			for _, edge := range child.edges[node.Key] {
				if edge.FromPort == "item" {
					child.route(token, edge, scopeKey, &ready)
				}
			}
			for len(ready) > 0 {
				current := ready[0]
				ready = ready[1:]
				if runErr := child.runNode(ctx, current, &ready); runErr != nil {
					if settings.OnError == "route" {
						child.routeLoopError(node, scopeKey, &ready)
						continue
					}
					outcomes[index].err = runErr
					break
				}
			}
			outcomes[index].report = child.report.Runs
			outcomes[index].results = child.results[scopeKey]
		}
		if settings.Concurrency == 1 {
			run(index, scopeKey, itemToken)
		} else {
			wait.Add(1)
			go func() { defer wait.Done(); run(index, scopeKey, itemToken) }()
		}
	}
	wait.Wait()
	results := make([]any, 0)
	for index, outcome := range outcomes {
		r.report.Runs = append(r.report.Runs, outcome.report...)
		if outcome.err != nil {
			if errors.Is(outcome.err, context.Canceled) {
				return nil, metadata, outcome.err
			}
			if settings.OnError != "partial" && settings.OnError != "continue" && settings.OnError != "route" {
				return nil, metadata, fmt.Errorf("loop card %q scope %q: %w", node.Key, fmt.Sprintf("%s:%06d", node.Key, index+1), outcome.err)
			}
			metadata["failed_iterations"] = metadataInt(metadata["failed_iterations"]) + 1
			continue
		}
		results = append(results, outcome.results...)
	}
	metadata["completed_iterations"] = len(items) - metadataInt(metadata["failed_iterations"])
	return map[string]any{"results": results}, metadata, nil
}

func (r *scopedRunner) routeLoopError(node Node, scopeKey string, ready *[]nodeScope) {
	token := Token{NodeKey: node.Key, PortKey: "error", Contract: "error", ScopeKey: scopeKey, Value: ErrorToken{Code: "execution_failed", NodeKey: node.Key, ScopeKey: scopeKey}}
	for _, edge := range r.edges[node.Key] {
		if edge.FromPort == "error" {
			r.route(token, edge, scopeKey, ready)
		}
	}
}

func metadataInt(value any) int {
	result, _ := value.(int)
	return result
}

type loopSettings struct {
	MaxIterations int
	Concurrency   int
	OnError       string
}

func loopSettingsFor(node Node) (loopSettings, error) {
	maxIterations, ok := integer(node.Config["max_iterations"])
	if !ok || maxIterations < 1 {
		return loopSettings{}, fmt.Errorf("loop card %q requires positive config.max_iterations", node.Key)
	}
	concurrency := 1
	if value, exists := node.Config["concurrency"]; exists {
		var valid bool
		concurrency, valid = integer(value)
		if !valid || concurrency < 1 {
			return loopSettings{}, fmt.Errorf("loop card %q requires positive config.concurrency", node.Key)
		}
	}
	if concurrency > 4 {
		return loopSettings{}, fmt.Errorf("loop card %q config.concurrency must not exceed 4", node.Key)
	}
	onError := "fail"
	if value, exists := node.Config["on_error"]; exists {
		var ok bool
		onError, ok = value.(string)
		if !ok || !validErrorPolicy(onError) {
			return loopSettings{}, fmt.Errorf("loop card %q config.on_error must be \"fail\", \"continue\", \"partial\", or \"route\"", node.Key)
		}
	}
	return loopSettings{MaxIterations: maxIterations, Concurrency: concurrency, OnError: onError}, nil
}

func validErrorPolicy(value string) bool {
	return value == "fail" || value == "continue" || value == "partial" || value == "route"
}

func errorPolicyFor(node Node) (string, error) {
	policy := errorPolicyForNode(node)
	if !validErrorPolicy(policy) {
		return "", fmt.Errorf("node %q config.on_error must be \"fail\", \"continue\", \"partial\", or \"route\"", node.Key)
	}
	return policy, nil
}

func errorPolicyForNode(node Node) string {
	if policy, ok := node.Config["on_error"].(string); ok && policy != "" {
		return policy
	}
	return "fail"
}

type errorControlSettings struct {
	Mode     string
	Fallback any
}

func errorControlSettingsFor(node Node) (errorControlSettings, error) {
	mode := "continue"
	if value, exists := node.Config["on_error"]; exists {
		configured, ok := value.(string)
		if !ok || (configured != "fail" && configured != "continue" && configured != "fallback") {
			return errorControlSettings{}, fmt.Errorf("error_control card %q config.on_error must be \"fail\", \"continue\", or \"fallback\"", node.Key)
		}
		mode = configured
	}
	settings := errorControlSettings{Mode: mode}
	if mode == "fallback" {
		value, exists := node.Config["fallback_result"]
		if !exists {
			return errorControlSettings{}, fmt.Errorf("error_control card %q requires config.fallback_result when config.on_error is \"fallback\"", node.Key)
		}
		settings.Fallback = value
	}
	return settings, nil
}

func loopItems(values []any) ([]any, error) {
	items := make([]any, 0)
	for _, value := range values {
		if value == nil {
			return nil, fmt.Errorf("items input must be a list or groups")
		}
		list := reflect.ValueOf(value)
		if list.Kind() != reflect.Slice && list.Kind() != reflect.Array {
			return nil, fmt.Errorf("items input must be a list or groups")
		}
		for index := 0; index < list.Len(); index++ {
			items = append(items, list.Index(index).Interface())
		}
	}
	return items, nil
}

func inputsReady(node Node, inbox map[string][]Token, incoming map[string]int, catalog Catalog) bool {
	if node.Type == "cache" && node.Config["mode"] == "write" && len(inbox["value"]) == 0 {
		return false
	}
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

func execute(ctx context.Context, node Node, inputs map[string][]any, input map[string]any, adapters Adapters, scopeKey string) (map[string]any, error) {
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
		if len(input) > 0 {
			return map[string]any{"event": input}, nil
		}
		if value, ok := node.Config["event"]; ok {
			return map[string]any{"event": value}, nil
		}
		return map[string]any{"event": input}, nil
	case "transform", "log":
		return map[string]any{"output": first()}, nil
	case "variable":
		return map[string]any{"value": first()}, nil
	case "cache":
		return executeCache(ctx, node, inputs, adapters)
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
		prompt, err := renderTemplatePrompt(template, inputs["context"])
		if err != nil {
			return nil, fmt.Errorf("template card %q context: %w", node.Key, err)
		}
		return map[string]any{"prompt": prompt}, nil
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
		return map[string]any{"output": inputs["inputs"]}, nil
	case "loop":
		return nil, fmt.Errorf("loop card %q must be executed by the scoped runner", node.Key)
	case "error_control":
		settings, err := errorControlSettingsFor(node)
		if err != nil {
			return nil, err
		}
		if _, ok := firstForPort(inputs, "error").(ErrorToken); !ok {
			return nil, fmt.Errorf("error_control card %q requires a typed error input", node.Key)
		}
		switch settings.Mode {
		case "continue":
			return map[string]any{}, nil
		case "fallback":
			return map[string]any{"recovered": settings.Fallback}, nil
		default:
			return nil, fmt.Errorf("error_control card %q terminated routed error", node.Key)
		}
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
		return publishReview(ctx, node, inputs, adapters, scopeKey)
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
		request, err := pullRequestRequestFromEvent(firstForPort(inputs, "event"))
		if err != nil {
			return nil, fmt.Errorf("fetch card %q requires a valid pull request event input", node.Key)
		}
		pullRequest, err := adapters.Gitea.ReadPullRequest(ctx, item, secret, request)
		if err != nil {
			return nil, fmt.Errorf("fetch card %q: %w", node.Key, err)
		}
		pullRequest.Target = request
		return map[string]any{"pull_request": pullRequest, "files": pullRequest.Files}, nil
	case "model":
		prompt, ok := firstForPort(inputs, "prompt").(string)
		if !ok || prompt == "" {
			return nil, fmt.Errorf("model card %q requires a prompt input", node.Key)
		}
		response, err := modelResponse(ctx, node, prompt, adapters)
		if err != nil {
			return nil, err
		}
		return map[string]any{"response": response.Content}, nil
	default:
		return nil, fmt.Errorf("card type %q has no configured executor", node.Type)
	}
}

// renderTemplatePrompt makes the typed context available to the model. Existing
// templates that do not use a placeholder remain valid: their context is
// appended as a JSON block. Authors may place {{context}} where it reads best.
func renderTemplatePrompt(template string, contextValues []any) (string, error) {
	if len(contextValues) == 0 {
		return template, nil
	}
	contextValue := any(contextValues)
	if len(contextValues) == 1 {
		contextValue = contextValues[0]
	}
	payload, err := json.Marshal(contextValue)
	if err != nil {
		return "", fmt.Errorf("serialize input context: %w", err)
	}
	contextBlock := "Contexto real para a tarefa (use estes dados na resposta):\n" + string(payload)
	if strings.Contains(template, "{{context}}") {
		template = strings.ReplaceAll(template, "{{context}}", contextBlock)
	} else {
		template += "\n\n" + contextBlock
	}
	if containsFileGroup(contextValues) {
		template += "\n\nContrato obrigatório para a review: responda somente JSON puro, sem markdown. Cada achado deve seguir [{\"path\":\"arquivo.ext\",\"line\":12,\"comment\":\"explicação objetiva\",\"severity\":\"low|medium|high|critical\"}]. O campo line é a linha do arquivo NOVO indicada no cabeçalho @@ ... +LINHA do diff; escolha somente uma linha visível no hunk do arquivo. Não invente linha, caminho ou achado para arquivo sem hunk."
	}
	return template, nil
}

func containsFileGroup(values []any) bool {
	for _, value := range values {
		if _, ok := value.(FileGroup); ok {
			return true
		}
	}
	return false
}

func executeCache(ctx context.Context, node Node, inputs map[string][]any, adapters Adapters) (map[string]any, error) {
	settings, err := cacheSettingsFor(node)
	if err != nil {
		return nil, err
	}
	if adapters.Cache == nil {
		return nil, fmt.Errorf("cache card %q requires an injected cache adapter", node.Key)
	}
	switch settings.Mode {
	case "read":
		payload, found, err := adapters.Cache.Get(ctx, settings.Key)
		if err != nil {
			return nil, fmt.Errorf("cache card %q: %w", node.Key, err)
		}
		if !found {
			return map[string]any{"value": nil}, nil
		}
		var value any
		if err := json.Unmarshal(payload, &value); err != nil {
			return nil, fmt.Errorf("cache card %q returned invalid JSON", node.Key)
		}
		return map[string]any{"value": value}, nil
	case "write":
		value := firstForPort(inputs, "value")
		payload, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("cache card %q could not encode value: %w", node.Key, err)
		}
		if err := adapters.Cache.Set(ctx, settings.Key, payload, time.Duration(settings.TTLSeconds)*time.Second); err != nil {
			return nil, fmt.Errorf("cache card %q: %w", node.Key, err)
		}
		return map[string]any{"value": value}, nil
	case "delete":
		if err := adapters.Cache.Delete(ctx, settings.Key); err != nil {
			return nil, fmt.Errorf("cache card %q: %w", node.Key, err)
		}
		return map[string]any{}, nil
	default:
		return nil, fmt.Errorf("cache card %q has invalid mode", node.Key)
	}
}

func modelResponse(ctx context.Context, node Node, prompt string, adapters Adapters) (integration.ChatResult, error) {
	item, err := configuredModelIntegration(ctx, node, adapters)
	if err != nil {
		return integration.ChatResult{}, err
	}
	var client integration.ChatClient
	switch item.Type {
	case integration.TypeOpenAI:
		client = adapters.OpenAI
	case integration.TypeOllama:
		client = adapters.Ollama
	default:
		return integration.ChatResult{}, fmt.Errorf("model card %q integration %q must be type %q or %q", node.Key, item.Key, integration.TypeOpenAI, integration.TypeOllama)
	}
	if client == nil {
		return integration.ChatResult{}, fmt.Errorf("model card %q requires an injected %s adapter", node.Key, item.Type)
	}
	secret, err := resolveSecret(item, adapters, "model", node.Key)
	if err != nil {
		return integration.ChatResult{}, err
	}
	settings, err := modelSettingsFor(node)
	if err != nil {
		return integration.ChatResult{}, err
	}
	config, err := item.ConfigValues()
	if err != nil {
		return integration.ChatResult{}, err
	}
	config["max_tokens"] = strconv.Itoa(settings.MaxTokens)
	item.Config, err = json.Marshal(config)
	if err != nil {
		return integration.ChatResult{}, err
	}
	response, err := client.Chat(ctx, item, secret, prompt)
	if err != nil {
		return integration.ChatResult{}, fmt.Errorf("model card %q: %w", node.Key, err)
	}
	adapters.Telemetry.Record(response)
	return response, nil
}

func configuredModelIntegration(ctx context.Context, node Node, adapters Adapters) (integration.Integration, error) {
	profileKey, _ := node.Config["model_profile"].(string)
	if profileKey == "" {
		return configuredIntegration(ctx, node, adapters, "model")
	}
	if adapters.ModelProfiles == nil {
		return integration.Integration{}, fmt.Errorf("model card %q requires an injected model profile lookup", node.Key)
	}
	profile, err := adapters.ModelProfiles.ModelProfile(ctx, profileKey)
	if err != nil {
		return integration.Integration{}, fmt.Errorf("model card %q profile %q is not configured: %w", node.Key, profileKey, err)
	}
	if profile.Status != integration.StatusActive {
		return integration.Integration{}, fmt.Errorf("model card %q profile %q is not active", node.Key, profileKey)
	}
	item, err := configuredIntegration(ctx, Node{Key: node.Key, Config: map[string]any{"integration": profile.IntegrationKey}}, adapters, "model")
	if err != nil {
		return integration.Integration{}, err
	}
	if item.Type != integration.TypeOpenAI && item.Type != integration.TypeOllama {
		return integration.Integration{}, fmt.Errorf("model card %q profile %q requires an LLM connection", node.Key, profileKey)
	}
	config, err := item.ConfigValues()
	if err != nil {
		return integration.Integration{}, err
	}
	config["model"] = profile.Model
	payload, err := json.Marshal(config)
	if err != nil {
		return integration.Integration{}, err
	}
	item.Config = payload
	return item, nil
}

func publishReview(ctx context.Context, node Node, inputs map[string][]any, adapters Adapters, scopeKey string) (map[string]any, error) {
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
	request, err := pullRequestRequestFromValue(firstForPort(inputs, "pull_request"))
	if err != nil {
		return nil, fmt.Errorf("publish card %q requires a valid pull_request input", node.Key)
	}
	key := publicationKey(adapters.Execution, node.Key, scopeKey)
	attempt, shouldPublish, err := adapters.Publications.BeginPublication(ctx, PublicationAttempt{IdempotencyKey: key, ExecutionID: adapters.Execution.ID, VersionID: adapters.Execution.VersionID, NodeKey: node.Key})
	if err != nil {
		return nil, fmt.Errorf("publish card %q could not persist idempotency state: %w", node.Key, err)
	}
	if !shouldPublish {
		return map[string]any{"receipt": attempt.Receipt}, nil
	}
	secret, err := resolveSecret(item, adapters, "publish", node.Key)
	if err != nil {
		var providerErr integration.PublicationError
		if errors.As(err, &providerErr) && providerErr.Uncertain {
			if ledger, ok := adapters.Publications.(interface {
				UncertainPublication(context.Context, string, error) error
			}); ok {
				_ = ledger.UncertainPublication(ctx, key, err)
			}
		} else {
			_ = adapters.Publications.RetryPublication(ctx, key, err)
		}
		return nil, err
	}
	event := publishEvent(formatted, node.Config)
	receipt, err := adapters.GiteaWriter.PublishReview(ctx, item, secret, integration.GiteaReviewRequest{Owner: request.Owner, Repo: request.Repo, Number: request.Number, Body: formattedReviewBody(formatted, event, adapters.Telemetry.Snapshot()), Event: event, Comments: giteaReviewComments(formatted.Observations), IdempotencyKey: key})
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

func publicationKey(execution ExecutionContext, nodeKey, scopeKey string) string {
	if scopeKey == "" || scopeKey == rootScope {
		return fmt.Sprintf("forgereview:publication:%d:%d:%s", execution.ID, execution.VersionID, nodeKey)
	}
	return fmt.Sprintf("forgereview:publication:%d:%d:%s:%s", execution.ID, execution.VersionID, nodeKey, scopeKey)
}

func formattedReviewBody(review FormattedReview, event string, telemetry TelemetrySnapshot) string {
	status := "comentado"
	if event == "REQUEST_CHANGES" {
		status = "alterações solicitadas"
	} else if review.Summary.Total == 0 {
		status = "sem achados"
	}
	lines := []string{"> status: " + status}
	if telemetry.ElapsedMS >= 0 {
		lines = append(lines, fmt.Sprintf("> tempo decorrido: %.3fs", float64(telemetry.ElapsedMS)/1000))
	}
	if len(telemetry.Models) > 0 {
		lines = append(lines, "> modelo: "+strings.Join(telemetry.Models, ", "))
	}
	if telemetry.Total > 0 {
		if telemetry.Prompt > 0 || telemetry.Completion > 0 {
			lines = append(lines, fmt.Sprintf("> tokens: %d (prompt: %d, completion: %d)", telemetry.Total, telemetry.Prompt, telemetry.Completion))
		} else {
			lines = append(lines, fmt.Sprintf("> tokens: %d", telemetry.Total))
		}
	}
	lines = append(lines, "")
	switch review.Summary.Total {
	case 0:
		lines = append(lines, "Nenhum problema relevante foi encontrado.")
	case 1:
		lines = append(lines, "Foi identificado 1 achado relevante; o comentário inline indica o ponto revisado.")
	default:
		lines = append(lines, fmt.Sprintf("Foram identificados %d achados relevantes; os comentários inline indicam os pontos revisados.", review.Summary.Total))
	}
	lines = append(lines, "", "Review automatizada concluída.")
	return strings.Join(lines, "\n")
}

func publishEvent(review FormattedReview, config map[string]any) string {
	if review.Summary.High > 0 || review.Summary.Critical > 0 {
		allowed, _ := configuredBool(config, "allow_autonomous_rejection", false)
		if allowed {
			return "REQUEST_CHANGES"
		}
		return "COMMENT"
	}
	if review.Summary.Medium > 0 {
		if event, ok := config["medium_severity_event"].(string); ok && event == "REQUEST_CHANGES" {
			return event
		}
	}
	return "COMMENT"
}

func validatePublishPolicy(node Node) error {
	if value, exists := node.Config["medium_severity_event"]; exists {
		event, ok := value.(string)
		if !ok || (event != "COMMENT" && event != "REQUEST_CHANGES") {
			return fmt.Errorf("publish card %q config.medium_severity_event must be \"COMMENT\" or \"REQUEST_CHANGES\"", node.Key)
		}
	}
	if _, err := configuredBool(node.Config, "allow_autonomous_rejection", false); err != nil {
		return fmt.Errorf("publish card %q config.allow_autonomous_rejection must be a boolean", node.Key)
	}
	return nil
}

func giteaReviewComments(observations []ReviewObservation) []integration.GiteaReviewComment {
	comments := make([]integration.GiteaReviewComment, 0, len(observations))
	for _, observation := range observations {
		comments = append(comments, integration.GiteaReviewComment{Path: observation.Path, Body: observation.Body, NewPosition: observation.NewPosition})
	}
	return comments
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
	if adapters.Secrets == nil {
		return "", fmt.Errorf("%s card %q requires an injected encrypted secret manager", card, nodeKey)
	}
	secret, err := adapters.Secrets.Resolve(item)
	if err != nil {
		return "", fmt.Errorf("%s card %q could not resolve integration %q secret: %w", card, nodeKey, item.Key, err)
	}
	return secret, nil
}

func pullRequestRequestFromValue(value any) (integration.PullRequestRequest, error) {
	switch item := value.(type) {
	case integration.PullRequest:
		if item.Target.Owner != "" && item.Target.Repo != "" && item.Target.Number > 0 {
			return item.Target, nil
		}
	case map[string]any:
		if nested, ok := item["pull_request"].(map[string]any); ok {
			item = nested
		}
		owner, _ := item["owner"].(string)
		repo, _ := item["repo"].(string)
		number, ok := integer(item["number"])
		if owner != "" && repo != "" && ok && number > 0 {
			return integration.PullRequestRequest{Owner: owner, Repo: repo, Number: number}, nil
		}
	}
	return integration.PullRequestRequest{}, fmt.Errorf("pull request target is unavailable")
}

func pullRequestRequestFromEvent(value any) (integration.PullRequestRequest, error) {
	event, ok := value.(map[string]any)
	if !ok {
		return integration.PullRequestRequest{}, fmt.Errorf("pull request event is unavailable")
	}
	target, ok := event["pull_request"]
	if !ok {
		return integration.PullRequestRequest{}, fmt.Errorf("pull request event is unavailable")
	}
	return pullRequestRequestFromValue(target)
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
