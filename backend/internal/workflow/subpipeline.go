package workflow

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	subpipelineEntryPrefix = "entry:"
	subpipelineExitPrefix  = "exit:"
)

var subpipelineInstanceKey = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type subpipelineInstance struct {
	Key                   string
	Name                  string
	Enabled               bool
	Template              string
	ReviewContractKey     string
	ReviewContractVersion int
	ModelProfile          string
	ValidatorModelProfile string
	MinimumSeverity       string
}

func validateSubpipelineNode(node Node) error {
	if node.ParentKey != "" {
		return fmt.Errorf("subpipeline %q cannot be nested", node.Key)
	}
	if node.Size == nil || node.Size.Width < 420 || node.Size.Height < 180 {
		return fmt.Errorf("subpipeline %q requires size of at least 420x180", node.Key)
	}
	if node.Size.Width > 5000 || node.Size.Height > 5000 {
		return fmt.Errorf("subpipeline %q size must not exceed 5000x5000", node.Key)
	}
	inputs, err := configuredSubpipelinePorts(node, "input_ports")
	if err != nil {
		return err
	}
	outputs, err := configuredSubpipelinePorts(node, "output_ports")
	if err != nil {
		return err
	}
	if len(inputs) == 0 || len(outputs) == 0 {
		return fmt.Errorf("subpipeline %q requires at least one input and one output", node.Key)
	}
	if _, configured := node.Config["instances"]; configured {
		if _, err := configuredSubpipelineInstances(node); err != nil {
			return err
		}
	}
	return nil
}

func configuredSubpipelineInstances(node Node) ([]subpipelineInstance, error) {
	value, ok := node.Config["instances"]
	if !ok {
		return nil, nil
	}
	raw, ok := value.([]any)
	if !ok {
		if typed, typedOK := value.([]map[string]any); typedOK {
			raw = make([]any, len(typed))
			for index := range typed {
				raw[index] = typed[index]
			}
		} else {
			return nil, fmt.Errorf("subpipeline %q config.instances must be a list", node.Key)
		}
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("subpipeline %q requires at least one configured instance", node.Key)
	}
	instances := make([]subpipelineInstance, 0, len(raw))
	seen := map[string]bool{}
	enabled := 0
	for _, item := range raw {
		fields, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("subpipeline %q config.instances contains an invalid instance", node.Key)
		}
		instance := subpipelineInstance{
			Key:                   strings.TrimSpace(stringConfig(fields, "key")),
			Name:                  strings.TrimSpace(stringConfig(fields, "name")),
			Enabled:               boolConfig(fields, "enabled", true),
			Template:              strings.TrimSpace(stringConfig(fields, "template")),
			ReviewContractKey:     strings.TrimSpace(stringConfig(fields, "review_contract_key")),
			ReviewContractVersion: intConfig(fields, "review_contract_version"),
			ModelProfile:          strings.TrimSpace(stringConfig(fields, "model_profile")),
			ValidatorModelProfile: strings.TrimSpace(stringConfig(fields, "validator_model_profile")),
			MinimumSeverity:       strings.TrimSpace(stringConfig(fields, "minimum_severity")),
		}
		if !subpipelineInstanceKey.MatchString(instance.Key) || instance.Name == "" {
			return nil, fmt.Errorf("subpipeline %q instances require a valid key and name", node.Key)
		}
		if seen[instance.Key] {
			return nil, fmt.Errorf("subpipeline %q contains duplicate instance %q", node.Key, instance.Key)
		}
		if instance.Template == "" || instance.ReviewContractKey == "" || instance.ReviewContractVersion < 1 {
			return nil, fmt.Errorf("subpipeline %q instance %q requires template and pinned review contract", node.Key, instance.Key)
		}
		if instance.MinimumSeverity == "" {
			instance.MinimumSeverity = "medium"
		}
		seen[instance.Key] = true
		if instance.Enabled {
			enabled++
		}
		instances = append(instances, instance)
	}
	if enabled == 0 {
		return nil, fmt.Errorf("subpipeline %q requires at least one enabled instance", node.Key)
	}
	return instances, nil
}

func stringConfig(config map[string]any, key string) string {
	value, _ := config[key].(string)
	return value
}

func boolConfig(config map[string]any, key string, fallback bool) bool {
	value, ok := config[key].(bool)
	if !ok {
		return fallback
	}
	return value
}

