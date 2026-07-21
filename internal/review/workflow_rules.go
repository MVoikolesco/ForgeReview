package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

type RuleOperator string

const (
	RuleEquals   RuleOperator = "equals"
	RuleContains RuleOperator = "contains"
	RuleIn       RuleOperator = "in"
	RuleGT       RuleOperator = "gt"
	RuleGTE      RuleOperator = "gte"
	RuleLT       RuleOperator = "lt"
	RuleLTE      RuleOperator = "lte"
	RuleMatches  RuleOperator = "matches"
	RuleExists   RuleOperator = "exists"
	RuleAll      RuleOperator = "all"
	RuleAny      RuleOperator = "any"
	RuleNot      RuleOperator = "not"
)

type CollectionScopeKind string

const (
	CollectionAny    CollectionScopeKind = "any"
	CollectionAll    CollectionScopeKind = "all"
	CollectionCount  CollectionScopeKind = "count"
	CollectionFilter CollectionScopeKind = "filter"
)

// Rule is the only expression format accepted for workflow routing.
type Rule struct {
	Operator RuleOperator     `json:"operator"`
	Path     string           `json:"path,omitempty"`
	Value    any              `json:"value,omitempty"`
	Rules    []Rule           `json:"rules,omitempty"`
	Scope    *CollectionScope `json:"scope,omitempty"`
}

// CollectionScope projects a collection before the parent rule is evaluated.
type CollectionScope struct {
	Kind CollectionScopeKind `json:"kind"`
	Path string              `json:"path"`
	Rule *Rule               `json:"rule,omitempty"`
}

func ValidateRule(rule Rule, schemaJSON string) error {
	var schema any
	if schemaJSON != "" {
		if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
			return fmt.Errorf("invalid contract schema: %w", err)
		}
	}
	return validateRule(rule, schema)
}

func validateRule(rule Rule, schema any) error {
	switch rule.Operator {
	case RuleAll, RuleAny:
		if len(rule.Rules) == 0 || rule.Path != "" || rule.Scope != nil {
			return fmt.Errorf("operator %s requires rules only", rule.Operator)
		}
		for _, child := range rule.Rules {
			if err := validateRule(child, schema); err != nil {
				return err
			}
		}
		return nil
	case RuleNot:
		if len(rule.Rules) != 1 || rule.Path != "" || rule.Scope != nil {
			return errors.New("operator not requires exactly one rule")
		}
		return validateRule(rule.Rules[0], schema)
	case RuleEquals, RuleContains, RuleIn, RuleGT, RuleGTE, RuleLT, RuleLTE, RuleMatches, RuleExists:
	default:
		return fmt.Errorf("unsupported rule operator %q", rule.Operator)
	}
	if len(rule.Rules) != 0 || rule.Path == "" && rule.Scope == nil {
		return fmt.Errorf("operator %s requires a path or scope", rule.Operator)
	}
	valueSchema := schema
	if rule.Scope != nil {
		var err error
		valueSchema, err = validateScope(*rule.Scope, schema)
		if err != nil {
			return err
		}
	} else if resolved, known := schemaAtPath(schema, rule.Path); known {
		valueSchema = resolved
	} else if schemaHasProperties(schema) {
		return fmt.Errorf("rule path %q is not defined by the contract schema", rule.Path)
	}
	if rule.Operator == RuleMatches {
		pattern, ok := rule.Value.(string)
		if !ok {
			return errors.New("operator matches requires a string value")
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("invalid matches pattern: %w", err)
		}
	}
	if rule.Operator == RuleGT || rule.Operator == RuleGTE || rule.Operator == RuleLT || rule.Operator == RuleLTE {
		if _, ok := number(rule.Value); !ok {
			return fmt.Errorf("operator %s requires a numeric value", rule.Operator)
		}
	}
	_ = valueSchema
	return nil
}

