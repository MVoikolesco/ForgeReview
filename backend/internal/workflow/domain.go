package workflow

import "fmt"

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
	ErrorOutput *Port `json:"error_output,omitempty"`
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
		nodes[node.Key] = node
	}
	for _, edge := range definition.Edges {
		from, fromOK := nodes[edge.FromNode]
		to, toOK := nodes[edge.ToNode]
		if edge.Key == "" || !fromOK || !toOK {
			return fmt.Errorf("edge %q references an unknown node", edge.Key)
		}
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

func port(ports []Port, key string) (Port, bool) {
	for _, item := range ports {
		if item.Key == key {
			return item, true
		}
	}
	return Port{}, false
}
