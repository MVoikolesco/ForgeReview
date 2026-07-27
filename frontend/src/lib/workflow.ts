import type { Edge, Node } from "@xyflow/react";
import type {
  CardData,
  CardType,
  Port,
  WorkflowDefinition,
  WorkflowMetadata,
  WorkflowExportEnvelope,
  WorkflowVersionStatus,
  ExecutionReport,
  WorkflowInterfaceField,
} from "./types";
import { responseSchemaError } from "./response-contracts";
import { reviewChecklistError } from "./review-checklists";

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

const reviewerGroupSpecs = [
  {
    key: "security",
    name: "Segurança",
    prompt:
      "Atue somente como reviewer de segurança desta unidade semântica. Procure bypass concreto de autenticação, autorização ou isolamento de dados introduzido pela alteração. Produza apenas CandidateFinding sustentados por evidência observável; quando faltar contexto, declare required_context. Não confirme nem publique achados. Responda somente uma lista JSON.",
  },
  {
    key: "correctness",
    name: "Corretude",
    prompt:
      "Atue somente como reviewer de corretude desta unidade semântica. Procure comportamento incorreto reproduzível, limites quebrados e tratamento de erro defeituoso introduzidos pela alteração. Produza apenas CandidateFinding sustentados por evidência observável; quando faltar contexto, declare required_context. Não confirme nem publique achados. Responda somente uma lista JSON.",
  },
  {
    key: "contracts",
    name: "Contratos",
    prompt:
      "Atue somente como reviewer de contratos desta unidade semântica. Procure quebra concreta de API, formato persistido, schema ou integração introduzida pela alteração. Não trate contrato ausente no diff como inexistente; quando não for observável, declare required_context. Não confirme nem publique achados. Responda somente uma lista JSON.",
  },
  {
    key: "performance",
    name: "Performance",
    prompt:
      "Atue somente como reviewer de performance desta unidade semântica. Procure regressão material em caminho frequente, trabalho repetido evitável ou crescimento não limitado introduzido pela alteração. Produza apenas CandidateFinding sustentados por evidência observável; quando faltar contexto, declare required_context. Não confirme nem publique achados. Responda somente uma lista JSON.",
  },
  {
    key: "architecture",
    name: "Arquitetura",
    prompt:
      "Atue somente como reviewer de arquitetura desta unidade semântica. Procure violação funcional de fronteira, dependência indevida ou efeito externo introduzido no lugar errado. Produza apenas CandidateFinding sustentados por evidência observável; quando faltar contexto, declare required_context. Não confirme nem publique achados. Responda somente uma lista JSON.",
  },
  {
    key: "observability",
    name: "Observabilidade",
    prompt:
      "Atue somente como reviewer de observabilidade desta unidade semântica. Procure falha operacional nova que fique silenciosa ou perca logs, métricas ou contexto indispensável ao diagnóstico. Produza apenas CandidateFinding sustentados por evidência observável; quando faltar contexto, declare required_context. Não confirme nem publique achados. Responda somente uma lista JSON.",
  },
] as const;

export type SubpipelineInstance = {
  key: string;
  name: string;
  enabled: boolean;
  template: string;
  review_contract_key: string;
  review_contract_version: number;
  model_profile: string;
  validator_model_profile: string;
  minimum_severity: string;
};

export const subpipelineInstances = (
  config: Record<string, unknown>,
): SubpipelineInstance[] =>
  Array.isArray(config.instances)
    ? config.instances.flatMap((value) => {
        if (!value || typeof value !== "object") return [];
        const item = value as Record<string, unknown>;
        if (typeof item.key !== "string" || typeof item.name !== "string")
          return [];
        return [
          {
            key: item.key,
            name: item.name,
            enabled: item.enabled !== false,
            template: typeof item.template === "string" ? item.template : "",
            review_contract_key:
              typeof item.review_contract_key === "string"
                ? item.review_contract_key
                : "",
            review_contract_version:
              typeof item.review_contract_version === "number"
                ? item.review_contract_version
                : 1,
            model_profile:
              typeof item.model_profile === "string" ? item.model_profile : "",
            validator_model_profile:
              typeof item.validator_model_profile === "string"
                ? item.validator_model_profile
                : "",
            minimum_severity:
              typeof item.minimum_severity === "string"
                ? item.minimum_severity
                : "medium",
          },
        ];
      })
    : [];

export function compactReviewerCards(nodes: Node<CardData>[]) {
  return nodes.map((node) =>
    node.parentId
      ? { ...node, data: { ...node.data, compact: true }, zIndex: 2 }
      : {
          ...node,
          data: { ...node.data, compact: false },
          zIndex: node.data.type === "subpipeline" ? 0 : 1,
        },
  );
}

export const reviewerVisualGroups = (nodes: Node<CardData>[]) =>
  nodes.filter((node) => node.data.type === "subpipeline");

export function subpipelinePorts(
  config: Record<string, unknown>,
  key: "input_ports" | "output_ports",
): Port[] {
  if (!Array.isArray(config[key])) return [];
  return (config[key] as unknown[]).flatMap((value) => {
    if (!value || typeof value !== "object") return [];
    const item = value as Record<string, unknown>;
    if (
      typeof item.key !== "string" ||
      typeof item.label !== "string" ||
      typeof item.contract !== "string"
    )
      return [];
    return [
      {
        key: item.key,
        label: item.label,
        contract: item.contract,
        required: Boolean(item.required),
      },
    ];
  });
}

export const isManualTrigger = (card: Pick<CardData, "type" | "config">) =>
  card.type === "trigger" &&
  [undefined, "", "manual"].includes(card.config.mode as string | undefined);

const terminalStatusPriority = { completed: 1, partial: 2, failed: 3 } as const;