func validateScope(scope CollectionScope, schema any) (any, error) {
	if scope.Path == "" {
		return nil, errors.New("collection scope requires a path")
	}
	collectionSchema, known := schemaAtPath(schema, scope.Path)
	itemSchema := any(nil)
	if known {
		object, _ := collectionSchema.(map[string]any)
		if kind, _ := object["type"].(string); kind != "" && kind != "array" {
			return nil, fmt.Errorf("collection scope path %q is not an array", scope.Path)
		}
		itemSchema = object["items"]
	} else if schemaHasProperties(schema) {
		return nil, fmt.Errorf("collection scope path %q is not defined by the contract schema", scope.Path)
	}
	switch scope.Kind {
	case CollectionAny, CollectionAll, CollectionFilter:
		if scope.Rule == nil {
			return nil, fmt.Errorf("collection scope %s requires a rule", scope.Kind)
		}
		if err := validateRule(*scope.Rule, itemSchema); err != nil {
			return nil, err
		}
	case CollectionCount:
		if scope.Rule != nil {
			if err := validateRule(*scope.Rule, itemSchema); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("unsupported collection scope %q", scope.Kind)
	}
	return nil, nil
}

// EvaluateRule validates the payload against the contract schema before routing.
func EvaluateRule(rule Rule, schemaJSON string, payload any) (bool, error) {
	if err := ValidateRule(rule, schemaJSON); err != nil {
		return false, err
	}
	normalized, err := normalizeJSON(payload)
	if err != nil {
		return false, err
	}
	if schemaJSON != "" {
		var schema any
		if err = json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
			return false, err
		}
		if err = validateSchemaValue(schema, normalized, "$", true); err != nil {
			return false, fmt.Errorf("contract_invalid: %w", err)
		}
	}
	return evaluateRule(rule, normalized)
}

func evaluateRule(rule Rule, payload any) (bool, error) {
	switch rule.Operator {
	case RuleAll:
		for _, child := range rule.Rules {
			matched, err := evaluateRule(child, payload)
			if err != nil || !matched {
				return false, err
			}
		}
		return true, nil
	case RuleAny:
		for _, child := range rule.Rules {
			matched, err := evaluateRule(child, payload)
			if err != nil {
				return false, err
			}
			if matched {
				return true, nil
			}
		}
		return false, nil
	case RuleNot:
		matched, err := evaluateRule(rule.Rules[0], payload)
		return !matched, err
	}
	var left any
	var exists bool
	var err error
	if rule.Scope != nil {
		left, err = evaluateScope(*rule.Scope, payload)
		exists = err == nil
	} else {
		left, exists = valueAtPath(payload, rule.Path)
	}
	if err != nil {
		return false, err
	}
	if rule.Operator == RuleExists {
		want, ok := rule.Value.(bool)
		if rule.Value == nil {
			want = true
		} else if !ok {
			return false, errors.New("operator exists requires a boolean value")
		}
		return exists == want, nil
	}
	if !exists {
		return false, nil
	}
	switch rule.Operator {
	case RuleEquals:
		if actual, ok := number(left); ok {
			if expected, numeric := number(rule.Value); numeric {
				return actual == expected, nil
			}
		}
		return reflect.DeepEqual(left, rule.Value), nil
	case RuleContains:
		switch value := left.(type) {
		case string:
			needle, ok := rule.Value.(string)
			return ok && strings.Contains(value, needle), nil
		case []any:
			for _, item := range value {
				if reflect.DeepEqual(item, rule.Value) {
					return true, nil
				}
			}
		}
		return false, nil
	case RuleIn:
		items, ok := rule.Value.([]any)
		if !ok {
			return false, errors.New("operator in requires an array value")
		}
		for _, item := range items {
			if actual, ok := number(left); ok {
				if expected, numeric := number(item); numeric && actual == expected {
					return true, nil
				}
			}
			if reflect.DeepEqual(left, item) {
				return true, nil
			}
		}
		return false, nil
	case RuleMatches:
		text, ok := left.(string)
		if !ok {
			return false, nil
		}
		return regexp.MatchString(rule.Value.(string), text)
	case RuleGT, RuleGTE, RuleLT, RuleLTE:
		actual, ok := number(left)
		if !ok {
			return false, nil
		}
		expected, _ := number(rule.Value)
		switch rule.Operator {
		case RuleGT:
			return actual > expected, nil
		case RuleGTE:
			return actual >= expected, nil
		case RuleLT:
			return actual < expected, nil
		default:
			return actual <= expected, nil
		}
	}
	return false, nil
}

