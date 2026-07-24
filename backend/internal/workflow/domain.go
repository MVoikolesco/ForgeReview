package workflow

import (
	"fmt"
	"strings"
)

const (
	VersionStatusDraft     = "draft"
	VersionStatusPublished = "published"
	VersionStatusArchived  = "archived"
)

type Port struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Contract   string `json:"contract"`
	Required   bool   `json:"required"`
	CollectAll bool   `json:"collect_all"`
}

type CardType struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Inputs      []Port `json:"inputs"`
	Outputs     []Port `json:"outputs"`
	// ErrorOutput is an opt-in output: it is usable only when a node selects
	// config.on_error="route", so ordinary cards do not gain a permanent port.
	ErrorOutput       *Port  `json:"error_output,omitempty"`
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

type Node struct {
	Key      string         `json:"key"`
	Type     string         `json:"type"`
	Name     string         `json:"name"`
	Config   map[string]any `json:"config"`
	Position Position       `json:"position"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Edge struct {
	Key      string `json:"key"`
	FromNode string `json:"from_node"`
	FromPort string `json:"from_port"`
	ToNode   string `json:"to_node"`
	ToPort   string `json:"to_port"`
}

type Definition struct {
	Key         string             `json:"key"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Interface   *WorkflowInterface `json:"interface,omitempty"`
	Nodes       []Node             `json:"nodes"`
	Edges       []Edge             `json:"edges"`
}

type WorkflowInterface struct {
	TriggerNodeKey string           `json:"trigger_node_key,omitempty"`
	Inputs         []InterfaceField `json:"inputs"`
	Outputs        []InterfaceField `json:"outputs"`
}

type InterfaceField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Contract string `json:"contract"`
	Required bool   `json:"required,omitempty"`
	NodeKey  string `json:"node_key,omitempty"`
	PortKey  string `json:"port_key,omitempty"`
}

// VersionSummary is the safe metadata returned for a stored workflow version.
// Loading a version still returns its definition only.
type VersionSummary struct {
	ID        int64  `json:"version_id"`
	Version   int    `json:"version"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// DefinitionSummary groups the versions belonging to one workflow key. Name
// and description are taken from the latest version.
type DefinitionSummary struct {
	Key         string           `json:"key"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Versions    []VersionSummary `json:"versions"`
}

// ExecutionSummary is the deliberately limited execution view used by the
// management dashboard. It never includes execution input, node data, errors,
// or workflow configuration.
type ExecutionSummary struct {
	ID         int64                   `json:"execution_id"`
	Status     string                  `json:"status"`
	StartedAt  string                  `json:"started_at"`
	FinishedAt string                  `json:"finished_at,omitempty"`
	Workflow   ExecutionWorkflow       `json:"workflow"`
	Review     *ExecutionReviewContext `json:"review,omitempty"`
}

// ExecutionStatus is the safe per-execution view used by Studio and SSE. Raw
// inputs, token values, provider responses, and node metadata are deliberately
// retained outside this contract.
type ExecutionStatus struct {
	ID         int64                `json:"execution_id"`
	Status     string               `json:"status"`
	StartedAt  string               `json:"started_at"`
	FinishedAt string               `json:"finished_at,omitempty"`
	Runs       []ExecutionNodeState `json:"runs"`
	Coverage   *CoverageSummary     `json:"coverage,omitempty"`
}

type ExecutionNodeState struct {
	NodeKey  string `json:"node_key"`
	ScopeKey string `json:"scope_key,omitempty"`
	Status   string `json:"status"`
}

// ExecutionEvent is persisted before it is sent through SSE, making replay
// possible after a browser reconnect or process restart.
type ExecutionEvent struct {
	ID          int64               `json:"id"`
	ExecutionID int64               `json:"execution_id"`
	Kind        string              `json:"kind"`
	Status      string              `json:"status"`
	Node        *ExecutionNodeState `json:"node,omitempty"`
	CreatedAt   string              `json:"created_at"`
}

