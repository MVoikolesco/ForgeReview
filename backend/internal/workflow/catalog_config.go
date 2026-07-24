package workflow

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type mergeSettings struct {
	Mode      string
	Quorum    int
	TimeoutMS int
}

func mergeSettingsFor(node Node) (mergeSettings, error) {
	settings := mergeSettings{Mode: "all"}
	if mode, _ := node.Config["mode"].(string); mode != "" {
		settings.Mode = mode
	}
	if settings.Mode != "all" && settings.Mode != "any" && settings.Mode != "quorum" {
		return mergeSettings{}, fmt.Errorf("merge card %q config.mode must be all, any, or quorum", node.Key)
	}
	if settings.Mode == "quorum" {
		quorum, ok := integer(node.Config["quorum"])
		if !ok || quorum < 1 {
			return mergeSettings{}, fmt.Errorf("merge card %q config.quorum must be positive", node.Key)
		}
		settings.Quorum = quorum
	}
	if value, exists := node.Config["timeout_ms"]; exists {
		timeout, ok := integer(value)
		if !ok || timeout < 1 || timeout > 60000 {
			return mergeSettings{}, fmt.Errorf("merge card %q config.timeout_ms must be between 1 and 60000", node.Key)
		}
		settings.TimeoutMS = timeout
	}
	return settings, nil
}

func validateTransformConfig(node Node) error {
	raw, exists := node.Config["operations"]
	if !exists {
		return nil // legacy pass-through
	}
	operations, ok := raw.([]any)
	if !ok || len(operations) == 0 || len(operations) > 32 {
		return fmt.Errorf("transform card %q config.operations must contain 1 to 32 operations", node.Key)
	}
	for index, rawOperation := range operations {
		operation, ok := rawOperation.(map[string]any)
		if !ok {
			return fmt.Errorf("transform card %q operation %d must be an object", node.Key, index+1)
		}
		kind, _ := operation["op"].(string)
		path, _ := operation["path"].(string)
		switch kind {
		case "select", "remove":
			if !validDataPath(path) {
				return fmt.Errorf("transform card %q operation %d requires a valid path", node.Key, index+1)
			}
		case "set":
			if !validDataPath(path) {
				return fmt.Errorf("transform card %q operation %d requires a valid path", node.Key, index+1)
			}
			if _, exists := operation["value"]; !exists {
				return fmt.Errorf("transform card %q set operation %d requires value", node.Key, index+1)
			}
		case "rename":
			to, _ := operation["to"].(string)
			if !validDataPath(path) || !validDataPath(to) {
				return fmt.Errorf("transform card %q rename operation %d requires path and to", node.Key, index+1)
			}
		case "coalesce":
			paths, ok := stringList(operation["paths"])
			to, _ := operation["to"].(string)
			if !ok || len(paths) == 0 || !validDataPath(to) {
				return fmt.Errorf("transform card %q coalesce operation %d requires paths and to", node.Key, index+1)
			}
		default:
			return fmt.Errorf("transform card %q operation %d has unsupported op %q", node.Key, index+1, kind)
		}
	}
	return nil
}

func applyTransform(node Node, input any) (any, error) {
	if _, exists := node.Config["operations"]; !exists {
		return input, nil
	}
	if err := validateTransformConfig(node); err != nil {
		return nil, err
	}
	value, err := cloneJSONValue(input)
	if err != nil {
		return nil, fmt.Errorf("transform card %q input is not JSON-compatible: %w", node.Key, err)
	}
	for _, raw := range node.Config["operations"].([]any) {
		operation := raw.(map[string]any)
		kind := operation["op"].(string)
		path, _ := operation["path"].(string)
		switch kind {
		case "select":
			selected, found := valueAtPath(value, path)
			if !found {
				return nil, fmt.Errorf("transform card %q path %q was not found", node.Key, path)
			}
			value = selected
		case "set":
			value, err = setAtPath(value, path, operation["value"])
		case "remove":
			value, err = removeAtPath(value, path)
		case "rename":
			selected, found := valueAtPath(value, path)
			if !found {
				return nil, fmt.Errorf("transform card %q path %q was not found", node.Key, path)
			}
			value, err = removeAtPath(value, path)
			if err == nil {
				value, err = setAtPath(value, operation["to"].(string), selected)
			}
		case "coalesce":
			var selected any
			for _, candidate := range operation["paths"].([]any) {
				if foundValue, found := valueAtPath(value, candidate.(string)); found && foundValue != nil {
					selected = foundValue
					break
				}
			}
			value, err = setAtPath(value, operation["to"].(string), selected)
		}
		if err != nil {
			return nil, fmt.Errorf("transform card %q: %w", node.Key, err)
		}
	}
	return value, nil
}