func evaluateScope(scope CollectionScope, payload any) (any, error) {
	value, ok := valueAtPath(payload, scope.Path)
	if !ok {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("collection scope path %q is not an array", scope.Path)
	}
	filtered := make([]any, 0, len(items))
	for _, item := range items {
		matched := true
		var err error
		if scope.Rule != nil {
			matched, err = evaluateRule(*scope.Rule, item)
			if err != nil {
				return nil, err
			}
		}
		if matched {
			filtered = append(filtered, item)
		}
	}
	switch scope.Kind {
	case CollectionAny:
		return len(filtered) > 0, nil
	case CollectionAll:
		return len(filtered) == len(items), nil
	case CollectionCount:
		return float64(len(filtered)), nil
	case CollectionFilter:
		return filtered, nil
	}
	return nil, fmt.Errorf("unsupported collection scope %q", scope.Kind)
}

func normalizeJSON(value any) (any, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err = decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func valueAtPath(value any, path string) (any, bool) {
	if path == "" || path == "$" {
		return value, true
	}
	current := value
	for _, part := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		switch item := current.(type) {
		case map[string]any:
			current, _ = item[part]
			if current == nil {
				_, exists := item[part]
				return current, exists
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(item) {
				return nil, false
			}
			current = item[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func schemaAtPath(schema any, path string) (any, bool) {
	current, ok := schema.(map[string]any)
	if !ok || path == "" {
		return schema, false
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		properties, hasProperties := current["properties"].(map[string]any)
		if !hasProperties {
			return nil, false
		}
		next, exists := properties[part]
		if !exists {
			return nil, false
		}
		current, ok = next.(map[string]any)
		if !ok {
			return next, true
		}
	}
	return current, true
}

func schemaHasProperties(schema any) bool {
	object, ok := schema.(map[string]any)
	if !ok {
		return false
	}
	_, ok = object["properties"].(map[string]any)
	return ok
}

func number(value any) (float64, bool) {
	switch item := value.(type) {
	case json.Number:
		value, err := item.Float64()
		return value, err == nil
	case float64:
		return item, true
	case float32:
		return float64(item), true
	case int:
		return float64(item), true
	case int64:
		return float64(item), true
	}
	return 0, false
}

func validateSchemaValue(schema, value any, path string, requiredOnly bool) error {
	object, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	typeName, _ := object["type"].(string)
	switch typeName {
	case "object":
		item, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		if required, ok := object["required"].([]any); ok {
			for _, field := range required {
				name, _ := field.(string)
				if _, exists := item[name]; !exists {
					return fmt.Errorf("%s.%s is required", path, name)
				}
			}
		}
		if properties, ok := object["properties"].(map[string]any); ok {
			for name, childSchema := range properties {
				if child, exists := item[name]; exists {
					if err := validateSchemaValue(childSchema, child, path+"."+name, requiredOnly); err != nil {
						return err
					}
				}
			}
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		for index, item := range items {
			if err := validateSchemaValue(object["items"], item, fmt.Sprintf("%s.%d", path, index), requiredOnly); err != nil {
				return err
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be a string", path)
		}
	case "number", "integer":
		if _, ok := number(value); !ok {
			return fmt.Errorf("%s must be numeric", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be boolean", path)
		}
	}
	return nil
}