// ExecutionCardLog is the operator diagnostic view for one card/scope attempt.
// Its payloads are retained for seven days and recursively redact credentials.
type ExecutionCardLog struct {
	ID          int64          `json:"id"`
	ExecutionID int64          `json:"execution_id"`
	NodeKey     string         `json:"node_key"`
	ScopeKey    string         `json:"scope_key,omitempty"`
	Status      string         `json:"status"`
	StartedAt   string         `json:"started_at,omitempty"`
	FinishedAt  string         `json:"finished_at,omitempty"`
	DurationMS  int64          `json:"duration_ms,omitempty"`
	Error       string         `json:"error,omitempty"`
	Facts       map[string]any `json:"facts,omitempty"`
	Inputs      any            `json:"inputs,omitempty"`
	Outputs     any            `json:"outputs,omitempty"`
}

type ExecutionWorkflow struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// ExecutionReviewContext contains only PR coordinates from typed execution
// input. It intentionally has no integration reference.
type ExecutionReviewContext struct {
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	PullRequest int    `json:"pull_request"`
}

type WebhookRegistration struct {
	Key              string `json:"key"`
	Name             string `json:"name"`
	WorkflowKey      string `json:"workflow_key"`
	TriggerNodeKey   string `json:"trigger_node_key"`
	SecretCiphertext string `json:"-"`
	Active           bool   `json:"active"`
	SecretConfigured bool   `json:"secret_configured"`
}

