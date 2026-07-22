import type { Edge, Node } from "@xyflow/react";
import type {
  CardData,
  CardType,
  Port,
  WorkflowDefinition,
  WorkflowMetadata,
  WorkflowExportEnvelope,
  WorkflowVersionStatus,
} from "./types";

export const categoryAccent = (category: string) =>
  ({
    Entradas: "#4b8cff",
    Dados: "#efb75e",
    Transformação: "#56d6b6",
    Controle: "#d58af3",
    IA: "#6d9eff",
    Validação: "#e87b91",
    Resultado: "#efb75e",
    Saída: "#78d2a5",
    Infraestrutura: "#8e9aaa",
  })[category] || "#8e9aaa";

export const localCards: CardType[] = [
  {
    key: "trigger",
    name: "Trigger manual",
    category: "Entradas",
    description: "Inicia a verificação visual.",
    inputs: [],
    outputs: [
      { key: "event", label: "Evento", contract: "event", required: false },
    ],
  },
  {
    key: "transform",
    name: "Transformar contexto",
    category: "Transformação",
    description: "Propaga o contexto tipado.",
    inputs: [
      { key: "input", label: "Entrada", contract: "any", required: true },
    ],
    outputs: [
      { key: "output", label: "Saída", contract: "any", required: false },
    ],
  },
  {
    key: "condition",
    name: "Condição",
    category: "Controle",
    description: "Roteia o contexto para uma saída.",
    inputs: [
      { key: "input", label: "Entrada", contract: "any", required: true },
    ],
    outputs: [
      { key: "true", label: "Atende", contract: "any", required: false },
      { key: "false", label: "Alternativa", contract: "any", required: false },
    ],
  },
  {
    key: "log",
    name: "Log de execução",
    category: "Infraestrutura",
    description: "Registra o resultado da rota.",
    inputs: [
      { key: "input", label: "Entrada", contract: "any", required: false },
    ],
    outputs: [
      { key: "output", label: "Saída", contract: "any", required: false },
    ],
  },
];

const initialData = (
  card: CardType,
  key = card.key,
  config: Record<string, unknown> = {},
): CardData => ({
  key,
  type: card.key,
  name: card.name,
  category: card.category,
    inputs: card.inputs,
    outputs: card.outputs,
    errorOutput: card.error_output,
    config,
  status: "idle",
});

export const starterNodes: Node<CardData>[] = [
  {
    id: "trigger",
    type: "card",
    position: { x: 70, y: 260 },
    data: initialData(localCards[0]),
  },
  {
    id: "transform",
    type: "card",
    position: { x: 360, y: 260 },
    data: initialData(localCards[1]),
  },
  {
    id: "condition",
    type: "card",
    position: { x: 665, y: 260 },
    data: initialData(localCards[2], "condition", { equals: "never" }),
  },
  {
    id: "log",
    type: "card",
    position: { x: 975, y: 395 },
    data: initialData(localCards[3]),
  },
];

export const starterEdges: Edge[] = [
  {
    id: "trigger-transform",
    source: "trigger",
    sourceHandle: "out-event",
    target: "transform",
    targetHandle: "in-input",
    animated: true,
  },
  {
    id: "transform-condition",
    source: "transform",
    sourceHandle: "out-output",
    target: "condition",
    targetHandle: "in-input",
    animated: true,
  },
  {
    id: "condition-log",
    source: "condition",
    sourceHandle: "out-false",
    target: "log",
    targetHandle: "in-input",
    animated: true,
  },
];

export function canConnect(source?: Port, target?: Port) {
  return Boolean(
    source &&
    target &&
    (source.contract === "any" ||
      target.contract === "any" ||
      source.contract === target.contract),
  );
}

export const hasErrorRoute = (edges: Edge[], nodeKey: string) =>
  edges.some(
    (edge) => edge.source === nodeKey && edge.sourceHandle === "out-error",
  );

export const cardOutputPorts = (card: CardData) =>
  card.config.on_error === "route" && card.errorOutput
    ? [...card.outputs, card.errorOutput]
    : card.outputs;

export const workflowVersionStatusLabel = (status: WorkflowVersionStatus) =>
  ({
    draft: "Rascunho",
    published: "Publicada",
    archived: "Arquivada",
  })[status];

export const canPublishVersion = (status: WorkflowVersionStatus) =>
  status === "draft";

export const defaultWorkflowMetadata: WorkflowMetadata = {
  key: "studio-check",
  name: "Fluxo de verificação do Studio",
  description: "Pipeline local para validar cards, portas e estados.",
};