func validateVariableConfig(node Node) error {
	if len(node.Config) == 0 {
		return nil
	}
	action, _ := node.Config["action"].(string)
	if action == "" {
		action = "set"
	}
	namespace, _ := node.Config["namespace"].(string)
	if namespace == "" {
		namespace = "execution"
	}
	name, _ := node.Config["name"].(string)
	if action != "set" && action != "get" {
		return fmt.Errorf("variable card %q config.action must be set or get", node.Key)
	}
	if namespace != "execution" && namespace != "loop" && namespace != "card" {
		return fmt.Errorf("variable card %q config.namespace must be execution, loop, or card", node.Key)
	}
	if !validIdentifier(name) {
		return fmt.Errorf("variable card %q requires config.name", node.Key)
	}
	return nil
}

func validateConditionConfig(node Node, card CardType) error {
	raw, exists := node.Config["branches"]
	if !exists {
		return nil
	}
	branches, ok := raw.([]any)
	if !ok || len(branches) == 0 || len(branches) > 8 {
		return fmt.Errorf("condition card %q config.branches must contain 1 to 8 rules", node.Key)
	}
	seen := map[string]bool{}
	for index, rawBranch := range branches {
		branch, ok := rawBranch.(map[string]any)
		if !ok {
			return fmt.Errorf("condition card %q branch %d must be an object", node.Key, index+1)
		}
		output, _ := branch["port"].(string)
		operator, _ := branch["operator"].(string)
		if _, ok = port(card.Outputs, output); !ok || output == "true" || output == "false" || output == "default" || seen[output] {
			return fmt.Errorf("condition card %q branch %d has invalid or duplicate port", node.Key, index+1)
		}
		if operator != "equals" && operator != "not_equals" && operator != "exists" && operator != "contains" && operator != "gt" && operator != "gte" && operator != "lt" && operator != "lte" {
			return fmt.Errorf("condition card %q branch %d has invalid operator", node.Key, index+1)
		}
		if path, exists := branch["path"]; exists {
			if text, ok := path.(string); !ok || (text != "" && !validDataPath(text)) {
				return fmt.Errorf("condition card %q branch %d has invalid path", node.Key, index+1)
			}
		}
		seen[output] = true
	}
	return nil
}

func conditionOutput(node Node, value any) string {
	branches, _ := node.Config["branches"].([]any)
	for _, raw := range branches {
		branch := raw.(map[string]any)
		candidate := value
		found := true
		if path, _ := branch["path"].(string); path != "" {
			candidate, found = valueAtPath(value, path)
			if !found && branch["operator"] != "exists" {
				continue
			}
		}
		if branch["operator"] == "exists" {
			want, ok := branch["value"].(bool)
			if !ok {
				want = true
			}
			if found == want {
				return branch["port"].(string)
			}
			continue
		}
		if matchesCondition(candidate, branch["operator"].(string), branch["value"]) {
			return branch["port"].(string)
		}
	}
	return "default"
}

func matchesCondition(actual any, operator string, expected any) bool {
	switch operator {
	case "equals":
		return reflect.DeepEqual(actual, expected) || fmt.Sprint(actual) == fmt.Sprint(expected)
	case "not_equals":
		return !matchesCondition(actual, "equals", expected)
	case "contains":
		return strings.Contains(fmt.Sprint(actual), fmt.Sprint(expected))
	}
	left, leftOK := decimal(actual)
	right, rightOK := decimal(expected)
	if !leftOK || !rightOK {
		return false
	}
	switch operator {
	case "gt":
		return left > right
	case "gte":
		return left >= right
	case "lt":
		return left < right
	case "lte":
		return left <= right
	}
	return false
}

func validDataPath(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	for _, part := range strings.Split(path, ".") {
		if !validIdentifier(part) {
			return false
		}
	}
	return true
}

func validIdentifier(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for index, character := range value {
		if !(character == '_' || character == '-' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || index > 0 && character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}

func stringList(value any) ([]string, bool) {
	raw, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok || !validDataPath(text) {
			return nil, false
		}
		result = append(result, text)
	}
	return result, true
}

func cloneJSONValue(value any) (any, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result any
	err = json.Unmarshal(payload, &result)
	return result, err
}

func valueAtPath(value any, path string) (any, bool) {
	current := value
	for _, part := range strings.Split(path, ".") {
		switch typed := current.(type) {
		case map[string]any:
			var found bool
			current, found = typed[part]
			if !found {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func setAtPath(value any, path string, replacement any) (any, error) {
	root, ok := value.(map[string]any)
	if !ok {
		root = map[string]any{"value": value}
	}
	parts := strings.Split(path, ".")
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, exists := current[part]
		if !exists {
			child := map[string]any{}
			current[part] = child
			current = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %q crosses a non-object value", path)
		}
		current = child
	}
	current[parts[len(parts)-1]] = replacement
	return root, nil
}

func removeAtPath(value any, path string) (any, error) {
	root, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("path %q requires an object", path)
	}
	parts := strings.Split(path, ".")
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %q was not found", path)
		}
		current = next
	}
	delete(current, parts[len(parts)-1])
	return root, nil
}