func Validate(definition Definition, catalog Catalog) error {
	if definition.Key == "" || definition.Name == "" {
		return fmt.Errorf("workflow key and name are required")
	}
	if err := validateWorkflowInterface(definition, catalog); err != nil {
		return err
	}
	nodes := map[string]Node{}
	for _, node := range definition.Nodes {
		if node.Key == "" || node.Name == "" {
			return fmt.Errorf("workflow nodes require key and name")
		}
		if _, exists := nodes[node.Key]; exists {
			return fmt.Errorf("duplicate workflow node %q", node.Key)
		}
		if _, ok := catalog.Get(node.Type); !ok {
			return fmt.Errorf("node %q uses unknown card type %q", node.Key, node.Type)
		}
		cardType, _ := catalog.Get(node.Type)
		if !cardType.Available {
			return fmt.Errorf("node %q uses unavailable card type %q", node.Key, node.Type)
		}
		if err := validateSafeConfig(node.Config); err != nil {
			return fmt.Errorf("node %q contains unsafe configuration: %w", node.Key, err)
		}
		if node.Type == "error_control" {
			if _, err := errorControlSettingsFor(node); err != nil {
				return err
			}
		} else if _, err := errorPolicyFor(node); err != nil {
			return err
		}
		if node.Type == "loop" {
			if _, err := loopSettingsFor(node); err != nil {
				return err
			}
		}
		if node.Type == "cache" {
			if _, err := cacheSettingsFor(node); err != nil {
				return err
			}
		}
		if node.Type == "trigger" {
			if _, err := TriggerMode(node); err != nil {
				return err
			}
		}
		if node.Type == "template" {
			if _, _, err := ReviewContractReference(node.Config); err != nil {
				return fmt.Errorf("template card %q config.review_contract: %w", node.Key, err)
			}
		}
		if node.Type == "model" || node.Type == "candidate_validator" {
			if _, err := modelSettingsFor(node); err != nil {
				return err
			}
			if _, profile := node.Config["model_profile"].(string); !profile {
				if _, legacy := node.Config["integration"].(string); !legacy {
					return fmt.Errorf("%s card %q requires config.model_profile", node.Type, node.Key)
				}
			}
			if _, _, err := reviewChecklistFromConfig(node.Config); err != nil {
				return fmt.Errorf("%s card %q config.review_checklist: %w", node.Type, node.Key, err)
			}
		}
		if node.Type == "validate" {
			if schema, exists := responseSchemaFromConfig(node.Config); exists {
				if _, err := validateResponseSchema(schema); err != nil {
					return fmt.Errorf("validate card %q config.response_schema: %w", node.Key, err)
				}
			}
		}
		if node.Type == "transform" {
			if err := validateTransformConfig(node); err != nil {
				return err
			}
		}
		if node.Type == "variable" {
			if err := validateVariableConfig(node); err != nil {
				return err
			}
		}
		if node.Type == "condition" {
			if err := validateConditionConfig(node, cardType); err != nil {
				return err
			}
		}
		if node.Type == "merge" {
			if _, err := mergeSettingsFor(node); err != nil {
				return err
			}
		}
		if node.Type == "workflow" {
			versionID, ok := integer(node.Config["workflow_version_id"])
			if !ok || versionID < 1 {
				return fmt.Errorf("workflow card %q requires positive config.workflow_version_id", node.Key)
			}
		}
		if node.Type == "fetch" {
			if err := validateNoFixedPullRequestConfig(node); err != nil {
				return err
			}
			if _, exists := node.Config["medium_severity_event"]; exists {
				return fmt.Errorf("fetch card %q does not support config.medium_severity_event; configure it on publish", node.Key)
			}
			if _, exists := node.Config["allow_autonomous_rejection"]; exists {
				return fmt.Errorf("fetch card %q does not support config.allow_autonomous_rejection; configure it on publish", node.Key)
			}
		}
		if node.Type == "publish" {
			if err := validateNoFixedPullRequestConfig(node); err != nil {
				return err
			}
			if err := validatePublishPolicy(node); err != nil {
				return err
			}
		}
		nodes[node.Key] = node
	}
	edges := map[string]struct{}{}
	for _, edge := range definition.Edges {
		from, fromOK := nodes[edge.FromNode]
		to, toOK := nodes[edge.ToNode]
		if edge.Key == "" || !fromOK || !toOK {
			return fmt.Errorf("edge %q references an unknown node", edge.Key)
		}
		if _, exists := edges[edge.Key]; exists {
			return fmt.Errorf("duplicate workflow edge %q", edge.Key)
		}
		edges[edge.Key] = struct{}{}
		fromType, _ := catalog.Get(from.Type)
		toType, _ := catalog.Get(to.Type)
		output, outputOK := port(fromType.Outputs, edge.FromPort)
		if !outputOK && edge.FromPort == "error" && fromType.ErrorOutput != nil && errorPolicyForNode(from) == "route" {
			output, outputOK = *fromType.ErrorOutput, true
		}
		input, inputOK := port(toType.Inputs, edge.ToPort)
		if !outputOK || !inputOK {
			return fmt.Errorf("edge %q references an unknown port", edge.Key)
		}
		if output.Contract != "any" && input.Contract != "any" && output.Contract != input.Contract {
			return fmt.Errorf("edge %q connects incompatible contracts", edge.Key)
		}
	}
	for _, node := range definition.Nodes {
		if node.Type != "error_control" && errorPolicyForNode(node) == "route" {
			hasRoute := false
			for _, edge := range definition.Edges {
				if edge.FromNode == node.Key && edge.FromPort == "error" {
					hasRoute = true
					break
				}
			}
			if !hasRoute {
				return fmt.Errorf("node %q config.on_error \"route\" requires an explicit error edge", node.Key)
			}
		}
		if node.Type == "merge" {
			settings, _ := mergeSettingsFor(node)
			incoming := 0
			for _, edge := range definition.Edges {
				if edge.ToNode == node.Key && edge.ToPort == "inputs" {
					incoming++
				}
			}
			if settings.Mode == "quorum" && settings.Quorum > incoming {
				return fmt.Errorf("merge card %q config.quorum exceeds its %d incoming edges", node.Key, incoming)
			}
		}
	}
	return nil
}

