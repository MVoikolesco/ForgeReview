package workflow

import (
	"context"
	"strings"
	"testing"
)

func embeddedSubpipelineDefinition() Definition {
	return Definition{
		Key:  "embedded",
		Name: "Embedded",
		Nodes: []Node{
			{Key: "start", Type: "trigger", Name: "Start"},
			{
				Key: "box", Type: "subpipeline", Name: "Box",
				Position: Position{X: 300, Y: 100},
				Size:     &NodeSize{Width: 640, Height: 240},
				Config: map[string]any{
					"input_ports":  []Port{{Key: "input", Label: "Input", Contract: "any", Required: true}},
					"output_ports": []Port{{Key: "output", Label: "Output", Contract: "any", Required: true}},
				},
			},
			{Key: "inside", Type: "transform", Name: "Inside", ParentKey: "box"},
			{Key: "after", Type: "log", Name: "After"},
		},
		Edges: []Edge{
			{Key: "external-in", FromNode: "start", FromPort: "event", ToNode: "box", ToPort: "entry:input"},
			{Key: "internal-in", FromNode: "box", FromPort: "entry:input", ToNode: "inside", ToPort: "input"},
			{Key: "internal-out", FromNode: "inside", FromPort: "output", ToNode: "box", ToPort: "exit:output"},
			{Key: "external-out", FromNode: "box", FromPort: "exit:output", ToNode: "after", ToPort: "input"},
		},
	}
}

func TestEmbeddedSubpipelineValidatesAndExecutesAsTransparentBoundary(t *testing.T) {
	definition := embeddedSubpipelineDefinition()
	if err := Validate(definition, DefaultCatalog()); err != nil {
		t.Fatalf("validate embedded subpipeline: %v", err)
	}
	executable := executableDefinition(definition)
	if len(executable.Nodes) != 3 || len(executable.Edges) != 2 {
		t.Fatalf("flattened definition = %#v", executable)
	}
	report, err := Run(context.Background(), definition, DefaultCatalog(), map[string]any{"value": "ok"})
	if err != nil {
		t.Fatalf("run embedded subpipeline: %v", err)
	}
	if report.Status != "completed" || len(report.Runs) != 3 {
		t.Fatalf("embedded report = %#v", report)
	}
	for _, run := range report.Runs {
		if run.NodeKey == "box" {
			t.Fatalf("structural subpipeline must not execute: %#v", report.Runs)
		}
	}
}

func TestEmbeddedSubpipelineRejectsDirectBoundaryCrossingAndHalfConnectedPort(t *testing.T) {
	definition := embeddedSubpipelineDefinition()
	definition.Edges[0] = Edge{Key: "invalid", FromNode: "start", FromPort: "event", ToNode: "inside", ToPort: "input"}
	if err := Validate(definition, DefaultCatalog()); err == nil || !strings.Contains(err.Error(), "crosses a subpipeline boundary") {
		t.Fatalf("direct boundary validation error = %v", err)
	}

	definition = embeddedSubpipelineDefinition()
	definition.Edges = definition.Edges[:3]
	if err := Validate(definition, DefaultCatalog()); err == nil || !strings.Contains(err.Error(), "must be connected on both sides") {
		t.Fatalf("half-connected boundary validation error = %v", err)
	}
}

func TestParameterizedSubpipelineExpandsRecipeOnlyForRuntime(t *testing.T) {
	definition := OfficialReviewDefinition()
	if err := Validate(definition, DefaultCatalog()); err != nil {
		t.Fatalf("validate parameterized recipe: %v", err)
	}
	executable := executableDefinition(definition)
	if len(executable.Nodes) != 39 || len(executable.Edges) != 56 {
		t.Fatalf("expanded recipe shape = %d nodes, %d edges", len(executable.Nodes), len(executable.Edges))
	}
	templates := map[string]Node{}
	for _, node := range executable.Nodes {
		if node.Type == "subpipeline" || node.ParentKey != "" {
			t.Fatalf("runtime definition retained structural node: %#v", node)
		}
		if node.Type == "template" {
			templates[node.Key] = node
		}
	}
	if len(templates) != 6 {
		t.Fatalf("expanded templates = %#v", templates)
	}
	security := templates["review-template::security"]
	contracts := templates["review-template::contracts"]
	if security.Config["review_contract_key"] != "review.security" ||
		contracts.Config["review_contract_key"] != "review.contracts" ||
		security.Config["template"] == contracts.Config["template"] {
		t.Fatalf("instance overrides were not isolated: security=%#v contracts=%#v", security.Config, contracts.Config)
	}
}

func TestParameterizedSubpipelineRequiresEnabledPinnedInstances(t *testing.T) {
	definition := OfficialReviewDefinition()
	for index := range definition.Nodes {
		if definition.Nodes[index].Key == "review-recipe" {
			definition.Nodes[index].Config["instances"] = []map[string]any{{
				"key": "security", "name": "Security", "enabled": false,
				"template": "Prompt", "review_contract_key": "review.security", "review_contract_version": 2,
			}}
		}
	}
	if err := Validate(definition, DefaultCatalog()); err == nil || !strings.Contains(err.Error(), "at least one enabled instance") {
		t.Fatalf("disabled instances validation error = %v", err)
	}
}