export function applyExecutionReport(
  nodes: Node<CardData>[],
  report: ExecutionReport,
) {
  const runs = Array.isArray(report.runs) ? report.runs : [];
  // A queued report legitimately has no node progress. Keep the last safe card
  // state rather than erasing it when an older server omits the runs field.
  if (!runs.length) return nodes;
  const statuses = new Map<string, CardData["status"]>();
  for (const run of runs) {
    const current = statuses.get(run.node_key);
    // Poll responses can overlap. A later stale running record must never
    // replace observed terminal progress for the same card.
    if (
      !current ||
      current === "running" ||
      (run.status !== "running" &&
        terminalStatusPriority[
          run.status as keyof typeof terminalStatusPriority
        ] >
          (terminalStatusPriority[
            current as keyof typeof terminalStatusPriority
          ] ?? 0))
    ) {
      statuses.set(run.node_key, run.status);
    }
  }
  const aggregate = (
    values: CardData["status"][],
  ): CardData["status"] => {
    if (values.includes("failed")) return "failed";
    if (values.includes("running")) return "running";
    if (values.includes("partial")) return "partial";
    if (values.includes("completed")) return "completed";
    return "idle";
  };
  return nodes.map((node) => {
    const instanceKeys =
      node.data.type === "subpipeline"
        ? new Set(
            subpipelineInstances(node.data.config)
              .filter((instance) => instance.enabled)
              .map((instance) => instance.key),
          )
        : undefined;
    const values = [...statuses.entries()]
      .filter(([key]) => {
        if (key === node.id || key.startsWith(`${node.id}::`)) return true;
        if (!instanceKeys?.size) return false;
        const separator = key.lastIndexOf("::");
        return separator >= 0 && instanceKeys.has(key.slice(separator + 2));
      })
      .map(([, status]) => status);
    return {
      ...node,
      data: { ...node.data, status: aggregate(values) },
    };
  });
}

export const resetExecutionStatuses = (nodes: Node<CardData>[]) =>
  nodes.map((node) => ({
    ...node,
    data: { ...node.data, status: "idle" as const },
  }));

export const edgeIsActivelyPropagating = (
  edge: Edge,
  nodes: Node<CardData>[],
) =>
  nodes.some(
    (node) => node.id === edge.target && node.data.status === "running",
  );

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
    key: "subpipeline",
    name: "Subpipeline",
    category: "Controle",
    description:
      "Organiza cards em um container com entradas e saídas próprias.",
    inputs: [],
    outputs: [],
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
    data: initialData(localCards[4]),
  },
];