func validateWorkflowInterface(definition Definition, catalog Catalog) error {
	if definition.Interface == nil {
		return nil
	}
	contracts := map[string]bool{"any": true, "string": true, "number": true, "boolean": true, "object": true, "list": true}
	for _, card := range catalog.All() {
		for _, item := range append(append([]Port{}, card.Inputs...), card.Outputs...) {
			contracts[item.Contract] = true
		}
	}
	if definition.Interface.TriggerNodeKey != "" {
		trigger, ok := nodeFor(definition.Nodes, definition.Interface.TriggerNodeKey)
		if !ok || trigger.Type != "trigger" {
			return fmt.Errorf("workflow interface trigger_node_key must reference a trigger card")
		}
	}
	for _, fields := range [][]InterfaceField{definition.Interface.Inputs, definition.Interface.Outputs} {
		seen := map[string]bool{}
		for _, field := range fields {
			if strings.TrimSpace(field.Key) == "" || strings.TrimSpace(field.Contract) == "" {
				return fmt.Errorf("workflow interface fields require key and contract")
			}
			if !contracts[field.Contract] {
				return fmt.Errorf("workflow interface field %q uses unknown contract %q", field.Key, field.Contract)
			}
			if seen[field.Key] {
				return fmt.Errorf("duplicate workflow interface field %q", field.Key)
			}
			seen[field.Key] = true
		}
	}
	for _, field := range definition.Interface.Outputs {
		node, ok := nodeFor(definition.Nodes, field.NodeKey)
		if !ok {
			return fmt.Errorf("workflow interface output %q references unknown node", field.Key)
		}
		card, ok := catalog.Get(node.Type)
		if !ok {
			return fmt.Errorf("workflow interface output %q references unknown card", field.Key)
		}
		output, ok := port(card.Outputs, field.PortKey)
		if !ok {
			return fmt.Errorf("workflow interface output %q references unknown port", field.Key)
		}
		if output.Contract != "any" && field.Contract != "any" && output.Contract != field.Contract {
			return fmt.Errorf("workflow interface output %q has incompatible contract", field.Key)
		}
	}
	return nil
}

// ValidateNoFixedPullRequestConfig is also used at the storage boundary so a
// caller cannot bypass graph validation and persist obsolete PR coordinates.
func ValidateNoFixedPullRequestConfig(definition Definition) error {
	for _, node := range definition.Nodes {
		if node.Type == "fetch" || node.Type == "publish" {
			if err := validateNoFixedPullRequestConfig(node); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateNoFixedPullRequestConfig(node Node) error {
	for _, key := range []string{"owner", "repo", "pull_request"} {
		if _, exists := node.Config[key]; exists {
			return fmt.Errorf("%s card %q does not support fixed PR coordinate config.%s", node.Type, node.Key, key)
		}
	}
	return nil
}

// TriggerMode keeps definitions created before trigger modes compatible.
func TriggerMode(node Node) (string, error) {
	mode, _ := node.Config["mode"].(string)
	if mode == "" {
		return "manual", nil
	}
	if mode != "manual" && mode != "api" && mode != "webhook" {
		return "", fmt.Errorf("trigger card %q config.mode must be \"manual\", \"api\", or \"webhook\"", node.Key)
	}
	return mode, nil
}

// validateSafeConfig prevents credentials and encrypted secret blobs from
// entering immutable workflow history through a draft save.
func validateSafeConfig(config map[string]any) error {
	for key, value := range config {
		lower := strings.ToLower(key)
		if lower == "secret" || lower == "ciphertext" || lower == "password" || lower == "token" || lower == "api_key" || strings.HasSuffix(lower, "_secret") || strings.HasSuffix(lower, "_ciphertext") || strings.HasSuffix(lower, "_password") || strings.HasSuffix(lower, "_token") || strings.HasSuffix(lower, "_api_key") {
			return fmt.Errorf("configuration field %q is not allowed", key)
		}
		if nested, ok := value.(map[string]any); ok {
			if err := validateSafeConfig(nested); err != nil {
				return err
			}
		}
		if values, ok := value.([]any); ok {
			for _, item := range values {
				if nested, ok := item.(map[string]any); ok {
					if err := validateSafeConfig(nested); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func port(ports []Port, key string) (Port, bool) {
	for _, item := range ports {
		if item.Key == key {
			return item, true
		}
	}
	return Port{}, false
}
