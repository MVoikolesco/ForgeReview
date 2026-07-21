package workflow

import (
	"context"
	"strings"
	"testing"
)

func TestValidateRejectsDisconnectedInputsAndIncompatiblePorts(t *testing.T) {
	registry := BuiltinRegistry()
	definition := Definition{
		Key:   "missing-input",
		Nodes: []Node{{ID: "source", Type: "source"}, {ID: "merge", Type: "merge"}},
		Edges: []Edge{{ID: "left", FromNodeID: "source", FromPort: "out", ToNodeID: "merge", ToPort: "left"}},
	}
	if err := Validate(definition, registry); err == nil || !strings.Contains(err.Error(), "right") {
		t.Fatalf("expected disconnected required port error, got %v", err)
	}

	typed, err := NewRegistry(
		NodeType{Key: "text-source", Outputs: []PortSpec{{Key: "out", Contract: "text"}}, Execute: sourceExecutor},
		NodeType{Key: "image-sink", Inputs: []PortSpec{{Key: "in", Contract: "image", Required: true}}, Execute: sinkExecutor},
	)
	if err != nil {
		t.Fatal(err)
	}
	definition = Definition{
		Key:   "bad-contract",
		Nodes: []Node{{ID: "source", Type: "text-source"}, {ID: "sink", Type: "image-sink"}},
		Edges: []Edge{{ID: "typed", FromNodeID: "source", FromPort: "out", ToNodeID: "sink", ToPort: "in"}},
	}
	if err := Validate(definition, typed); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("expected typed port error, got %v", err)
	}
}

func TestSchedulerWaitsForRequiredInputsInTheSameScope(t *testing.T) {
	scheduler, err := NewScheduler(Definition{
		Key:   "join",
		Nodes: []Node{{ID: "left", Type: "source"}, {ID: "right", Type: "source"}, {ID: "merge", Type: "merge"}, {ID: "sink", Type: "sink"}},
		Edges: []Edge{
			{ID: "left-merge", FromNodeID: "left", FromPort: "out", ToNodeID: "merge", ToPort: "left"},
			{ID: "right-merge", FromNodeID: "right", FromPort: "out", ToNodeID: "merge", ToPort: "right"},
			{ID: "merged", FromNodeID: "merge", FromPort: "out", ToNodeID: "sink", ToPort: "in"},
		},
	}, BuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	result, err := scheduler.Run(context.Background(), []Start{
		{NodeID: "left", Port: "out", Token: Token{Value: "L", Scope: Scope{ID: "iteration-1"}}},
		{NodeID: "right", Port: "out", Token: Token{Value: "R", Scope: Scope{ID: "iteration-1"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if countExecutions(result, "merge") != 1 || countExecutions(result, "sink") != 1 {
		t.Fatalf("required-input join did not execute once: %#v", result.Executions)
	}
	for _, execution := range result.Executions {
		if execution.NodeID == "merge" && (len(execution.Inputs["left"]) != 1 || len(execution.Inputs["right"]) != 1 || execution.Scope.ID != "iteration-1") {
			t.Fatalf("join lost scoped inputs: %#v", execution)
		}
	}
}

func TestSchedulerRoutesConditionOutput(t *testing.T) {
	for _, test := range []struct {
		name  string
		value bool
		want  string
	}{{"true", true, "true-sink"}, {"false", false, "false-sink"}} {
		t.Run(test.name, func(t *testing.T) {
			scheduler, err := NewScheduler(Definition{
				Key:   "condition-" + test.name,
				Nodes: []Node{{ID: "source", Type: "source"}, {ID: "condition", Type: "condition"}, {ID: "true-sink", Type: "sink"}, {ID: "false-sink", Type: "sink"}},
				Edges: []Edge{
					{ID: "input", FromNodeID: "source", FromPort: "out", ToNodeID: "condition", ToPort: "in"},
					{ID: "on-true", FromNodeID: "condition", FromPort: "true", ToNodeID: "true-sink", ToPort: "in"},
					{ID: "on-false", FromNodeID: "condition", FromPort: "false", ToNodeID: "false-sink", ToPort: "in"},
				},
			}, BuiltinRegistry())
			if err != nil {
				t.Fatal(err)
			}
			result, err := scheduler.Run(context.Background(), []Start{{NodeID: "source", Port: "out", Token: Token{Value: test.value}}})
			if err != nil {
				t.Fatal(err)
			}
			if countExecutions(result, test.want) != 1 || countExecutions(result, otherConditionSink(test.want)) != 0 {
				t.Fatalf("condition routed incorrectly: %#v", result.Executions)
			}
		})
	}
}

func TestSchedulerAppliesRouteAndContinueErrorPolicies(t *testing.T) {
	for _, test := range []struct {
		name      string
		policy    ErrorPolicy
		withSink  bool
		wantState string
	}{{"route", ErrorPolicy{Mode: ErrorRoute}, true, "routed"}, {"continue", ErrorPolicy{Mode: ErrorContinue}, false, "continued"}} {
		t.Run(test.name, func(t *testing.T) {
			nodes := []Node{{ID: "source", Type: "source"}, {ID: "fail", Type: "fail", Config: map[string]any{"message": "safe test failure"}, ErrorPolicy: test.policy}}
			edges := []Edge{{ID: "input", FromNodeID: "source", FromPort: "out", ToNodeID: "fail", ToPort: "in"}}
			if test.withSink {
				nodes = append(nodes, Node{ID: "error-sink", Type: "sink"})
				edges = append(edges, Edge{ID: "error", FromNodeID: "fail", FromPort: "error", ToNodeID: "error-sink", ToPort: "in"})
			}
			scheduler, err := NewScheduler(Definition{Key: "errors-" + test.name, Nodes: nodes, Edges: edges}, BuiltinRegistry())
			if err != nil {
				t.Fatal(err)
			}
			result, err := scheduler.Run(context.Background(), []Start{{NodeID: "source", Port: "out", Token: Token{Value: "input"}}})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Errors) != 1 || countExecutions(result, "fail") != 1 || executionState(result, "fail") != test.wantState {
				t.Fatalf("error policy was not applied: %#v", result)
			}
			if test.withSink && countExecutions(result, "error-sink") != 1 {
				t.Fatalf("routed error was not delivered: %#v", result.Executions)
			}
		})
	}
}

func countExecutions(result RunResult, nodeID string) int {
	count := 0
	for _, execution := range result.Executions {
		if execution.NodeID == nodeID {
			count++
		}
	}
	return count
}

func executionState(result RunResult, nodeID string) string {
	for _, execution := range result.Executions {
		if execution.NodeID == nodeID {
			return execution.Status
		}
	}
	return ""
}

func otherConditionSink(nodeID string) string {
	if nodeID == "true-sink" {
		return "false-sink"
	}
	return "true-sink"
}
