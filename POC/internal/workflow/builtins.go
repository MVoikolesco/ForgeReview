package workflow

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// BuiltinRegistry contains only safe, provider-independent executors. Gitea
// and LLM adapters are deliberately not represented by this foundation.
func BuiltinRegistry() *Registry {
	registry, err := NewRegistry(
		NodeType{Key: "source", Outputs: []PortSpec{{Key: "out", Contract: AnyContract}, {Key: "error", Contract: "error"}}, Execute: sourceExecutor},
		NodeType{Key: "passthrough", Inputs: []PortSpec{{Key: "in", Contract: AnyContract, Required: true}}, Outputs: []PortSpec{{Key: "out", Contract: AnyContract}, {Key: "error", Contract: "error"}}, Execute: passthroughExecutor},
		NodeType{Key: "condition", Inputs: []PortSpec{{Key: "in", Contract: AnyContract, Required: true}}, Outputs: []PortSpec{{Key: "true", Contract: AnyContract}, {Key: "false", Contract: AnyContract}, {Key: "error", Contract: "error"}}, Execute: conditionExecutor},
		NodeType{Key: "merge", Inputs: []PortSpec{{Key: "left", Contract: AnyContract, Required: true}, {Key: "right", Contract: AnyContract, Required: true}}, Outputs: []PortSpec{{Key: "out", Contract: AnyContract}, {Key: "error", Contract: "error"}}, Execute: mergeExecutor},
		NodeType{Key: "sink", Inputs: []PortSpec{{Key: "in", Contract: AnyContract, Required: true}}, Outputs: []PortSpec{{Key: "error", Contract: "error"}}, Execute: sinkExecutor},
		NodeType{Key: "fail", Inputs: []PortSpec{{Key: "in", Contract: AnyContract, Required: true}}, Outputs: []PortSpec{{Key: "error", Contract: "error"}}, Execute: failExecutor},
	)
	if err != nil {
		panic(fmt.Sprintf("invalid builtin workflow registry: %v", err))
	}
	return registry
}

func sourceExecutor(_ context.Context, invocation Invocation) (ExecutorResult, error) {
	return ExecutorResult{Outputs: map[string][]any{"out": {invocation.Node.Config["value"]}}}, nil
}

func passthroughExecutor(_ context.Context, invocation Invocation) (ExecutorResult, error) {
	values := make([]any, 0, len(invocation.Inputs["in"]))
	for _, token := range invocation.Inputs["in"] {
		values = append(values, token.Value)
	}
	return ExecutorResult{Outputs: map[string][]any{"out": values}}, nil
}

// condition reads a boolean input by default. Config may select a top-level map
// field and compare it with config.equals for deterministic condition routing.
func conditionExecutor(_ context.Context, invocation Invocation) (ExecutorResult, error) {
	inputs := invocation.Inputs["in"]
	if len(inputs) == 0 {
		return ExecutorResult{}, errors.New("condition requires input")
	}
	value := inputs[0].Value
	if field, _ := invocation.Node.Config["field"].(string); field != "" {
		object, ok := value.(map[string]any)
		if !ok {
			return ExecutorResult{}, fmt.Errorf("condition field %q requires an object", field)
		}
		value = object[field]
	}
	matched := false
	if expected, configured := invocation.Node.Config["equals"]; configured {
		matched = reflect.DeepEqual(value, expected)
	} else {
		matched, _ = value.(bool)
	}
	port := "false"
	if matched {
		port = "true"
	}
	return ExecutorResult{Outputs: map[string][]any{port: {inputs[0].Value}}}, nil
}

func mergeExecutor(_ context.Context, invocation Invocation) (ExecutorResult, error) {
	values := map[string]any{}
	for _, port := range []string{"left", "right"} {
		items := make([]any, 0, len(invocation.Inputs[port]))
		for _, token := range invocation.Inputs[port] {
			items = append(items, token.Value)
		}
		values[port] = items
	}
	return ExecutorResult{Outputs: map[string][]any{"out": {values}}}, nil
}

func sinkExecutor(context.Context, Invocation) (ExecutorResult, error) { return ExecutorResult{}, nil }

func failExecutor(_ context.Context, invocation Invocation) (ExecutorResult, error) {
	message, _ := invocation.Node.Config["message"].(string)
	if message == "" {
		message = "configured workflow failure"
	}
	return ExecutorResult{}, errors.New(message)
}