export function hydrateDefinition(
  definition: WorkflowDefinition,
  cards: CardType[],
): { nodes: Node<CardData>[]; edges: Edge[] } {
  const cardsByKey = new Map(cards.map((card) => [card.key, card]));
  const missing = definition.nodes.find((node) => !cardsByKey.has(node.type));
  if (missing)
    throw new Error(
      `O catálogo não contém o card "${missing.type}" necessário para abrir esta versão.`,
    );

  return {
    nodes: definition.nodes.map((node) => {
      const card = cardsByKey.get(node.type)!;
      return {
        id: node.key,
        type: "card",
        position: node.position,
        data: { ...initialData(card, node.key, node.config || {}), name: node.name },
      };
    }),
    edges: definition.edges.map((edge) => ({
      id: edge.key,
      source: edge.from_node,
      sourceHandle: `out-${edge.from_port}`,
      target: edge.to_node,
      targetHandle: `in-${edge.to_port}`,
      animated: true,
    })),
  };
}

export function toDefinition(
  nodes: Node<CardData>[],
  edges: Edge[],
  metadata: WorkflowMetadata = defaultWorkflowMetadata,
): WorkflowDefinition {
  return {
    ...metadata,
    nodes: nodes.map(({ id, data, position }) => ({
      key: id,
      type: data.type,
      name: data.name,
      config: data.config,
      position,
    })),
    edges: edges
      .filter((edge) => edge.sourceHandle && edge.targetHandle)
      .map((edge) => ({
        key: edge.id,
        from_node: edge.source,
        from_port: edge.sourceHandle!.replace("out-", ""),
        to_node: edge.target,
        to_port: edge.targetHandle!.replace("in-", ""),
      })),
  };
}

const unsafeConfigKey = (key: string) =>
  /(^|_)(secret|ciphertext|password|token|api_key)$/i.test(key);

function hasUnsafeConfig(value: unknown): boolean {
  if (Array.isArray(value)) return value.some(hasUnsafeConfig);
  if (!value || typeof value !== "object") return false;
  return Object.entries(value as Record<string, unknown>).some(
    ([key, nested]) => unsafeConfigKey(key) || hasUnsafeConfig(nested),
  );
}

export function validateWorkflowDefinition(
  definition: WorkflowDefinition,
  cards: CardType[],
): string | undefined {
  if (!definition.key.trim() || !definition.name.trim())
    return "O workflow importado precisa ter chave e nome.";
  const cardByType = new Map(cards.map((card) => [card.key, card]));
  const nodeKeys = new Set<string>();
  for (const node of definition.nodes) {
    if (!node.key || !node.name || !cardByType.has(node.type))
      return `O node "${node.key || "sem chave"}" é inválido ou usa um card indisponível.`;
    if (nodeKeys.has(node.key)) return `A chave de node "${node.key}" está duplicada.`;
    if (hasUnsafeConfig(node.config))
      return `O node "${node.key}" contém segredo, token ou ciphertext e não pode ser importado.`;
    nodeKeys.add(node.key);
  }
  const edgeKeys = new Set<string>();
  for (const edge of definition.edges) {
    const source = definition.nodes.find((node) => node.key === edge.from_node);
    const target = definition.nodes.find((node) => node.key === edge.to_node);
    const sourceCard = source && cardByType.get(source.type);
    const targetCard = target && cardByType.get(target.type);
    const sourcePorts = sourceCard && (sourceCard.error_output && source?.config.on_error === "route"
      ? [...sourceCard.outputs, sourceCard.error_output]
      : sourceCard.outputs);
    const output = sourcePorts?.find((port) => port.key === edge.from_port);
    const input = targetCard?.inputs.find((port) => port.key === edge.to_port);
    if (!edge.key || !source || !target || !output || !input)
      return `A conexão "${edge.key || "sem chave"}" referencia um node ou porta inválida.`;
    if (edgeKeys.has(edge.key)) return `A chave de conexão "${edge.key}" está duplicada.`;
    if (output.contract !== "any" && input.contract !== "any" && output.contract !== input.contract)
      return `A conexão "${edge.key}" usa contratos incompatíveis.`;
    edgeKeys.add(edge.key);
  }
  return undefined;
}

export function parseWorkflowExport(
  text: string,
  cards: CardType[],
): { envelope?: WorkflowExportEnvelope; error?: string } {
  try {
    const candidate = JSON.parse(text) as WorkflowExportEnvelope;
    if (candidate?.format !== "forgereview.workflow" || candidate.version !== 1 || !candidate.definition)
      return { error: "O arquivo não usa o envelope ForgeReview Workflow v1." };
    const error = validateWorkflowDefinition(candidate.definition, cards);
    return error ? { error } : { envelope: candidate };
  } catch {
    return { error: "O arquivo selecionado não contém JSON válido." };
  }
}

