import assert from "node:assert/strict";
import test from "node:test";
import { normalizeCard } from "./api";
import {
  canConnect,
  canPublishVersion,
  hasErrorRoute,
  reviewTemplate,
  workflowVersionStatusLabel,
} from "./workflow";

test("catalog cards without ports normalize to empty lists", () => {
  const card = normalizeCard({
    key: "trigger",
    name: "Trigger",
    category: "Entradas",
    description: "",
    inputs: null,
    outputs: null,
  } as never);
  assert.deepEqual(card.inputs, []);
  assert.deepEqual(card.outputs, []);
});

test("typed connection accepts matching and wildcard contracts", () => {
  assert.equal(
    canConnect(
      { key: "out", label: "Out", contract: "event", required: false },
      { key: "in", label: "In", contract: "event", required: true },
    ),
    true,
  );
  assert.equal(
    canConnect(
      { key: "out", label: "Out", contract: "any", required: false },
      { key: "in", label: "In", contract: "files", required: true },
    ),
    true,
  );
});

test("typed connection blocks missing and incompatible ports", () => {
  assert.equal(
    canConnect(undefined, {
      key: "in",
      label: "In",
      contract: "event",
      required: true,
    }),
    false,
  );
  assert.equal(
    canConnect(
      { key: "out", label: "Out", contract: "event", required: false },
      { key: "in", label: "In", contract: "files", required: true },
    ),
    false,
  );
});

test("error routes require the explicit error output edge", () => {
  assert.equal(hasErrorRoute([], "template"), false);
  assert.equal(
    hasErrorRoute(
      [
        {
          id: "template-control",
          source: "template",
          sourceHandle: "out-error",
          target: "control",
          targetHandle: "in-error",
        },
      ],
      "template",
    ),
    true,
  );
});

test("workflow version lifecycle exposes publishable drafts only", () => {
  assert.equal(workflowVersionStatusLabel("draft"), "Rascunho");
  assert.equal(workflowVersionStatusLabel("published"), "Publicada");
  assert.equal(workflowVersionStatusLabel("archived"), "Arquivada");
  assert.equal(canPublishVersion("draft"), true);
  assert.equal(canPublishVersion("published"), false);
  assert.equal(canPublishVersion("archived"), false);
});

test("review template scopes group review through loop before one root publication", () => {
  const keys = [
    "trigger",
    "fetch",
    "filter",
    "group",
    "loop",
    "template",
    "model",
    "validate",
    "response_filter",
    "consolidate",
    "format",
    "publish",
  ];
  const template = reviewTemplate(
    keys.map((key) => ({
      key,
      name: key,
      category: "Test",
      description: "",
      inputs: [],
      outputs: [],
    })),
  );
  assert.ok(template);
  assert.deepEqual(
    template.edges.map((edge) => [
      edge.source,
      edge.sourceHandle,
      edge.target,
      edge.targetHandle,
    ]),
    [
      ["trigger", "out-event", "fetch", "in-event"],
      ["fetch", "out-files", "filter", "in-files"],
      ["filter", "out-files", "group", "in-files"],
      ["group", "out-groups", "loop", "in-items"],
      ["loop", "out-item", "template", "in-context"],
      ["template", "out-prompt", "model", "in-prompt"],
      ["model", "out-response", "validate", "in-response"],
      ["loop", "out-item", "validate", "in-files"],
      ["validate", "out-valid", "response-filter", "in-response"],
      ["loop", "out-results", "consolidate", "in-comments"],
      ["consolidate", "out-review", "format", "in-review"],
      ["format", "out-formatted", "publish", "in-formatted_review"],
    ],
  );
  assert.deepEqual(
    template.nodes.find((node) => node.id === "loop")?.data.config,
    { max_iterations: 20, concurrency: 1, on_error: "fail" },
  );
  assert.deepEqual(template.nodes.find((node) => node.id === "model")?.data.config, {
    integration: "",
    max_tokens: 2000,
    retry_limit: 0,
    retry_delay_ms: 0,
  });
});
