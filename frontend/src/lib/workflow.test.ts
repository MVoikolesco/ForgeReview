import assert from "node:assert/strict";
import test from "node:test";
import { normalizeCard } from "./api";
import {
  canConnect,
  canPublishVersion,
  hasErrorRoute,
  hydrateDefinition,
  isManualTrigger,
  localCards,
  reviewTemplate,
  removeSelectedElements,
  selectionHasChanged,
  starterEdges,
  starterNodes,
  toDefinition,
  cloneWorkflowDefinition,
  parseWorkflowExport,
  validateWorkflowDefinition,
  validateStudioWorkflow,
  workflowExportEnvelope,
  workflowVersionStatusLabel,
} from "./workflow";
import { publishedOfficialVersion, reviewPipelineState } from "./dashboard";

test("saved definitions hydrate canvas cards, edges, and configuration", () => {
  const hydrated = hydrateDefinition(
    {
      key: "saved-review",
      name: "Saved review",
      description: "",
      nodes: [
        {
          key: "saved-trigger",
          type: "trigger",
          name: "Ignored persisted label",
          config: { source: "manual" },
          position: { x: 42, y: 84 },
        },
      ],
      edges: [
        {
          key: "saved-edge",
          from_node: "saved-trigger",
          from_port: "event",
          to_node: "saved-trigger",
          to_port: "event",
        },
      ],
    },
    [
      {
        key: "trigger",
        name: "Trigger manual",
        category: "Entradas",
        description: "",
        inputs: [],
        outputs: [],
      },
    ],
  );

  assert.deepEqual(hydrated.nodes[0], {
    id: "saved-trigger",
    type: "card",
    position: { x: 42, y: 84 },
    data: {
      key: "saved-trigger",
      type: "trigger",
      name: "Ignored persisted label",
      category: "Entradas",
      inputs: [],
      outputs: [],
      errorOutput: undefined,
      config: { source: "manual" },
      status: "idle",
    },
  });
  assert.deepEqual(hydrated.edges[0], {
    id: "saved-edge",
    source: "saved-trigger",
    sourceHandle: "out-event",
    target: "saved-trigger",
    targetHandle: "in-event",
    animated: true,
  });
});

test("saving an opened canvas preserves its workflow identity", () => {
  const definition = toDefinition([], [], {
    key: "saved-review",
    name: "Saved review",
    description: "Retained from the opened version.",
  });
  assert.deepEqual(
    {
      key: definition.key,
      name: definition.name,
      description: definition.description,
    },
    {
      key: "saved-review",
      name: "Saved review",
      description: "Retained from the opened version.",
    },
  );
});

test("catalog availability and bounded model/publish configuration are validated", () => {
  const cards = [
    { key: "workflow", name: "Workflow", category: "Controle", description: "", inputs: [], outputs: [], available: false, unavailable_reason: "Ainda não suportado" },
    { key: "model", name: "Modelo", category: "IA", description: "", inputs: [], outputs: [] },
    { key: "fetch", name: "Buscar", category: "Dados", description: "", inputs: [], outputs: [] },
    { key: "publish", name: "Publicar", category: "Saída", description: "", inputs: [], outputs: [] },
  ];
  const definition = {
    key: "truthful", name: "Truthful", description: "", edges: [],
    nodes: [
      { key: "child", type: "workflow", name: "Child", config: {}, position: { x: 0, y: 0 } },
      { key: "model", type: "model", name: "Model", config: { model_profile: "reviewer", max_tokens: 128001 }, position: { x: 0, y: 0 } },
      { key: "fetch", type: "fetch", name: "Fetch", config: { integration: "gitea", owner: "acme", repo: "api", pull_request: 1, allow_autonomous_rejection: true }, position: { x: 0, y: 0 } },
      { key: "publish", type: "publish", name: "Publish", config: { integration: "gitea", owner: "acme", repo: "api", pull_request: 1, medium_severity_event: "APPROVE" }, position: { x: 0, y: 0 } },
    ],
  };
  const messages = validateStudioWorkflow(definition, cards).map((issue) => issue.message);
  assert.ok(messages.some((message) => message.includes("tipo indisponível")));
  assert.ok(messages.some((message) => message.includes("max_tokens")));
  assert.ok(messages.some((message) => message.includes("Mova a política")));
  assert.ok(messages.some((message) => message.includes("evento válido")));
  assert.ok(messages.some((message) => message.includes("coordenadas fixas")));
});

test("workflow export envelope validates safe graphs and rejects credential fields", () => {
  const definition = toDefinition(starterNodes, starterEdges);
  const parsed = parseWorkflowExport(JSON.stringify(workflowExportEnvelope(definition)), localCards);
  assert.equal(parsed.envelope?.definition.key, "studio-check");
  definition.nodes[0].config = { nested: { secret: "not-exportable" } };
  assert.match(validateWorkflowDefinition(definition, localCards) || "", /segredo/);
});

test("cloning creates a new workflow identity and only disambiguates duplicate graph keys", () => {
  const source = {
    key: "review", name: "Review", description: "", nodes: [
      { key: "node", type: "trigger", name: "One", config: {}, position: { x: 0, y: 0 } },
      { key: "node", type: "trigger", name: "Two", config: {}, position: { x: 1, y: 1 } },
    ], edges: [
      { key: "edge", from_node: "node", from_port: "event", to_node: "node", to_port: "event" },
      { key: "edge", from_node: "node", from_port: "event", to_node: "node", to_port: "event" },
    ],
  };
  const clone = cloneWorkflowDefinition(source, ["review-copy"]);
  assert.equal(clone.key, "review-copy-2");
  assert.equal(clone.name, "Review (cópia)");
  assert.deepEqual(clone.nodes.map((node) => node.key), ["node", "node-2"]);
  assert.deepEqual(clone.edges.map((edge) => edge.key), ["edge", "edge-2"]);
});

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