export const starterEdges: Edge[] = [
  {
    id: "trigger-transform",
    source: "trigger",
    sourceHandle: "out-event",
    target: "transform",
    targetHandle: "in-input",
    animated: false,
  },
  {
    id: "transform-condition",
    source: "transform",
    sourceHandle: "out-output",
    target: "condition",
    targetHandle: "in-input",
    animated: false,
  },
  {
    id: "condition-log",
    source: "condition",
    sourceHandle: "out-false",
    target: "log",
    targetHandle: "in-input",
    animated: false,
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
    (edge) =>
      edge.source === nodeKey &&
      edge.sourceHandle === "out-error",
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

  const orderedNodes = [...definition.nodes].sort(
    (left, right) => Number(Boolean(left.parent_key)) - Number(Boolean(right.parent_key)),
  );
  return {
    nodes: orderedNodes.map((node) => {
      const card = cardsByKey.get(node.type)!;
      const isSubpipeline = node.type === "subpipeline";
      const inputs = isSubpipeline
        ? [
            ...subpipelinePorts(node.config || {}, "input_ports").map(
              (port) => ({ ...port, key: `entry:${port.key}` }),
            ),
            ...subpipelinePorts(node.config || {}, "output_ports").map(
              (port) => ({ ...port, key: `exit:${port.key}` }),
            ),
          ]
        : card.inputs;
      const outputs = isSubpipeline
        ? [...inputs]
        : card.outputs;
      return {
        id: node.key,
        type: isSubpipeline ? "subpipeline" : "card",
        position: node.position,
        ...(isSubpipeline
          ? {
              dragHandle: ".drag-handle",
              width: node.size?.width,
              height: node.size?.height,
              initialWidth: node.size?.width,
              initialHeight: node.size?.height,
              style: node.size
                ? { width: node.size.width, height: node.size.height }
                : undefined,
            }
          : {}),
        ...(node.parent_key
          ? {
              parentId: node.parent_key,
              extent: "parent" as const,
              expandParent: false,
            }
          : {}),
        data: {
          ...initialData(card, node.key, node.config || {}),
          name: node.name,
          inputs,
          outputs,
        },
      };
    }),
    edges: definition.edges.map((edge) => ({
      id: edge.key,
      source: edge.from_node,
      sourceHandle: `out-${edge.from_port}`,
      target: edge.to_node,
      targetHandle: `in-${edge.to_port}`,
      animated: false,
    })),
  };
}

export function toDefinition(
  nodes: Node<CardData>[],
  edges: Edge[],
  metadata: WorkflowMetadata = defaultWorkflowMetadata,
): WorkflowDefinition {
  const declaredInputs = nodes
    .filter((node) => node.data.type === "trigger")
    .flatMap((node) => {
      if (Array.isArray(node.data.config.published_input_fields))
        return node.data.config
          .published_input_fields as WorkflowInterfaceField[];
      return Array.isArray(node.data.config.published_inputs)
        ? node.data.config.published_inputs
            .filter((value): value is string => nonEmptyText(value))
            .map((key) => ({
              key,
              label: key,
              contract: "any",
              required: true,
            }))
        : [];
    });
  const declaredOutputs = nodes.flatMap((node) => {
    const key = configText(node.data.config.published_output_key);
    const portKey = configText(node.data.config.published_output_port);
    if (!key || !portKey) return [];
    const port = node.data.outputs.find(
      (candidate) => candidate.key === portKey,
    );
    return [
      {
        key,
        label: key,
        contract: port?.contract || "any",
        required: Boolean(node.data.config.published_output_required),
        node_key: node.id,
        port_key: portKey,
      },
    ];
  });
  const interfaceConfigured = nodes.some(
    (node) =>
      Object.prototype.hasOwnProperty.call(
        node.data.config,
        "published_inputs",
      ) ||
      Object.prototype.hasOwnProperty.call(
        node.data.config,
        "published_input_fields",
      ) ||
      Object.prototype.hasOwnProperty.call(
        node.data.config,
        "published_output_key",
      ) ||
      Object.prototype.hasOwnProperty.call(
        node.data.config,
        "published_output_port",
      ),
  );
  return {
    ...metadata,
    interface: interfaceConfigured
      ? {
          trigger_node_key: nodes.find((node) => node.data.type === "trigger")
            ?.id,
          inputs: declaredInputs,
          outputs: declaredOutputs,
        }
      : metadata.interface,
    nodes: nodes.map(({ id, data, position, parentId, measured, width, height }) => ({
      key: id,
      type: data.type,
      name: data.name,
      config: data.config,
      position,
      parent_key: parentId,
      size:
        data.type === "subpipeline"
          ? {
              width: measured?.width ?? width ?? 900,
              height: measured?.height ?? height ?? 190,
            }
          : undefined,
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
  return validateStudioWorkflow(definition, cards)[0]?.message;
}

export type WorkflowValidationIssue = {
  nodeKey?: string;
  message: string;
};

const nonEmptyText = (value: unknown) =>
  typeof value === "string" && value.trim().length > 0;
const positiveInteger = (value: unknown) =>
  typeof value === "number" && Number.isInteger(value) && value > 0;

const validKeepAlive = (value: unknown) => {
  if (value === undefined || value === "") return true;
  if (value === "0") return true;
  if (typeof value !== "string") return false;
  const match = value.trim().match(/^(\d+(?:\.\d+)?)(ms|s|m|h)$/);
  if (!match) return false;
  const amount = Number(match[1]);
  const unitMilliseconds: Record<string, number> = {
    ms: 1,
    s: 1000,
    m: 60000,
    h: 3600000,
  };
  const milliseconds = amount * (unitMilliseconds[match[2]] ?? 0);
  return amount > 0 && milliseconds <= 24 * 60 * 60 * 1000;
};

const validDataPath = (value: unknown) =>
  typeof value === "string" &&
  /^[A-Za-z_-][A-Za-z0-9_-]*(?:\.[A-Za-z_-][A-Za-z0-9_-]*)*$/.test(value);

const validDeclarativeOperations = (value: unknown) =>
  value === undefined ||
  (Array.isArray(value) &&
    value.length >= 1 &&
    value.length <= 32 &&
    value.every((raw) => {
      if (!raw || typeof raw !== "object") return false;
      const operation = raw as Record<string, unknown>;
      if (["select", "remove"].includes(String(operation.op)))
        return validDataPath(operation.path);
      if (operation.op === "set")
        return (
          validDataPath(operation.path) &&
          Object.prototype.hasOwnProperty.call(operation, "value")
        );
      if (operation.op === "rename")
        return validDataPath(operation.path) && validDataPath(operation.to);
      if (operation.op === "coalesce")
        return (
          Array.isArray(operation.paths) &&
          operation.paths.length > 0 &&
          operation.paths.every(validDataPath) &&
          validDataPath(operation.to)
        );
      return false;
    }));

/**
 * Mirrors graph and configuration failures that the client can know from the
 * current catalog. The backend still validates every persisted definition.
 */
export function validateStudioWorkflow(
  definition: WorkflowDefinition,
  cards: CardType[],
): WorkflowValidationIssue[] {
  const issues: WorkflowValidationIssue[] = [];
  if (!definition.key.trim() || !definition.name.trim())
    issues.push({ message: "Informe a chave e o nome do workflow." });
  if (definition.nodes.length === 0)
    issues.push({ message: "Adicione ao menos um card ao workflow." });
  const cardByType = new Map(cards.map((card) => [card.key, card]));
  const knownContracts = new Set([
    "any",
    "string",
    "number",
    "boolean",
    "object",
    "list",
    ...cards.flatMap((card) =>
      [...card.inputs, ...card.outputs].map((port) => port.contract),
    ),
  ]);
  const nodeKeys = new Set<string>();
  for (const node of definition.nodes) {
    const catalogCard = cardByType.get(node.type);
    if (
      !node.key ||
      !node.name ||
      !catalogCard ||
      catalogCard.available === false
    )
      issues.push({
        nodeKey: node.key,
        message: `O card "${node.key || "sem chave"}" é inválido ou usa um tipo indisponível.`,
      });
    if (nodeKeys.has(node.key))
      issues.push({
        nodeKey: node.key,
        message: `A chave do card "${node.key}" está duplicada.`,
      });
    if (hasUnsafeConfig(node.config))
      issues.push({
        nodeKey: node.key,
        message: `O card "${node.key}" contém segredo, token ou ciphertext e não pode ser salvo.`,
      });
    nodeKeys.add(node.key);
    const policy =
      configText(node.config.on_error) ||
      (node.type === "error_control" ? "continue" : "fail");
    if (node.type === "error_control") {
      if (!["fail", "continue", "fallback"].includes(policy))
        issues.push({
          nodeKey: node.key,
          message: `O card "${node.name}" usa uma política de erro inválida.`,
        });
      if (policy === "fallback" && !nonEmptyText(node.config.fallback_result))
        issues.push({
          nodeKey: node.key,
          message: `Defina o resultado de fallback do card "${node.name}".`,
        });
    } else if (!["fail", "continue", "partial", "route"].includes(policy)) {
      issues.push({
        nodeKey: node.key,
        message: `O card "${node.name}" usa uma política de erro inválida.`,
      });
    }
    if (node.type === "template") {
      if (!nonEmptyText(node.config.template))
        issues.push({
          nodeKey: node.key,
          message: `Defina o template do card "${node.name}".`,
        });
      const hasContractKey = node.config.review_contract_key !== undefined;
      const hasContractVersion =
        node.config.review_contract_version !== undefined;
      if (
        hasContractKey !== hasContractVersion ||
        (hasContractKey &&
          (!nonEmptyText(node.config.review_contract_key) ||
            !Number.isInteger(node.config.review_contract_version) ||
            (node.config.review_contract_version as number) < 1))
      )
        issues.push({
          nodeKey: node.key,
          message: `"${node.name}" precisa de uma chave e versão positiva do contrato.`,
        });
    }
    if (node.type === "model" || node.type === "candidate_validator") {
      if (
        !nonEmptyText(node.config.model_profile) &&
        !nonEmptyText(node.config.integration)
      )
        issues.push({
          nodeKey: node.key,
          message: `Selecione um perfil de modelo para "${node.name}".`,
        });
      if (node.config.review_checklist !== undefined) {
        const checklistError = reviewChecklistError(
          node.config.review_checklist,
        );
        if (checklistError)
          issues.push({
            nodeKey: node.key,
            message: `"${node.name}" possui checklist inválida: ${checklistError}`,
          });
      }
      for (const [key, max] of [
        ["retry_limit", 3],
        ["retry_delay_ms", 60000],
      ] as const) {
        const value = node.config[key];
        if (
          value !== undefined &&
          (!Number.isInteger(value) ||
            (value as number) < 0 ||
            (value as number) > max)
        )
          issues.push({
            nodeKey: node.key,
            message: `"${node.name}" precisa de ${key} entre 0 e ${max}.`,
          });
      }
      const maxTokens = node.config.max_tokens;
      if (
        maxTokens !== undefined &&
        (!Number.isInteger(maxTokens) ||
          (maxTokens as number) < 1 ||
          (maxTokens as number) > 128000)
      )
        issues.push({
          nodeKey: node.key,
          message: `"${node.name}" precisa de max_tokens entre 1 e 128000.`,
        });
      const temperature = node.config.temperature;
      if (
        temperature !== undefined &&
        (typeof temperature !== "number" ||
          !Number.isFinite(temperature) ||
          temperature < 0 ||
          temperature > 2)
      )
        issues.push({
          nodeKey: node.key,
          message: `"${node.name}" precisa de temperature entre 0 e 2.`,
        });
      const topP = node.config.top_p;
      if (
        topP !== undefined &&
        (typeof topP !== "number" ||
          !Number.isFinite(topP) ||
          topP <= 0 ||
          topP > 1)
      )
        issues.push({
          nodeKey: node.key,
          message: `"${node.name}" precisa de top_p maior que 0 e no máximo 1.`,
        });
      const timeout = node.config.timeout_seconds;
      if (
        timeout !== undefined &&
        (!Number.isInteger(timeout) ||
          (timeout as number) < 1 ||
          (timeout as number) > 3600)
      )
        issues.push({
          nodeKey: node.key,
          message: `"${node.name}" precisa de timeout_seconds entre 1 e 3600.`,
        });
      if (!validKeepAlive(node.config.keep_alive))
        issues.push({
          nodeKey: node.key,
          message: `"${node.name}" precisa de keep_alive igual a 0 ou uma duração de até 24h.`,
        });
      const fallback = configText(node.config.fallback_model_profile);
      if (fallback && fallback === configText(node.config.model_profile))
        issues.push({
          nodeKey: node.key,
          message: `O fallback de "${node.name}" deve usar outro perfil.`,
        });
      const costs = [
        node.config.input_cost_per_million_usd,
        node.config.output_cost_per_million_usd,
        node.config.max_cost_usd,
      ];
      if (
        costs.some(
          (value) =>
            value !== undefined &&
            (typeof value !== "number" || !Number.isFinite(value) || value < 0),
        ) ||
        ((node.config.max_cost_usd as number) > 0 &&
          !((node.config.input_cost_per_million_usd as number) > 0) &&
          !((node.config.output_cost_per_million_usd as number) > 0))
      )
        issues.push({
          nodeKey: node.key,
          message: `"${node.name}" requer preços não negativos para aplicar orçamento de custo.`,
        });
    }
    if (node.type === "validate" && node.config.response_schema !== undefined) {
      const schemaError = responseSchemaError(node.config.response_schema);
      if (schemaError)
        issues.push({
          nodeKey: node.key,
          message: `"${node.name}" possui contrato inválido: ${schemaError}`,
        });
    }
    if (
      node.type === "transform" &&
      !validDeclarativeOperations(node.config.operations)
    )
      issues.push({
        nodeKey: node.key,
        message: `As operações declarativas de "${node.name}" são inválidas.`,
      });
    if (node.type === "variable" && Object.keys(node.config).length > 0) {
      if (
        !["set", "get"].includes(configText(node.config.action) || "set") ||
        !["execution", "loop", "card"].includes(
          configText(node.config.namespace) || "execution",
        ) ||
        !/^[A-Za-z_-][A-Za-z0-9_-]{0,63}$/.test(configText(node.config.name))
      )
        issues.push({
          nodeKey: node.key,
          message: `Defina operação, namespace e nome válidos para "${node.name}".`,
        });
    }
    if (node.type === "condition" && node.config.branches !== undefined) {
      const branches = node.config.branches;
      const branchPorts = Array.isArray(branches)
        ? branches.map((raw) =>
            raw && typeof raw === "object"
              ? configText((raw as Record<string, unknown>).port)
              : "",
          )
        : [];
      if (
        !Array.isArray(branches) ||
        branches.length < 1 ||
        branches.length > 8 ||
        new Set(branchPorts).size !== branchPorts.length ||
        branches.some((raw) => {
          if (!raw || typeof raw !== "object") return true;
          const branch = raw as Record<string, unknown>;
          return (
            !/^match_[1-8]$/.test(configText(branch.port)) ||
            ![
              "equals",
              "not_equals",
              "exists",
              "contains",
              "gt",
              "gte",
              "lt",
              "lte",
            ].includes(configText(branch.operator)) ||
            (branch.path !== undefined &&
              branch.path !== "" &&
              !validDataPath(branch.path))
          );
        })
      )
        issues.push({
          nodeKey: node.key,
          message: `Os ramos declarativos de "${node.name}" são inválidos.`,
        });
    }
    if (node.type === "merge") {
      const mode = configText(node.config.mode) || "all";
      if (
        !["all", "any", "quorum"].includes(mode) ||
        (mode === "quorum" && !positiveInteger(node.config.quorum)) ||
        (node.config.timeout_ms !== undefined &&
          (!positiveInteger(node.config.timeout_ms) ||
            (node.config.timeout_ms as number) > 60000))
      )
        issues.push({
          nodeKey: node.key,
          message: `A política de join de "${node.name}" é inválida.`,
        });
    }
    if (
      node.type === "workflow" &&
      !positiveInteger(node.config.workflow_version_id)
    )
      issues.push({
        nodeKey: node.key,
        message: `Selecione uma versão publicada para "${node.name}".`,
      });
    if (node.type === "subpipeline") {
      const inputs = subpipelinePorts(node.config, "input_ports");
      const outputs = subpipelinePorts(node.config, "output_ports");
      if (
        inputs.length === 0 ||
        outputs.length === 0 ||
        !node.size ||
        node.size.width < 420 ||
        node.size.height < 180 ||
        [...inputs, ...outputs].some(
          (port) =>
            !port.key ||
            !port.label ||
            !port.contract ||
            /[:\s]/.test(port.key),
        )
      )
        issues.push({
          nodeKey: node.key,
          message: `"${node.name}" precisa de tamanho mínimo 420x180 e portas de entrada e saída válidas.`,
        });
      if (node.config.instances !== undefined) {
        const rawInstances = node.config.instances;
        const instances = subpipelineInstances(node.config);
        const keys = instances.map((instance) => instance.key);
        if (
          !Array.isArray(rawInstances) ||
          instances.length !== rawInstances.length ||
          instances.length === 0 ||
          !instances.some((instance) => instance.enabled) ||
          new Set(keys).size !== keys.length ||
          instances.some(
            (instance) =>
              !/^[a-zA-Z0-9_-]+$/.test(instance.key) ||
              !instance.name.trim() ||
              !instance.template.trim() ||
              !instance.review_contract_key.trim() ||
              !Number.isInteger(instance.review_contract_version) ||
              instance.review_contract_version < 1,
          )
        )
          issues.push({
            nodeKey: node.key,
            message: `"${node.name}" precisa de ao menos uma revisão ativa com chave, prompt e contrato versionado válidos.`,
          });
      }
    }
    const publishedKey = configText(node.config.published_output_key);
    const publishedPort = configText(node.config.published_output_port);
    if (
      Boolean(publishedKey) !== Boolean(publishedPort) ||
      (publishedPort &&
        !catalogCard?.outputs.some((port) => port.key === publishedPort))
    )
      issues.push({
        nodeKey: node.key,
        message: `Complete a saída publicada de "${node.name}".`,
      });
    if (
      node.type === "fetch" &&
      (node.config.medium_severity_event !== undefined ||
        node.config.allow_autonomous_rejection !== undefined)
    )
      issues.push({
        nodeKey: node.key,
        message: `Mova a política de publicação de "${node.name}" para o card Publicar.`,
      });
    if (node.type === "publish") {
      const mediumEvent = node.config.medium_severity_event;
      if (
        mediumEvent !== undefined &&
        !["COMMENT", "REQUEST_CHANGES"].includes(configText(mediumEvent))
      )
        issues.push({
          nodeKey: node.key,
          message: `Selecione um evento válido para severidade média em "${node.name}".`,
        });
      if (
        node.config.allow_autonomous_rejection !== undefined &&
        typeof node.config.allow_autonomous_rejection !== "boolean"
      )
        issues.push({
          nodeKey: node.key,
          message: `A rejeição autônoma de "${node.name}" deve ser booleana.`,
        });
    }
    if (["fetch", "publish"].includes(node.type)) {
      if (
        ["owner", "repo", "pull_request"].some((key) =>
          Object.prototype.hasOwnProperty.call(node.config, key),
        )
      )
        issues.push({
          nodeKey: node.key,
          message: `Remova as coordenadas fixas de PR de "${node.name}".`,
        });
      if (!nonEmptyText(node.config.integration))
        issues.push({
          nodeKey: node.key,
          message: `Configure a conexão de "${node.name}".`,
        });
    }
    if (
      node.type === "trigger" &&
      !["manual", "api", "webhook"].includes(
        configText(node.config.mode) || "manual",
      )
    )
      issues.push({
        nodeKey: node.key,
        message: `Selecione um modo de trigger válido em "${node.name}".`,
      });
    if (
      node.type === "trigger" &&
      node.config.published_input_fields !== undefined
    ) {
      const fields = node.config.published_input_fields;
      const keys = Array.isArray(fields)
        ? fields.map((field) =>
            field && typeof field === "object"
              ? configText((field as Record<string, unknown>).key)
              : "",
          )
        : [];
      if (
        !Array.isArray(fields) ||
        fields.length === 0 ||
        new Set(keys).size !== keys.length ||
        fields.some(
          (field) =>
            !field ||
            typeof field !== "object" ||
            !nonEmptyText((field as Record<string, unknown>).key) ||
            !knownContracts.has(
              configText((field as Record<string, unknown>).contract),
            ),
        )
      )
        issues.push({
          nodeKey: node.key,
          message: `A interface de entrada publicada por "${node.name}" é inválida.`,
        });
    }
    if (
      node.type === "loop" &&
      (!positiveInteger(node.config.max_iterations) ||
        (node.config.concurrency !== undefined &&
          (!Number.isInteger(node.config.concurrency) ||
            (node.config.concurrency as number) < 1 ||
            (node.config.concurrency as number) > 4)))
    )
      issues.push({
        nodeKey: node.key,
        message: `"${node.name}" requer máximo de iterações positivo e concorrência entre 1 e 4.`,
      });
    if (
      node.type === "group" &&
      (!positiveInteger(node.config.max_files) ||
        !positiveInteger(node.config.max_characters))
    )
      issues.push({
        nodeKey: node.key,
        message: `"${node.name}" requer limites positivos de arquivos e caracteres.`,
      });
    if (node.type === "semantic_units") {
      const boundedInteger = (
        key: string,
        minimum: number,
        maximum: number,
      ) => {
        const value = node.config[key];
        return (
          value === undefined ||
          (Number.isInteger(value) &&
            (value as number) >= minimum &&
            (value as number) <= maximum)
        );
      };
      if (
        !boundedInteger("max_units", 1, 1000) ||
        !boundedInteger("max_characters", 1000, 200000) ||
        !boundedInteger("context_lines", 0, 20)
      )
        issues.push({
          nodeKey: node.key,
          message: `"${node.name}" possui limites inválidos para unidades semânticas.`,
        });
    }
    if (node.type === "cache") {
      const mode = configText(node.config.mode);
      if (
        !nonEmptyText(node.config.key) ||
        !["read", "write", "delete"].includes(mode) ||
        (mode === "write" &&
          (!positiveInteger(node.config.ttl_seconds) ||
            (node.config.ttl_seconds as number) > 86400))
      )
        issues.push({
          nodeKey: node.key,
          message: `"${node.name}" requer chave, operação válida e TTL de 1 a 86400 para gravação.`,
        });
    }
  }
  for (const node of definition.nodes) {
    if (!node.parent_key) continue;
    const parent = definition.nodes.find(
      (candidate) => candidate.key === node.parent_key,
    );
    if (!parent || parent.type !== "subpipeline" || node.type === "subpipeline")
      issues.push({
        nodeKey: node.key,
        message: `O card "${node.name}" referencia uma subpipeline pai inválida.`,
      });
  }
  const edgeKeys = new Set<string>();
  for (const edge of definition.edges) {
    const source = definition.nodes.find((node) => node.key === edge.from_node);
    const target = definition.nodes.find((node) => node.key === edge.to_node);
    const sourceCard = source && cardByType.get(source.type);
    const targetCard = target && cardByType.get(target.type);
    const sourcePorts =
      source?.type === "subpipeline"
        ? [
            ...subpipelinePorts(source.config, "input_ports").map((port) => ({
              ...port,
              key: `entry:${port.key}`,
            })),
            ...subpipelinePorts(source.config, "output_ports").map((port) => ({
              ...port,
              key: `exit:${port.key}`,
            })),
          ]
        : sourceCard &&
          (sourceCard.error_output && source?.config.on_error === "route"
            ? [...sourceCard.outputs, sourceCard.error_output]
            : sourceCard.outputs);
    const targetPorts =
      target?.type === "subpipeline"
        ? [
            ...subpipelinePorts(target.config, "input_ports").map((port) => ({
              ...port,
              key: `entry:${port.key}`,
            })),
            ...subpipelinePorts(target.config, "output_ports").map((port) => ({
              ...port,
              key: `exit:${port.key}`,
            })),
          ]
        : targetCard?.inputs;
    const output = sourcePorts?.find((port) => port.key === edge.from_port);
    const input = targetPorts?.find((port) => port.key === edge.to_port);
    if (!edge.key || !source || !target || !output || !input) {
      issues.push({
        message: `A conexão "${edge.key || "sem chave"}" referencia um card ou porta inválida.`,
      });
      continue;
    }
    const boundaryAllowed =
      source.type !== "subpipeline" && target.type !== "subpipeline"
        ? source.parent_key === target.parent_key
        : source.type === "subpipeline" && target.type !== "subpipeline"
          ? edge.from_port.startsWith("entry:")
            ? target.parent_key === source.key
            : edge.from_port.startsWith("exit:") && !target.parent_key
          : target.type === "subpipeline" && source.type !== "subpipeline"
            ? edge.to_port.startsWith("entry:")
              ? !source.parent_key
              : edge.to_port.startsWith("exit:") &&
                source.parent_key === target.key
            : false;
    if (!boundaryAllowed)
      issues.push({
        message: `A conexão "${edge.key}" atravessa uma subpipeline sem usar corretamente sua fronteira.`,
      });
    if (edgeKeys.has(edge.key))
      issues.push({
        message: `A chave da conexão "${edge.key}" está duplicada.`,
      });
    if (
      output.contract !== "any" &&
      input.contract !== "any" &&
      output.contract !== input.contract
    )
      issues.push({
        message: `A conexão "${edge.key}" usa contratos incompatíveis.`,
      });
    edgeKeys.add(edge.key);
  }
  for (const group of definition.nodes.filter(
    (node) => node.type === "subpipeline",
  )) {
    const boundaryPorts = [
      ...subpipelinePorts(group.config, "input_ports").map(
        (port) => `entry:${port.key}`,
      ),
      ...subpipelinePorts(group.config, "output_ports").map(
        (port) => `exit:${port.key}`,
      ),
    ];
    for (const port of boundaryPorts) {
      const incoming = definition.edges.filter(
        (edge) => edge.to_node === group.key && edge.to_port === port,
      ).length;
      const outgoing = definition.edges.filter(
        (edge) => edge.from_node === group.key && edge.from_port === port,
      ).length;
      if ((incoming === 0) !== (outgoing === 0))
        issues.push({
          nodeKey: group.key,
          message: `Complete os dois lados da porta "${port.replace(/^[^:]+:/, "")}" em "${group.name}".`,
        });
    }
  }
  for (const node of definition.nodes) {
    const card = cardByType.get(node.type);
    if (!card) continue;
    if (node.type === "merge" && configText(node.config.mode) === "quorum") {
      const incoming = definition.edges.filter(
        (edge) => edge.to_node === node.key && edge.to_port === "inputs",
      ).length;
      if ((node.config.quorum as number) > incoming)
        issues.push({
          nodeKey: node.key,
          message: `O quórum de "${node.name}" excede suas ${incoming} entradas.`,
        });
    }
    for (const input of card.inputs.filter((port) => port.required)) {
      if (
        !definition.edges.some(
          (edge) => edge.to_node === node.key && edge.to_port === input.key,
        )
      )
        issues.push({
          nodeKey: node.key,
          message: `Conecte a entrada obrigatória "${input.label}" do card "${node.name}".`,
        });
    }
    if (
      node.type !== "error_control" &&
      configText(node.config.on_error) === "route" &&
      !definition.edges.some(
        (edge) => edge.from_node === node.key && edge.from_port === "error",
      )
    )
      issues.push({
        nodeKey: node.key,
        message: `Conecte a rota de erro do card "${node.name}".`,
      });
  }
  return issues;
}

export function removeSelectedElements(
  nodes: Node<CardData>[],
  edges: Edge[],
  nodeIDs: Iterable<string>,
  edgeIDs: Iterable<string>,
) {
  const selectedNodes = new Set(nodeIDs);
  for (const node of nodes)
    if (node.parentId && selectedNodes.has(node.parentId))
      selectedNodes.add(node.id);
  const selectedEdges = new Set(edgeIDs);
  const remainingNodes = nodes.filter((node) => !selectedNodes.has(node.id));
  const removedEdges = edges.filter(
    (edge) =>
      selectedEdges.has(edge.id) ||
      selectedNodes.has(edge.source) ||
      selectedNodes.has(edge.target),
  );
  return {
    nodes: remainingNodes,
    edges: edges.filter((edge) => !removedEdges.includes(edge)),
    removedEdges: removedEdges.length,
  };
}

/**
 * React Flow can report the current selection after a controlled graph update.
 * Avoid turning an identical report into another React state update.
 */
export const selectionHasChanged = (
  current: readonly string[],
  next: readonly string[],
) =>
  current.length !== next.length ||
  current.some((id, index) => id !== next[index]);

export function parseWorkflowExport(
  text: string,
  cards: CardType[],
): { envelope?: WorkflowExportEnvelope; error?: string } {
  try {
    const candidate = JSON.parse(text) as WorkflowExportEnvelope;
    if (
      candidate?.format !== "forgereview.workflow" ||
      candidate.version !== 1 ||
      !candidate.definition
    )
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
  const nodes = definition.nodes.map((node) => ({
    ...node,
    key: uniqueKey(node.key, nodeKeys),
    config: { ...node.config },
  }));
  const remappedNodes = new Map(
    definition.nodes.map((node, index) => [node.key, nodes[index].key]),
  );
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

export const workflowExportEnvelope = (
  definition: WorkflowDefinition,
): WorkflowExportEnvelope => ({
  format: "forgereview.workflow",
  version: 1,
  definition,
});

export function legacyReviewTemplate(
  cards: CardType[],
): { nodes: Node<CardData>[]; edges: Edge[] } | undefined {
  const types = [
    "trigger",
    "fetch",
    "filter",
    "semantic_units",
    "loop",
    "subpipeline",
    "template",
    "model",
    "validate",
    "candidate_validator",
    "response_filter",
    "merge",
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
  const edge = (
    id: string,
    source: string,
    sourceHandle: string,
    target: string,
    targetHandle: string,
  ): Edge => ({
    id,
    source,
    sourceHandle,
    target,
    targetHandle,
    animated: false,
  });
  const nodes: Node<CardData>[] = [
    node("trigger", "trigger", "Webhook Gitea", 40, 650, { mode: "webhook" }),
    node("fetch", "fetch", "Buscar dados do PR", 315, 650, { integration: "" }),
    node("filter", "filter", "Filtrar arquivos", 610, 650, {
      include_extensions: [".go", ".ts", ".tsx", ".php"],
      ignore_generated: true,
    }),
    node(
      "semantic-units",
      "semantic_units",
      "Criar unidades semânticas",
      900,
      650,
      {
        max_units: 200,
        max_characters: 50000,
        context_lines: 4,
      },
    ),
    node("loop", "loop", "Revisar cada unidade", 1190, 650, {
      max_iterations: 200,
      concurrency: 1,
      on_error: "fail",
    }),
  ];
  const edges: Edge[] = [
    edge("trigger-fetch", "trigger", "out-event", "fetch", "in-event"),
    edge("fetch-filter", "fetch", "out-files", "filter", "in-files"),
    edge(
      "filter-semantic-units",
      "filter",
      "out-files",
      "semantic-units",
      "in-files",
    ),
    edge(
      "semantic-units-loop",
      "semantic-units",
      "out-units",
      "loop",
      "in-items",
    ),
  ];
  reviewerGroupSpecs.forEach((reviewer, index) => {
    const groupKey = `reviewer-${reviewer.key}`;
    const groupY = 20 + index * 220;
    const templateKey = `template-${reviewer.key}`;
    const modelKey = `model-${reviewer.key}`;
    const validateKey = `validate-${reviewer.key}`;
    const validatorKey = `candidate-validator-${reviewer.key}`;
    const filterKey = `response-filter-${reviewer.key}`;
    const group = node(
      groupKey,
      "subpipeline",
      `Reviewer · ${reviewer.name}`,
      1450,
      groupY,
      {
        input_ports: [
          {
            key: "context",
            label: "Unidade",
            contract: "any",
            required: true,
          },
        ],
        output_ports: [
          {
            key: "comments",
            label: "Comentários",
            contract: "list",
            required: true,
          },
        ],
      },
    );
    group.type = "subpipeline";
    group.dragHandle = ".drag-handle";
    group.width = 900;
    group.height = 190;
    group.initialWidth = 900;
    group.initialHeight = 190;
    group.style = { width: 900, height: 190 };
    group.data.inputs = [
      {
        key: "entry:context",
        label: "Unidade",
        contract: "any",
        required: true,
      },
      {
        key: "exit:comments",
        label: "Comentários",
        contract: "list",
        required: true,
      },
    ];
    group.data.outputs = [...group.data.inputs];
    const child = (item: Node<CardData>) => ({
      ...item,
      parentId: groupKey,
      extent: "parent" as const,
      expandParent: false,
    });
    nodes.push(
      group,
      child(
        node(templateKey, "template", `Tarefa: ${reviewer.name}`, 28, 54, {
          template: reviewer.prompt,
          review_contract_key: `review.${reviewer.key}`,
          review_contract_version: 2,
        }),
      ),
      child(
        node(modelKey, "model", `Reviewer: ${reviewer.name}`, 198, 54, {
          model_profile: "",
          max_tokens: 2000,
          retry_limit: 0,
          retry_delay_ms: 0,
        }),
      ),
      child(
        node(
          validateKey,
          "validate",
          `Validar contrato: ${reviewer.name}`,
          368,
          54,
          {
            validate_paths: true,
          },
        ),
      ),
      child(
        node(
          validatorKey,
          "candidate_validator",
          `Confirmar: ${reviewer.name}`,
          538,
          54,
          {
            model_profile: "",
            max_tokens: 300,
            temperature: 0,
            timeout_seconds: 120,
          },
        ),
      ),
      child(
        node(
          filterKey,
          "response_filter",
          `Filtrar: ${reviewer.name}`,
          708,
          54,
          {
            minimum_severity: "medium",
          },
        ),
      ),
    );
    edges.push(
      edge(
        `loop-${groupKey}`,
        "loop",
        "out-item",
        groupKey,
        "in-entry:context",
      ),
      edge(
        `${groupKey}-${templateKey}`,
        groupKey,
        "out-entry:context",
        templateKey,
        "in-context",
      ),
      edge(
        `${groupKey}-${validateKey}`,
        groupKey,
        "out-entry:context",
        validateKey,
        "in-files",
      ),
      edge(
        `${groupKey}-${validatorKey}`,
        groupKey,
        "out-entry:context",
        validatorKey,
        "in-files",
      ),
      edge(
        `${templateKey}-${modelKey}`,
        templateKey,
        "out-prompt",
        modelKey,
        "in-prompt",
      ),
      edge(
        `${modelKey}-${validateKey}`,
        modelKey,
        "out-response",
        validateKey,
        "in-response",
      ),
      edge(
        `${validateKey}-${validatorKey}`,
        validateKey,
        "out-valid",
        validatorKey,
        "in-candidates",
      ),
      edge(
        `${validatorKey}-${filterKey}`,
        validatorKey,
        "out-confirmed",
        filterKey,
        "in-response",
      ),
      edge(
        `${filterKey}-${groupKey}`,
        filterKey,
        "out-comments",
        groupKey,
        "in-exit:comments",
      ),
      edge(
        `${groupKey}-merge-reviewers`,
        groupKey,
        "out-exit:comments",
        "merge-reviewers",
        "in-inputs",
      ),
    );
  });
  nodes.push(
    node("merge-reviewers", "merge", "Unir reviewers", 2430, 650, {
      mode: "all",
    }),
    node("consolidate", "consolidate", "Consolidar reviewers", 2710, 650),
    node("format", "format", "Formatar review", 2990, 650),
    node("publish", "publish", "Publicar no Gitea", 3270, 650, {
      integration: "",
      medium_severity_event: "COMMENT",
      allow_autonomous_rejection: false,
    }),
  );
  edges.push(
    edge(
      "loop-consolidate",
      "loop",
      "out-results",
      "consolidate",
      "in-comments",
    ),
    edge(
      "consolidate-format",
      "consolidate",
      "out-review",
      "format",
      "in-review",
    ),
    edge(
      "format-publish",
      "format",
      "out-formatted",
      "publish",
      "in-formatted_review",
    ),
    edge(
      "fetch-publish-target",
      "fetch",
      "out-pull_request",
      "publish",
      "in-pull_request",
    ),
  );
  return { nodes, edges };
}

export function reviewTemplate(
  cards: CardType[],
): { nodes: Node<CardData>[]; edges: Edge[] } | undefined {
  const required = [
    "trigger",
    "fetch",
    "filter",
    "semantic_units",
    "loop",
    "subpipeline",
    "template",
    "model",
    "validate",
    "candidate_validator",
    "response_filter",
    "merge",
    "consolidate",
    "format",
    "publish",
  ];
  if (required.some((type) => !cards.some((card) => card.key === type)))
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
  const edge = (
    id: string,
    source: string,
    sourceHandle: string,
    target: string,
    targetHandle: string,
  ): Edge => ({
    id,
    source,
    sourceHandle,
    target,
    targetHandle,
    animated: false,
  });
  const nodes = [
    node("trigger", "trigger", "Webhook Gitea", 40, 300, { mode: "webhook" }),
    node("fetch", "fetch", "Buscar dados do PR", 315, 300, {
      integration: "",
    }),
    node("filter", "filter", "Filtrar arquivos", 610, 300, {
      include_extensions: [".go", ".ts", ".tsx", ".php"],
      ignore_generated: true,
    }),
    node(
      "semantic-units",
      "semantic_units",
      "Criar unidades semânticas",
      900,
      300,
      { max_units: 200, max_characters: 50000, context_lines: 4 },
    ),
    node("loop", "loop", "Revisar cada unidade", 1190, 300, {
      max_iterations: 200,
      concurrency: 1,
      on_error: "fail",
    }),
  ];
  const edges = [
    edge("trigger-fetch", "trigger", "out-event", "fetch", "in-event"),
    edge("fetch-filter", "fetch", "out-files", "filter", "in-files"),
    edge(
      "filter-semantic-units",
      "filter",
      "out-files",
      "semantic-units",
      "in-files",
    ),
    edge(
      "semantic-units-loop",
      "semantic-units",
      "out-units",
      "loop",
      "in-items",
    ),
  ];
  const recipe = node(
    "review-recipe",
    "subpipeline",
    `Receita de revisão · ${reviewerGroupSpecs.length} instâncias`,
    1450,
    90,
    {
      input_ports: [
        { key: "context", label: "Unidade", contract: "any", required: true },
      ],
      output_ports: [
        {
          key: "comments",
          label: "Comentários",
          contract: "list",
          required: true,
        },
      ],
      instances: reviewerGroupSpecs.map((reviewer) => ({
        key: reviewer.key,
        name: reviewer.name,
        enabled: true,
        template: reviewer.prompt,
        review_contract_key: `review.${reviewer.key}`,
        review_contract_version: 2,
        model_profile: "",
        validator_model_profile: "",
        minimum_severity: "medium",
      })),
    },
  );
  recipe.type = "subpipeline";
  recipe.dragHandle = ".drag-handle";
  recipe.width = 670;
  recipe.height = 470;
  recipe.initialWidth = 670;
  recipe.initialHeight = 470;
  recipe.style = { width: 670, height: 470 };
  recipe.data.inputs = [
    { key: "entry:context", label: "Unidade", contract: "any", required: true },
    {
      key: "exit:comments",
      label: "Comentários",
      contract: "list",
      required: true,
    },
  ];
  recipe.data.outputs = [...recipe.data.inputs];
  const child = (item: Node<CardData>) => ({
    ...item,
    parentId: "review-recipe",
    extent: "parent" as const,
    expandParent: false,
  });
  nodes.push(
    recipe,
    child(
      node("review-template", "template", "Prompt + contrato", 40, 85, {
        template: reviewerGroupSpecs[0].prompt,
        review_contract_key: "review.security",
        review_contract_version: 2,
      }),
    ),
    child(
      node("review-model", "model", "Executar modelo", 250, 65, {
        model_profile: "",
        max_tokens: 2000,
        retry_limit: 0,
        retry_delay_ms: 0,
      }),
    ),
    child(
      node("review-validate", "validate", "Validar contrato", 455, 105, {
        validate_paths: true,
      }),
    ),
    child(
      node(
        "review-confirm",
        "candidate_validator",
        "Confirmar achados",
        350,
        285,
        {
          model_profile: "",
          max_tokens: 300,
          temperature: 0,
          timeout_seconds: 120,
        },
      ),
    ),
    child(
      node("review-filter", "response_filter", "Aplicar filtro", 115, 285, {
        minimum_severity: "medium",
      }),
    ),
    node("merge-reviewers", "merge", "Unir revisões", 2210, 300, {
      mode: "all",
    }),
    node("consolidate", "consolidate", "Consolidar reviewers", 2490, 300),
    node("format", "format", "Formatar review", 2770, 300),
    node("publish", "publish", "Publicar no Gitea", 3050, 300, {
      integration: "",
      medium_severity_event: "COMMENT",
      allow_autonomous_rejection: false,
    }),
  );
  edges.push(
    edge(
      "loop-review-recipe",
      "loop",
      "out-item",
      "review-recipe",
      "in-entry:context",
    ),
    edge(
      "recipe-template",
      "review-recipe",
      "out-entry:context",
      "review-template",
      "in-context",
    ),
    edge(
      "recipe-validate-files",
      "review-recipe",
      "out-entry:context",
      "review-validate",
      "in-files",
    ),
    edge(
      "recipe-confirm-files",
      "review-recipe",
      "out-entry:context",
      "review-confirm",
      "in-files",
    ),
    edge(
      "template-model",
      "review-template",
      "out-prompt",
      "review-model",
      "in-prompt",
    ),
    edge(
      "model-validate",
      "review-model",
      "out-response",
      "review-validate",
      "in-response",
    ),
    edge(
      "validate-confirm",
      "review-validate",
      "out-valid",
      "review-confirm",
      "in-candidates",
    ),
    edge(
      "confirm-filter",
      "review-confirm",
      "out-confirmed",
      "review-filter",
      "in-response",
    ),
    edge(
      "filter-recipe",
      "review-filter",
      "out-comments",
      "review-recipe",
      "in-exit:comments",
    ),
    edge(
      "recipe-merge",
      "review-recipe",
      "out-exit:comments",
      "merge-reviewers",
      "in-inputs",
    ),
    edge(
      "loop-consolidate",
      "loop",
      "out-results",
      "consolidate",
      "in-comments",
    ),
    edge(
      "consolidate-format",
      "consolidate",
      "out-review",
      "format",
      "in-review",
    ),
    edge(
      "format-publish",
      "format",
      "out-formatted",
      "publish",
      "in-formatted_review",
    ),
    edge(
      "fetch-publish-target",
      "fetch",
      "out-pull_request",
      "publish",
      "in-pull_request",
    ),
  );
  return { nodes, edges };
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