func intConfig(config map[string]any, key string) int {
	switch value := config[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func configuredSubpipelinePorts(node Node, configKey string) ([]Port, error) {
	value, ok := node.Config[configKey]
	if !ok {
		return nil, fmt.Errorf("subpipeline %q requires config.%s", node.Key, configKey)
	}
	var raw []any
	switch items := value.(type) {
	case []any:
		raw = items
	case []Port:
		ports := append([]Port(nil), items...)
		return validateConfiguredSubpipelinePorts(node.Key, configKey, ports)
	default:
		return nil, fmt.Errorf("subpipeline %q config.%s must be a list", node.Key, configKey)
	}
	ports := make([]Port, 0, len(raw))
	for _, item := range raw {
		fields, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("subpipeline %q config.%s contains an invalid port", node.Key, configKey)
		}
		key, _ := fields["key"].(string)
		label, _ := fields["label"].(string)
		contract, _ := fields["contract"].(string)
		required, _ := fields["required"].(bool)
		ports = append(ports, Port{Key: key, Label: label, Contract: contract, Required: required})
	}
	return validateConfiguredSubpipelinePorts(node.Key, configKey, ports)
}

func validateConfiguredSubpipelinePorts(nodeKey, configKey string, ports []Port) ([]Port, error) {
	seen := map[string]bool{}
	for index := range ports {
		ports[index].Key = strings.TrimSpace(ports[index].Key)
		ports[index].Label = strings.TrimSpace(ports[index].Label)
		ports[index].Contract = strings.TrimSpace(ports[index].Contract)
		if ports[index].Key == "" || ports[index].Label == "" || ports[index].Contract == "" {
			return nil, fmt.Errorf("subpipeline %q config.%s ports require key, label, and contract", nodeKey, configKey)
		}
		if strings.ContainsAny(ports[index].Key, ": \t\r\n") {
			return nil, fmt.Errorf("subpipeline %q config.%s port key %q is invalid", nodeKey, configKey, ports[index].Key)
		}
		if seen[ports[index].Key] {
			return nil, fmt.Errorf("subpipeline %q config.%s contains duplicate port %q", nodeKey, configKey, ports[index].Key)
		}
		seen[ports[index].Key] = true
	}
	return ports, nil
}

func nodeOutputPort(node Node, key string, catalog Catalog) (Port, bool) {
	if node.Type == "subpipeline" {
		if strings.HasPrefix(key, subpipelineEntryPrefix) {
			return configuredSubpipelinePort(node, "input_ports", strings.TrimPrefix(key, subpipelineEntryPrefix))
		}
		if strings.HasPrefix(key, subpipelineExitPrefix) {
			return configuredSubpipelinePort(node, "output_ports", strings.TrimPrefix(key, subpipelineExitPrefix))
		}
		return Port{}, false
	}
	card, ok := catalog.Get(node.Type)
	if !ok {
		return Port{}, false
	}
	output, outputOK := port(card.Outputs, key)
	if !outputOK && key == "error" && card.ErrorOutput != nil && errorPolicyForNode(node) == "route" {
		return *card.ErrorOutput, true
	}
	return output, outputOK
}

func nodeInputPort(node Node, key string, catalog Catalog) (Port, bool) {
	if node.Type == "subpipeline" {
		if strings.HasPrefix(key, subpipelineEntryPrefix) {
			return configuredSubpipelinePort(node, "input_ports", strings.TrimPrefix(key, subpipelineEntryPrefix))
		}
		if strings.HasPrefix(key, subpipelineExitPrefix) {
			return configuredSubpipelinePort(node, "output_ports", strings.TrimPrefix(key, subpipelineExitPrefix))
		}
		return Port{}, false
	}
	card, ok := catalog.Get(node.Type)
	if !ok {
		return Port{}, false
	}
	return port(card.Inputs, key)
}

func configuredSubpipelinePort(node Node, configKey, key string) (Port, bool) {
	ports, err := configuredSubpipelinePorts(node, configKey)
	if err != nil {
		return Port{}, false
	}
	return port(ports, key)
}

func validateSubpipelineBoundaries(definition Definition) error {
	nodes := make(map[string]Node, len(definition.Nodes))
	for _, node := range definition.Nodes {
		nodes[node.Key] = node
	}
	type boundaryCount struct {
		external int
		internal int
	}
	counts := map[string]*boundaryCount{}
	count := func(group, port string) *boundaryCount {
		key := group + "\x00" + port
		if counts[key] == nil {
			counts[key] = &boundaryCount{}
		}
		return counts[key]
	}
	for _, edge := range definition.Edges {
		from := nodes[edge.FromNode]
		to := nodes[edge.ToNode]
		if from.Type == "subpipeline" && to.Type == "subpipeline" {
			return fmt.Errorf("edge %q cannot connect subpipelines directly", edge.Key)
		}
		switch {
		case to.Type == "subpipeline":
			if strings.HasPrefix(edge.ToPort, subpipelineEntryPrefix) {
				if from.ParentKey != "" {
					return fmt.Errorf("edge %q must enter subpipeline %q from the main pipeline", edge.Key, to.Key)
				}
				count(to.Key, edge.ToPort).external++
			} else if strings.HasPrefix(edge.ToPort, subpipelineExitPrefix) {
				if from.ParentKey != to.Key {
					return fmt.Errorf("edge %q must reach subpipeline exit %q from one of its cards", edge.Key, to.Key)
				}
				count(to.Key, edge.ToPort).internal++
			}
		case from.Type == "subpipeline":
			if strings.HasPrefix(edge.FromPort, subpipelineEntryPrefix) {
				if to.ParentKey != from.Key {
					return fmt.Errorf("edge %q must route subpipeline entry %q to one of its cards", edge.Key, from.Key)
				}
				count(from.Key, edge.FromPort).internal++
			} else if strings.HasPrefix(edge.FromPort, subpipelineExitPrefix) {
				if to.ParentKey != "" {
					return fmt.Errorf("edge %q must leave subpipeline %q for the main pipeline", edge.Key, from.Key)
				}
				count(from.Key, edge.FromPort).external++
			}
		default:
			if from.ParentKey != to.ParentKey {
				return fmt.Errorf("edge %q crosses a subpipeline boundary without using its ports", edge.Key)
			}
		}
	}
	for key, value := range counts {
		if value.external == 0 || value.internal == 0 {
			groupPort := strings.SplitN(key, "\x00", 2)
			return fmt.Errorf("subpipeline %q boundary %q must be connected on both sides", groupPort[0], groupPort[1])
		}
	}
	return nil
}

// executableDefinition removes structural subpipeline containers and splices
// each external boundary edge to its corresponding internal edge. The runtime
// therefore continues to execute the original typed DAG and never treats a
// visual container as an agent or processing step.
func executableDefinition(definition Definition) Definition {
	nodes := make(map[string]Node, len(definition.Nodes))
	groups := map[string]bool{}
	parameterized := map[string][]subpipelineInstance{}
	result := definition
	result.Nodes = make([]Node, 0, len(definition.Nodes))
	for _, node := range definition.Nodes {
		nodes[node.Key] = node
		if node.Type == "subpipeline" {
			groups[node.Key] = true
			if _, configured := node.Config["instances"]; configured {
				instances, err := configuredSubpipelineInstances(node)
				if err == nil {
					parameterized[node.Key] = instances
				}
			}
		}
	}
	for _, node := range definition.Nodes {
		if node.Type == "subpipeline" {
			continue
		}
		if _, expanded := parameterized[node.ParentKey]; expanded {
			continue
		}
		result.Nodes = append(result.Nodes, node)
	}
	result.Edges = make([]Edge, 0, len(definition.Edges))
	for _, edge := range definition.Edges {
		fromParameterizedChild := parameterized[nodes[edge.FromNode].ParentKey] != nil
		toParameterizedChild := parameterized[nodes[edge.ToNode].ParentKey] != nil
		if !groups[edge.FromNode] && !groups[edge.ToNode] && !fromParameterizedChild && !toParameterizedChild {
			result.Edges = append(result.Edges, edge)
		}
	}
	for group := range groups {
		if instances, expanded := parameterized[group]; expanded {
			result = expandParameterizedSubpipeline(result, definition, group, instances, nodes)
			continue
		}
		for _, externalIn := range definition.Edges {
			if externalIn.ToNode != group || !strings.HasPrefix(externalIn.ToPort, subpipelineEntryPrefix) {
				continue
			}
			for _, internalOut := range definition.Edges {
				if internalOut.FromNode != group || internalOut.FromPort != externalIn.ToPort {
					continue
				}
				result.Edges = append(result.Edges, Edge{
					Key:      externalIn.Key + "__" + internalOut.Key,
					FromNode: externalIn.FromNode,
					FromPort: externalIn.FromPort,
					ToNode:   internalOut.ToNode,
					ToPort:   internalOut.ToPort,
				})
			}
		}
		for _, internalIn := range definition.Edges {
			if internalIn.ToNode != group || !strings.HasPrefix(internalIn.ToPort, subpipelineExitPrefix) {
				continue
			}
			for _, externalOut := range definition.Edges {
				if externalOut.FromNode != group || externalOut.FromPort != internalIn.ToPort {
					continue
				}
				result.Edges = append(result.Edges, Edge{
					Key:      internalIn.Key + "__" + externalOut.Key,
					FromNode: internalIn.FromNode,
					FromPort: internalIn.FromPort,
					ToNode:   externalOut.ToNode,
					ToPort:   externalOut.ToPort,
				})
			}
		}
	}
	return result
}

func expandParameterizedSubpipeline(result, definition Definition, group string, instances []subpipelineInstance, nodes map[string]Node) Definition {
	members := map[string]bool{}
	for _, node := range definition.Nodes {
		if node.ParentKey == group {
			members[node.Key] = true
		}
	}
	for _, instance := range instances {
		if !instance.Enabled {
			continue
		}
		cloneKeys := map[string]string{}
		for _, node := range definition.Nodes {
			if !members[node.Key] {
				continue
			}
			clone := node
			clone.Key = node.Key + "::" + instance.Key
			clone.Name = node.Name + " · " + instance.Name
			clone.ParentKey = ""
			clone.Size = nil
			clone.Config = cloneSubpipelineConfig(node.Config)
			applySubpipelineInstance(&clone, instance)
			cloneKeys[node.Key] = clone.Key
			result.Nodes = append(result.Nodes, clone)
		}
		for _, edge := range definition.Edges {
			if members[edge.FromNode] && members[edge.ToNode] {
				clone := edge
				clone.Key = edge.Key + "::" + instance.Key
				clone.FromNode = cloneKeys[edge.FromNode]
				clone.ToNode = cloneKeys[edge.ToNode]
				result.Edges = append(result.Edges, clone)
			}
		}
		for _, externalIn := range definition.Edges {
			if externalIn.ToNode != group || !strings.HasPrefix(externalIn.ToPort, subpipelineEntryPrefix) {
				continue
			}
			for _, internalOut := range definition.Edges {
				if internalOut.FromNode == group && internalOut.FromPort == externalIn.ToPort && members[internalOut.ToNode] {
					result.Edges = append(result.Edges, Edge{
						Key:      externalIn.Key + "__" + internalOut.Key + "::" + instance.Key,
						FromNode: externalIn.FromNode,
						FromPort: externalIn.FromPort,
						ToNode:   cloneKeys[internalOut.ToNode],
						ToPort:   internalOut.ToPort,
					})
				}
			}
		}
		for _, internalIn := range definition.Edges {
			if internalIn.ToNode != group || !strings.HasPrefix(internalIn.ToPort, subpipelineExitPrefix) || !members[internalIn.FromNode] {
				continue
			}
			for _, externalOut := range definition.Edges {
				if externalOut.FromNode == group && externalOut.FromPort == internalIn.ToPort {
					result.Edges = append(result.Edges, Edge{
						Key:      internalIn.Key + "__" + externalOut.Key + "::" + instance.Key,
						FromNode: cloneKeys[internalIn.FromNode],
						FromPort: internalIn.FromPort,
						ToNode:   externalOut.ToNode,
						ToPort:   externalOut.ToPort,
					})
				}
			}
		}
	}
	return result
}

func cloneSubpipelineConfig(config map[string]any) map[string]any {
	clone := make(map[string]any, len(config))
	for key, value := range config {
		clone[key] = value
	}
	return clone
}

func applySubpipelineInstance(node *Node, instance subpipelineInstance) {
	switch node.Type {
	case "template":
		node.Config["template"] = instance.Template
		node.Config["review_contract_key"] = instance.ReviewContractKey
		node.Config["review_contract_version"] = instance.ReviewContractVersion
	case "model":
		if instance.ModelProfile != "" {
			node.Config["model_profile"] = instance.ModelProfile
		}
	case "candidate_validator":
		if instance.ValidatorModelProfile != "" {
			node.Config["model_profile"] = instance.ValidatorModelProfile
		}
	case "response_filter":
		node.Config["minimum_severity"] = instance.MinimumSeverity
	}
}