const uniqueKey = (base: string, used: Set<string>) => {
  let candidate = base;
  let index = 2;
  while (used.has(candidate)) candidate = `${base}-${index++}`;
  used.add(candidate);
  return candidate;
};

export function cloneWorkflowDefinition(
  definition: WorkflowDefinition,
  workflowKeys: Iterable<string>,
): WorkflowDefinition {
  const keys = new Set(workflowKeys);
  const workflowKey = uniqueKey(`${definition.key}-copy`, keys);
  const nodeKeys = new Set<string>();
  const nodes = definition.nodes.map((node) => ({ ...node, key: uniqueKey(node.key, nodeKeys), config: { ...node.config } }));
  const remappedNodes = new Map(definition.nodes.map((node, index) => [node.key, nodes[index].key]));
  const edgeKeys = new Set<string>();
  return {
    key: workflowKey,
    name: `${definition.name} (cópia)`,
    description: definition.description,
    nodes,
    edges: definition.edges.map((edge) => ({
      ...edge,
      key: uniqueKey(edge.key, edgeKeys),
      from_node: remappedNodes.get(edge.from_node) ?? edge.from_node,
      to_node: remappedNodes.get(edge.to_node) ?? edge.to_node,
    })),
  };
}

export const workflowExportEnvelope = (definition: WorkflowDefinition): WorkflowExportEnvelope => ({
  format: "forgereview.workflow",
  version: 1,
  definition,
});

export function reviewTemplate(
  cards: CardType[],
): { nodes: Node<CardData>[]; edges: Edge[] } | undefined {
  const types = [
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
  if (types.some((type) => !cards.some((card) => card.key === type)))
    return undefined;
  const cardFor = (type: string) => cards.find((card) => card.key === type)!;
  const node = (
    id: string,
    type: string,
    name: string,
    x: number,
    y: number,
    config: Record<string, unknown> = {},
  ): Node<CardData> => ({
    id,
    type: "card",
    position: { x, y },
     data: { ...initialData(cardFor(type), id, config), name },
  });
  return {
    nodes: [
      node("trigger", "trigger", "Webhook / manual", 40, 280),
      node("fetch", "fetch", "Buscar dados do PR", 315, 280, {
        owner: "",
        repo: "",
        pull_request: 0,
        integration: "",
       }),
      node("filter", "filter", "Filtrar arquivos", 610, 125, {
        include_extensions: [".go", ".ts", ".tsx", ".php"],
        ignore_generated: true,
      }),
      node("group", "group", "Agrupar arquivos", 900, 125, {
        max_files: 8,
        max_characters: 12000,
        group_by_extension: true,
      }),
      node("loop", "loop", "Revisar cada grupo", 1190, 125, {
        max_iterations: 20,
        concurrency: 1,
        on_error: "fail",
      }),
      node("template", "template", "Prompt de review", 1480, 125, {
        template:
          "Analise este grupo de arquivos e responda somente uma lista JSON de achados.",
      }),
      node("model", "model", "Modelo de review", 1775, 125, {
        model_profile: "",
        max_tokens: 2000,
        retry_limit: 0,
        retry_delay_ms: 0,
      }),
      node("validate", "validate", "Validar resposta", 2070, 125, {
        validate_paths: true,
      }),
      node("response-filter", "response_filter", "Filtrar achados", 2365, 125, {
        minimum_severity: "medium",
      }),
      node("consolidate", "consolidate", "Consolidar review", 2070, 480),
      node("format", "format", "Formatar review", 2365, 480),
      node("publish", "publish", "Publicar no Gitea", 2660, 480, {
        owner: "",
        repo: "",
        pull_request: 0,
        integration: "",
        medium_severity_event: "COMMENT",
        allow_autonomous_rejection: false,
      }),
    ],
    edges: [
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
    ].map(([source, sourceHandle, target, targetHandle]) => ({
      id: `${source}-${target}`,
      source,
      sourceHandle,
      target,
      targetHandle,
      animated: true,
    })),
  };
}

export const configText = (value: unknown) =>
  typeof value === "string" ? value : "";
export const configNumber = (value: unknown, fallback = 0) =>
  typeof value === "number" ? value : fallback;
export const configList = (value: unknown) =>
  Array.isArray(value)
    ? value
        .filter((item): item is string => typeof item === "string")
        .join(", ")
    : "";
