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
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Nodes       []Node `json:"nodes"`
	Edges       []Edge `json:"edges"`
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

type ExecutionWorkflow struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// ExecutionReviewContext contains only the PR coordinates configured on the
// stored fetch/publish cards. It intentionally has no integration reference.
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
		if node.Type == "model" {
			if _, err := modelSettingsFor(node); err != nil {
				return err
			}
			if _, profile := node.Config["model_profile"].(string); !profile {
				if _, legacy := node.Config["integration"].(string); !legacy {
					return fmt.Errorf("model card %q requires config.model_profile", node.Key)
				}
			}
		}
		if node.Type == "fetch" {
			if _, exists := node.Config["medium_severity_event"]; exists {
				return fmt.Errorf("fetch card %q does not support config.medium_severity_event; configure it on publish", node.Key)
			}
			if _, exists := node.Config["allow_autonomous_rejection"]; exists {
				return fmt.Errorf("fetch card %q does not support config.allow_autonomous_rejection; configure it on publish", node.Key)
			}
		}
		if node.Type == "publish" {
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