test("local Studio validation reports actionable identity, graph, configuration, and error-route failures", () => {
  const issues = validateStudioWorkflow({
    key: "", name: "", description: "", nodes: [
      { key: "template", type: "template", name: "Template", config: { on_error: "route" }, position: { x: 0, y: 0 } },
      { key: "model", type: "model", name: "Model", config: {}, position: { x: 1, y: 0 } },
    ], edges: [],
  }, [
    { key: "template", name: "Template", category: "Test", description: "", inputs: [], outputs: [], error_output: { key: "error", label: "Erro", contract: "error", required: false } },
    { key: "model", name: "Model", category: "Test", description: "", inputs: [{ key: "prompt", label: "Prompt", contract: "prompt", required: true }], outputs: [] },
  ]);
  assert.deepEqual(issues.map((issue) => issue.nodeKey), [undefined, "template", "model", "template", "model"]);
  assert.match(issues.map((issue) => issue.message).join("\n"), /chave e o nome/);
  assert.match(issues.map((issue) => issue.message).join("\n"), /template/);
  assert.match(issues.map((issue) => issue.message).join("\n"), /perfil de modelo/);
  assert.match(issues.map((issue) => issue.message).join("\n"), /rota de erro/);
  assert.match(issues.map((issue) => issue.message).join("\n"), /entrada obrigatória/);
});

test("removing selected cards also removes their connected edges without touching other graph elements", () => {
  const result = removeSelectedElements(starterNodes, starterEdges, ["transform"], ["condition-log"]);
  assert.deepEqual(result.nodes.map((node) => node.id), ["trigger", "condition", "log"]);
  assert.deepEqual(result.edges.map((edge) => edge.id), []);
  assert.equal(result.removedEdges, 3);
});

test("Studio manual execution only offers trigger cards in manual mode", () => {
  const trigger = starterNodes.find((node) => node.id === "trigger");
  assert.ok(trigger);
  assert.equal(isManualTrigger(trigger.data), true);
  assert.equal(isManualTrigger({ ...trigger.data, config: { mode: "api" } }), false);
  assert.equal(isManualTrigger({ ...trigger.data, config: { mode: "webhook" } }), false);
  assert.equal(isManualTrigger({ ...trigger.data, type: "fetch", config: { mode: "manual" } }), false);
});

test("repeated React Flow selection reports do not require another Studio state update", () => {
  assert.equal(selectionHasChanged(["transform"], ["transform"]), false);
  assert.equal(selectionHasChanged(["transform"], ["condition"]), true);
  assert.equal(selectionHasChanged(["transform"], ["transform", "condition"]), true);
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
      ["fetch", "out-pull_request", "publish", "in-pull_request"],
    ],
  );
  assert.deepEqual(
    template.nodes.find((node) => node.id === "loop")?.data.config,
    { max_iterations: 20, concurrency: 1, on_error: "fail" },
  );
  assert.deepEqual(template.nodes.find((node) => node.id === "model")?.data.config, {
    model_profile: "",
    max_tokens: 2000,
    retry_limit: 0,
    retry_delay_ms: 0,
  });
  assert.deepEqual(template.nodes.find((node) => node.id === "publish")?.data.config, {
    integration: "",
    medium_severity_event: "COMMENT",
    allow_autonomous_rejection: false,
  });
});

test("dashboard derives official review readiness and conservative publication safety", () => {
  const definition = {
    key: "official-gitea-pr-review", name: "Official", description: "", edges: [
      { key: "event", from_node: "trigger", from_port: "event", to_node: "fetch", to_port: "event" },
      { key: "target", from_node: "fetch", from_port: "pull_request", to_node: "publish", to_port: "pull_request" },
    ], nodes: [
      { key: "trigger", type: "trigger", name: "Trigger", config: {}, position: { x: 0, y: 0 } },
      { key: "fetch", type: "fetch", name: "Fetch", config: { integration: "gitea" }, position: { x: 0, y: 0 } },
      { key: "model", type: "model", name: "Model", config: { model_profile: "reviewer" }, position: { x: 0, y: 0 } },
      { key: "publish", type: "publish", name: "Publish", config: { integration: "gitea", allow_autonomous_rejection: false, medium_severity_event: "COMMENT" }, position: { x: 0, y: 0 } },
    ],
  };
  const state = reviewPipelineState(definition, [
    { key: "gitea", name: "Gitea", type: "gitea", config: { base_url: "https://gitea.example" }, secret_configured: true, status: "active" },
    { key: "models", name: "Models", type: "openai", config: { base_url: "https://models.example" }, secret_configured: true, status: "active" },
  ], [{ key: "reviewer", name: "Reviewer", integration_key: "models", model: "review", status: "active" }]);
  assert.equal(state.readiness, "Pronta para revisão");
  assert.equal(state.safe, true);
  assert.equal(publishedOfficialVersion([{ key: "official-gitea-pr-review", name: "Official", description: "", versions: [{ version_id: 1, version: 1, status: "published", created_at: "" }] }])?.version_id, 1);
});
