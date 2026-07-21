"use client";

import type { AdminRequest } from "@/lib/admin-client";
import type {
  ReviewSettingsProfile,
  ReviewSettingsStageType,
} from "@/lib/contracts";
import {
  defaultRule,
  ruleSummary,
  schemaFields,
  type PipelineStage,
  type PipelineTransition,
  type Rule,
  type WorkflowCatalog,
  type WorkflowCatalogSchema,
} from "./pipeline-types";
import { RuleBuilder } from "./rule-builder";
import {
  Background,
  Controls,
  Handle,
  Position,
  ReactFlow,
  addEdge,
  useEdgesState,
  useNodesState,
  type Connection,
  type Edge,
  type Node,
  type NodeProps,
  type ReactFlowInstance,
} from "@xyflow/react";
import {
  AlertCircle,
  Braces,
  CircleAlert,
  GitBranch,
  Hand,
  MousePointer2,
  PanelLeftOpen,
  Plus,
  RotateCcw,
  Save,
  Send,
  Star,
  Trash2,
  Webhook,
  X,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

type Stage = PipelineStage;
type Transition = PipelineTransition;
type Trigger = {
  source: "webhook" | "api" | "manual";
  enabled: boolean;
  target_stage_key: string;
  config: Record<string, unknown>;
};
type Version = {
  id: number;
  version: number;
  status: string;
  profile_id?: number;
  scheduler_max_runs: number;
  stages: Stage[];
  transitions: Transition[];
  triggers?: Trigger[] | null;
};
type Pipeline = {
  id: number;
  name: string;
  description: string;
  profile_id?: number;
  is_default: boolean;
  versions: Version[];
};
type RuntimeEvent = {
  status?: string;
  percent?: number;
  message?: string;
  timestamp?: string;
};
type StudioNodeData = {
  stage: Stage;
  update: (patch: Partial<Stage>) => void;
  remove: () => void;
  select: () => void;
  openLogs: () => void;
  editable: boolean;
  removable: boolean;
  runtime?: RuntimeEvent;
  active?: boolean;
};
type EntrypointNodeData = {
  trigger: Trigger;
  editable: boolean;
  updateSource: (source: Trigger["source"]) => void;
  toggle: () => void;
};

const triggerLabels = {
  webhook: "Webhook Gitea",
  api: "POST API",
  manual: "Disparo manual",
};
const triggerSources = Object.keys(triggerLabels) as Trigger["source"][];
const defaultTriggers = (targetStageKey = "") =>
  triggerSources.map((source, index) => ({
    source,
    enabled: true,
    target_stage_key: targetStageKey,
    config: { x: -360, y: (index - 1) * 145 },
  }));
const normalizeTriggers = (items: Trigger[]) =>
  triggerSources.map((source) => ({
    source,
    enabled: items.find((item) => item.source === source)?.enabled ?? false,
    target_stage_key:
      items.find((item) => item.source === source)?.target_stage_key ?? "",
    config: items.find((item) => item.source === source)?.config ?? {},
  }));
const businessConditions = [
  "always",
  "has_findings",
  "no_findings",
  "partial_result",
  "confidence_below_threshold",
];
const technicalConditions = ["error", "timeout", "contract_invalid"];
const processorGroups = ["llm", "rule_filter", "transform_merge", "system"] as const;
const processorGroupLabels = {
  llm: "LLM",
  rule_filter: "Rule filter",
  transform_merge: "Transform / merge",
  system: "Sistema",
};
const emptyWorkflowCatalog: WorkflowCatalog = {
  contracts: [],
  processors: [],
  entrypoints: [],
  route_modes: ["all_matches", "first_match"],
  join_modes: [{ key: "each_arrival", executable: true }],
};
const defaultPositions = [
  { x: 0, y: 0 },
  { x: 400, y: 0 },
  { x: 800, y: 0 },
  { x: 1200, y: 280 },
  { x: 800, y: 430 },
  { x: 400, y: 430 },
  { x: 0, y: 430 },
];
const entrypointPositions = [
  { x: -450, y: -150 },
  { x: -450, y: 40 },
  { x: -450, y: 230 },
];
const defaultSchedulerMaxRuns = 256;
const maxSchedulerMaxRuns = 10000;

function contractColor(key?: string) {
  if (key?.includes("findings")) return "#46c892";
  if (key?.includes("review")) return "#ab7cff";
  if (key?.includes("diff")) return "#4d91ff";
  if (key?.includes("error")) return "#e25f67";
  return "#e6a34e";
}
function portSide(stage: Stage, kind: "input" | "output") {
  const configured = stage.config?.[`${kind}_side`];
  if (configured === "left" || configured === "right") return configured;
  if (
    kind === "output" &&
    ["consolidator", "verification", "formatting"].includes(
      stage.executor_key || "",
    )
  )
    return "left";
  if (
    kind === "input" &&
    ["verification", "formatting", "publication"].includes(
      stage.executor_key || "",
    )
  )
    return "right";
  return kind === "input" ? "left" : "right";
}
function handlePosition(side: string) {
  return side === "left" ? Position.Left : Position.Right;
}
function stagePercent(event?: RuntimeEvent) {
  return ["done", "concluido", "concluído", "completed"].includes(
    event?.status || "",
  )
    ? 100
    : (event?.percent ?? 0);
}
function processorKind(stage: Stage) {
  if (stage.executor_key === "rule_filter" || stage.executor_key === "transform_merge")
    return stage.executor_key;
  const kind = stage.processor_kind || (stage.use_llm ? "llm" : "system");
  return kind === "deterministic" ? "system" : kind;
}

function stageTypeProcessorKind(type: ReviewSettingsStageType) {
  if (type.executor_key === "rule_filter" || type.executor_key === "transform_merge")
    return type.executor_key;
  if (type.processor_kind) return type.processor_kind === "deterministic" ? "system" : type.processor_kind;
  return ["preparation", "publication", "error_log"].includes(type.executor_key)
    ? "system"
    : "llm";
}

function isTechnicalTransition(type: unknown) {
  return type === "failure" || type === "fallback";
}

function catalogFieldType(schema: WorkflowCatalogSchema) {
  return Array.isArray(schema.type)
    ? schema.type.find((type) => type !== "null")
    : schema.type;
}

function configValue(config: Record<string, unknown>, path: string[]) {
  return path.reduce<unknown>((value, key) =>
    value && typeof value === "object"
      ? (value as Record<string, unknown>)[key]
      : undefined, config);
}

function patchConfigValue(
  config: Record<string, unknown>,
  path: string[],
  value: unknown,
) {
  const updated = { ...config };
  let cursor = updated;
  path.forEach((key, index) => {
    if (index === path.length - 1) cursor[key] = value;
    else {
      const child = cursor[key];
      cursor[key] = child && typeof child === "object" && !Array.isArray(child)
        ? { ...(child as Record<string, unknown>) }
        : {};
      cursor = cursor[key] as Record<string, unknown>;
    }
  });
  return updated;
}

function schemaDefaults(schema?: WorkflowCatalogSchema): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(schema?.properties ?? {}).flatMap(([key, field]) => {
      if (field.default !== undefined) return [[key, field.default]];
      if (catalogFieldType(field) === "object") {
        const nested = schemaDefaults(field);
        return Object.keys(nested).length ? [[key, nested]] : [];
      }
      return field.enum?.length ? [[key, field.enum[0]]] : [];
    }),
  );
}

function meaningfulRule(rule: Rule): boolean {
  if (rule.operator === "all" || rule.operator === "any")
    return rule.rules.length > 0 && rule.rules.every(meaningfulRule);
  if (rule.operator === "not") return meaningfulRule(rule.rules[0]);
  if (rule.scope) {
    if (!rule.scope.path.trim()) return false;
    if (
      rule.scope.kind !== "count" &&
      (!rule.scope.rule || !meaningfulRule(rule.scope.rule))
    )
      return false;
  } else if (!rule.path?.trim()) return false;
  if (rule.operator === "exists") return true;
  if (Array.isArray(rule.value))
    return (
      rule.value.length > 0 &&
      rule.value.every((value) => typeof value !== "string" || value.trim() !== "")
    );
  return (
    rule.value !== undefined &&
    rule.value !== null &&
    (typeof rule.value !== "string" || rule.value.trim() !== "")
  );
}

function stageConfigForSave(stage: Stage, position: { x: number; y: number }) {
  const config = { ...stage.config };
  const kind = processorKind(stage);
  if (kind === "rule_filter" || kind === "transform_merge") {
    delete config.x;
    delete config.y;
    delete config.input_side;
    delete config.output_side;
    if (kind === "rule_filter" && Array.isArray(config.include_extensions)) {
      const extensions = config.include_extensions
        .filter((value): value is string => typeof value === "string")
        .map((value) => value.trim())
        .filter(Boolean);
      if (extensions.length) config.include_extensions = extensions;
      else delete config.include_extensions;
    }
    return config;
  }
  return { ...config, x: position.x, y: position.y };
}

function processorConfigSchema(
  processor?: WorkflowCatalog["processors"][number],
): WorkflowCatalogSchema | undefined {
  if (!processor) return undefined;
  const implemented: Record<string, Record<string, WorkflowCatalogSchema>> = {
    rule_filter: {
      include_extensions: {
        type: "array",
        title: "Extensões incluídas",
        description: "Lista separada por vírgulas, por exemplo .go, .ts.",
        items: { type: "string" },
        default: [],
      },
    },
    transform_merge: {
      operation: {
        type: "string",
        title: "Operação",
        enum: ["identity", "merge", "dedupe_findings"],
        default: "identity",
      },
    },
  };
  return {
    ...processor.config_schema,
    properties: {
      ...implemented[processor.key],
      ...processor.config_schema.properties,
    },
  };
}

function graphReaches(
  adjacency: Map<string, string[]>,
  current: string,
  target: string,
  seen = new Set<string>(),
): boolean {
  if (current === target) return true;
  if (seen.has(current)) return false;
  seen.add(current);
  return (adjacency.get(current) ?? []).some((next) =>
    graphReaches(adjacency, next, target, seen));
}

function ConfigSchemaFields({
  schema,
  config,
  onChange,
  path = [],
}: {
  schema?: WorkflowCatalogSchema;
  config: Record<string, unknown>;
  onChange: (config: Record<string, unknown>) => void;
  path?: string[];
}) {
  return Object.entries(schema?.properties ?? {}).map(([key, field]) => {
    if (["rule", "prompt_template", "model_id", "max_output_tokens"].includes(key))
      return null;
    const fieldPath = [...path, key];
    const value = configValue(config, fieldPath) ?? field.default;
    const label = field.title || key.replaceAll("_", " ");
    const type = catalogFieldType(field);
    if (type === "object" && field.properties)
      return (
        <fieldset className="studio-config-group" key={fieldPath.join(".")}>
          <legend>{label}</legend>
          <ConfigSchemaFields
            schema={field}
            config={config}
            onChange={onChange}
            path={fieldPath}
          />
        </fieldset>
      );
    if (type === "boolean")
      return (
        <label className="studio-checkbox" key={fieldPath.join(".")}>
          <input
            type="checkbox"
            checked={Boolean(value)}
            onChange={(event) =>
              onChange(patchConfigValue(config, fieldPath, event.target.checked))
            }
          />
          {label}
        </label>
      );
    if (type === "array" && catalogFieldType(field.items ?? {}) === "string")
      return (
        <label key={fieldPath.join(".")} title={field.description}>
          {label}
          <input
            value={Array.isArray(value) ? value.join(", ") : ""}
            onChange={(event) =>
              onChange(patchConfigValue(
                config,
                fieldPath,
                event.target.value
                  .split(",")
                  .map((item) => item.trim())
                  .filter(Boolean),
              ))
            }
          />
          {field.description && <small>{field.description}</small>}
        </label>
      );
    return (
      <label key={fieldPath.join(".")} title={field.description}>
        {label}
        {field.enum ? (
          <select
            value={String(value ?? field.enum[0] ?? "")}
            onChange={(event) =>
              onChange(patchConfigValue(
                config,
                fieldPath,
                field.enum?.find((option) => String(option) === event.target.value),
              ))
            }
          >
            {field.enum.map((option) => (
              <option key={String(option)} value={String(option)}>{String(option)}</option>
            ))}
          </select>
        ) : (
          <input
            type={type === "integer" || type === "number" ? "number" : "text"}
            min={field.minimum}
            max={field.maximum}
            value={String(value ?? "")}
            onChange={(event) =>
              onChange(patchConfigValue(
                config,
                fieldPath,
                type === "integer" || type === "number"
                  ? event.target.valueAsNumber
                  : event.target.value,
              ))
            }
          />
        )}
        {field.description && <small>{field.description}</small>}
      </label>
    );
  });
}
function triggerIcon(source: Trigger["source"]) {
  if (source === "api") return <Braces size={15} />;
  if (source === "manual") return <Hand size={15} />;
  return <Webhook size={15} />;
}

function EntrypointNode({ data }: NodeProps<Node<EntrypointNodeData>>) {
  return (
    <article
      className={`studio-entrypoint ${data.trigger.enabled ? "enabled" : "disabled"}`}
    >
      <header>
        <span>
          <i />
          Entrypoint
        </span>
        <small>{data.trigger.enabled ? "Ativo" : "Inativo"}</small>
      </header>
      <div className="studio-entrypoint-body">
        <span className="studio-entrypoint-icon">
          {triggerIcon(data.trigger.source)}
        </span>
        <label>
          Tipo de entrada
          <select
            className="nodrag"
            disabled={!data.editable}
            value={data.trigger.source}
            onChange={(event) =>
              data.updateSource(event.target.value as Trigger["source"])
            }
          >
            {Object.entries(triggerLabels).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
      </div>
      <button
        className="nodrag studio-entrypoint-state"
        disabled={!data.editable}
        onClick={data.toggle}
      >
        {data.trigger.enabled ? "Desativar entrada" : "Ativar entrada"}
      </button>
      <Handle
        id="entry-output"
        type="source"
        position={Position.Right}
        className="studio-port-handle"
        style={{ background: "#e6a34e" }}
      />
    </article>
  );
}

function StageFields({
  stage,
  onUpdate,
}: {
  stage: Stage;
  onUpdate: (patch: Partial<Stage>) => void;
}) {
  return (
    <div className="studio-fields">
      <label>
        Nome
        <input
          value={stage.name}
          onChange={(event) => onUpdate({ name: event.target.value })}
        />
      </label>
      {stage.use_llm ? (
        <>
          <label>
            Prompt
            <textarea
              value={stage.prompt_template}
              onChange={(event) =>
                onUpdate({ prompt_template: event.target.value })
              }
            />
          </label>
          <div>
            <label>
              Modelo ID
              <input
                type="number"
                value={stage.model_id ?? ""}
                onChange={(event) =>
                  onUpdate({
                    model_id: event.target.value
                      ? Number(event.target.value)
                      : undefined,
                  })
                }
              />
            </label>
            <label>
              Tokens
              <input
                type="number"
                min="0"
                value={stage.max_output_tokens}
                onChange={(event) =>
                  onUpdate({ max_output_tokens: Number(event.target.value) })
                }
              />
            </label>
          </div>
          <div>
            <label>
              Tentativas
              <input
                type="number"
                min="0"
                value={stage.retry_limit}
                onChange={(event) =>
                  onUpdate({ retry_limit: Number(event.target.value) })
                }
              />
            </label>
            <label>
              Timeout
              <input
                type="number"
                min="0"
                value={stage.timeout_seconds}
                onChange={(event) =>
                  onUpdate({ timeout_seconds: Number(event.target.value) })
                }
              />
            </label>
          </div>
        </>
      ) : (
        <div className="studio-deterministic-fields">
          <label>
            Executor
            <input
              disabled
              value={stage.executor_key || stage.stage_type_key}
            />
          </label>
          <label>
            Timeout
            <input
              type="number"
              min="0"
              value={stage.timeout_seconds}
              onChange={(event) =>
                onUpdate({ timeout_seconds: Number(event.target.value) })
              }
            />
          </label>
        </div>
      )}
    </div>
  );
}

function StudioNode({ data }: NodeProps<Node<StudioNodeData>>) {
  const s = data.stage;
  const inputSide = portSide(s, "input");
  const outputSide = portSide(s, "output");
  return (
    <article
      className={`studio-node ${s.executor_key === "error_log" ? "error" : ""}`}
      onClick={data.select}
    >
      <header>
        <span>
          <i />
          {s.name}
        </span>
        {data.editable && data.removable && (
          <button
            aria-label="Remover etapa"
            onClick={(event) => {
              event.stopPropagation();
              data.remove();
            }}
          >
            <Trash2 size={13} />
          </button>
        )}
      </header>
      <div
        className={`studio-ports ${inputSide === "right" && outputSide === "left" ? "reversed" : ""}`}
      >
        <span className={`input-port ${inputSide}`}>
          <Handle
            id={`input-${inputSide}`}
            type="target"
            position={handlePosition(inputSide)}
            className="studio-port-handle"
            style={{ background: contractColor(s.input_contract?.key) }}
          />
          <i style={{ background: contractColor(s.input_contract?.key) }} />
          {s.input_contract?.key || "Entrada inicial"}
        </span>
        <span className={`output-port ${outputSide}`}>
          {s.output_contract?.key || "Terminal"}
          <i style={{ background: contractColor(s.output_contract?.key) }} />
          <Handle
            id={`output-${outputSide}`}
            type="source"
            position={handlePosition(outputSide)}
            className="studio-port-handle"
            style={{ background: contractColor(s.output_contract?.key) }}
          />
        </span>
      </div>
      {data.editable && (
        <div className="studio-node-meta">
          <span>{s.stage_type_key}</span>
          <span>{s.use_llm ? "LLM" : "Determinística"}</span>
          {s.required && <span>Obrigatória</span>}
        </div>
      )}
      {data.editable ? (
        <StageFields stage={s} onUpdate={data.update} />
      ) : (
        <div className={`studio-run-log ${data.runtime?.status || "pending"}`}>
          <header>
            <span>
              {data.active
                ? "Em execução"
                : data.runtime?.status || "Aguardando"}
            </span>
            <span>
              <strong>{stagePercent(data.runtime)}%</strong>
              <button
                className="studio-log-button"
                onClick={(event) => {
                  event.stopPropagation();
                  data.openLogs();
                }}
                title="Abrir logs"
              >
                <AlertCircle size={15} />
              </button>
            </span>
          </header>
          <p>
            {data.runtime?.message || "Aguardando uma execução desta etapa."}
          </p>
          <div>
            <i style={{ width: `${stagePercent(data.runtime)}%` }} />
          </div>
        </div>
      )}
    </article>
  );
}
const nodeTypes = { studio: StudioNode, entrypoint: EntrypointNode };

function StageInspector({
  stage,
  editable,
  runtime,
  workflowCatalog,
  onUpdate,
}: {
  stage: Stage;
  editable: boolean;
  runtime?: RuntimeEvent;
  workflowCatalog: WorkflowCatalog;
  onUpdate: (patch: Partial<Stage>) => void;
}) {
  const processor = workflowCatalog.processors.find(
    (item) => item.key === processorKind(stage),
  );
  const joinModes = workflowCatalog.join_modes.length
    ? workflowCatalog.join_modes
    : emptyWorkflowCatalog.join_modes;
  const routeModes = workflowCatalog.route_modes.length
    ? workflowCatalog.route_modes
    : emptyWorkflowCatalog.route_modes;
  const schemaText = (contract?: Stage["input_contract"]) =>
    JSON.stringify(contract?.schema ?? {}, null, 2);
  if (!editable)
    return (
      <>
        <strong>Status da execução</strong>
        <div
          className={`studio-inspector-status ${runtime?.status || "pending"}`}
        >
          <header>
            <span>{runtime?.status || "Aguardando"}</span>
            <strong>{runtime?.percent ?? 0}%</strong>
          </header>
          <p>
            {runtime?.message ||
              "Esta etapa ainda não possui eventos nesta execução."}
          </p>
          <i style={{ width: `${runtime?.percent ?? 0}%` }} />
        </div>
        <section className="studio-readonly-config">
          <strong>Configuração</strong>
          <label>
            Etapa
            <input disabled value={stage.name} />
          </label>
          <label>
            Entrada
            <input
              disabled
              value={stage.input_contract?.key || "Entrada inicial"}
            />
          </label>
          <label>
            Saída
            <input disabled value={stage.output_contract?.key || "Terminal"} />
          </label>
          {stage.use_llm ? (
            <>
              <label>
                Modelo
                <input
                  disabled
                  value={
                    stage.model_id ? `#${stage.model_id}` : "Modelo do profile"
                  }
                />
              </label>
              <label>
                Tokens
                <input disabled value={stage.max_output_tokens} />
              </label>
              <label>
                Retry / timeout
                <input
                  disabled
                  value={`${stage.retry_limit} / ${stage.timeout_seconds}s`}
                />
              </label>
              <label>
                Prompt
                <textarea disabled value={stage.prompt_template} />
              </label>
            </>
          ) : (
            <>
              <label>
                Executor
                <input
                  disabled
                  value={stage.executor_key || stage.stage_type_key}
                />
              </label>
              <label>
                Timeout
                <input disabled value={`${stage.timeout_seconds}s`} />
              </label>
            </>
          )}
          <label>
            Schema completo de entrada
            <textarea className="studio-contract-schema" readOnly value={schemaText(stage.input_contract)} />
          </label>
          <label>
            Schema completo de saída
            <textarea className="studio-contract-schema" readOnly value={schemaText(stage.output_contract)} />
          </label>
        </section>
      </>
    );
  return (
    <>
      <strong>Editar etapa</strong>
      <label>
        Chave estável
        <input disabled value={stage.key} />
      </label>
      <label>
        Executor
        <input disabled value={stage.executor_key || stage.stage_type_key} />
      </label>
      <label>
        Processador
        <input disabled value={processorKind(stage)} />
      </label>
      <label>
        Contratos
        <input
          disabled
          value={`${stage.input_contract?.key || "entrada inicial"} → ${stage.output_contract?.key || "terminal"}`}
        />
      </label>
      <label>
        Obrigatoriedade
        <input disabled value={stage.required ? "Etapa obrigatória" : "Etapa opcional"} />
      </label>
      <label>
        Roteamento de saída
        <select
          value={stage.routing_mode ?? stage.route_mode ?? "all_matches"}
          onChange={(event) =>
            onUpdate({
              route_mode: event.target.value as Stage["route_mode"],
              routing_mode: event.target.value as Stage["routing_mode"],
            })
          }
        >
          {routeModes.map((mode) => (
            <option key={mode} value={mode}>
              {mode === "all_matches" ? "Todas as regras compatíveis" : "Primeira regra compatível"}
            </option>
          ))}
        </select>
      </label>
      <label>
        Junção de entradas
        <select
          value={stage.join_mode ?? "each_arrival"}
          onChange={(event) =>
            onUpdate({ join_mode: event.target.value as Stage["join_mode"] })
          }
        >
          {joinModes.map((mode) => (
            <option key={mode.key} value={mode.key} disabled={!mode.executable}>
              {mode.key === "each_arrival"
                ? "Executar a cada chegada"
                : mode.key === "any"
                  ? "Continuar com qualquer entrada"
                  : "Aguardar todas as entradas"}
              {!mode.executable ? " (indisponível)" : ""}
            </option>
          ))}
        </select>
      </label>
      <label>
        Nome
        <input
          value={stage.name}
          onChange={(event) => onUpdate({ name: event.target.value })}
        />
      </label>
      <label>
        Entrada
        <select
          value={portSide(stage, "input")}
          onChange={(event) =>
            onUpdate({
              config: { ...stage.config, input_side: event.target.value },
            })
          }
        >
          <option value="left">Esquerda</option>
          <option value="right">Direita</option>
        </select>
      </label>
      <label>
        Saída
        <select
          value={portSide(stage, "output")}
          onChange={(event) =>
            onUpdate({
              config: { ...stage.config, output_side: event.target.value },
            })
          }
        >
          <option value="left">Esquerda</option>
          <option value="right">Direita</option>
        </select>
      </label>
      {["planner", "consolidator", "verification", "formatting"].includes(
        stage.executor_key || "",
      ) && (
        <label>
          Modo de execução
          <select
            value={stage.use_llm ? "llm" : "deterministic"}
            onChange={(event) =>
              onUpdate({ use_llm: event.target.value === "llm" })
            }
          >
            <option value="llm">Modelo de IA</option>
            <option value="deterministic">Fallback determinístico</option>
          </select>
        </label>
      )}
      {stage.use_llm ? (
        <>
          <label>
            Modelo ID
            <input
              type="number"
              value={stage.model_id ?? ""}
              onChange={(event) =>
                onUpdate({
                  model_id: event.target.value
                    ? Number(event.target.value)
                    : undefined,
                })
              }
            />
          </label>
          <label>
            Tokens
            <input
              type="number"
              min="0"
              value={stage.max_output_tokens}
              onChange={(event) =>
                onUpdate({ max_output_tokens: Number(event.target.value) })
              }
            />
          </label>
          <label>
            Tentativas
            <input
              type="number"
              min="0"
              value={stage.retry_limit}
              onChange={(event) =>
                onUpdate({ retry_limit: Number(event.target.value) })
              }
            />
          </label>
          <label>
            Timeout (s)
            <input
              type="number"
              min="0"
              value={stage.timeout_seconds}
              onChange={(event) =>
                onUpdate({ timeout_seconds: Number(event.target.value) })
              }
            />
          </label>
          <label>
            Prompt
            <textarea
              value={stage.prompt_template}
              onChange={(event) =>
                onUpdate({ prompt_template: event.target.value })
              }
            />
          </label>
        </>
      ) : (
        <>
          <label>
            Executor
            <input
              disabled
              value={stage.executor_key || stage.stage_type_key}
            />
          </label>
          <label>
            Timeout (s)
            <input
              type="number"
              min="0"
              value={stage.timeout_seconds}
              onChange={(event) =>
                onUpdate({ timeout_seconds: Number(event.target.value) })
              }
            />
          </label>
        </>
      )}
      {processorKind(stage) === "rule_filter" && (
        <section className="studio-processor-config">
          <strong>Regra do filtro</strong>
          <RuleBuilder
            rule={stage.config.rule as Rule | undefined}
            schema={stage.input_contract?.schema}
            onChange={(rule) => onUpdate({ config: { ...stage.config, rule } })}
          />
        </section>
      )}
      {(processorKind(stage) === "rule_filter" ||
        processorKind(stage) === "transform_merge") && (
        <section className="studio-processor-config">
          <strong>Configuração controlada</strong>
          <ConfigSchemaFields
            schema={processorConfigSchema(processor)}
            config={stage.config}
            onChange={(config) => onUpdate({ config })}
          />
        </section>
      )}
      <label>
        Schema completo de entrada
        <textarea className="studio-contract-schema" readOnly value={schemaText(stage.input_contract)} />
      </label>
      <label>
        Schema completo de saída
        <textarea className="studio-contract-schema" readOnly value={schemaText(stage.output_contract)} />
      </label>
    </>
  );
}

export function PipelineManager({
  request,
  profiles,
  catalog,
  onAuthError,
  onSaved,
  readOnly = false,
  profileID: runtimeProfileID,
  onPipelineName,
  editRequest = 0,
  saveRequest = 0,
  publishRequest = 0,
  discardRequest = 0,
  onEditingChange,
  runtimeEvents = {},
  runtimeLogs = {},
  activeStage = "",
  manualApproval = false,
  onOpenLogs,
  onOpenPreReview,
}: {
  request: AdminRequest;
  profiles: ReviewSettingsProfile[];
  catalog: ReviewSettingsStageType[];
  onAuthError: () => void;
  onSaved: () => void;
  readOnly?: boolean;
  profileID?: number;
  onPipelineName?: (name: string) => void;
  editRequest?: number;
  saveRequest?: number;
  publishRequest?: number;
  discardRequest?: number;
  onEditingChange?: (editing: boolean) => void;
  runtimeEvents?: Record<string, RuntimeEvent>;
  runtimeLogs?: Record<string, RuntimeEvent[]>;
  activeStage?: string;
  manualApproval?: boolean;
  onOpenLogs?: (stage: string, name: string, events: RuntimeEvent[]) => void;
  onOpenPreReview?: () => void;
}) {
  const [pipelines, setPipelines] = useState<Pipeline[]>([]),
    [pipelineID, setPipelineID] = useState<number | null>(null),
    [version, setVersion] = useState<Version | null>(null),
    [nodes, setNodes, onNodesChange] = useNodesState<Node<StudioNodeData>>([]),
    [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]),
    [name, setName] = useState(""),
    [description, setDescription] = useState(""),
    [profileID, setProfileID] = useState(""),
    [schedulerMaxRuns, setSchedulerMaxRuns] = useState(defaultSchedulerMaxRuns),
    [triggers, setTriggers] = useState<Trigger[]>(defaultTriggers),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [editing, setEditing] = useState(false),
    [menuOpen, setMenuOpen] = useState(false),
    [flowInstance, setFlowInstance] = useState<ReactFlowInstance | null>(null),
    [selectedEdge, setSelectedEdge] = useState<string | null>(null),
    [selectedNode, setSelectedNode] = useState<string | null>(null),
    [statusOpen, setStatusOpen] = useState(false),
    [workflowCatalog, setWorkflowCatalog] = useState<WorkflowCatalog>(emptyWorkflowCatalog);
  const handledEditRequest = useRef(0);
  const current = pipelines.find((p) => p.id === pipelineID);
  const editable =
    editing || (!readOnly && (!version || version.status === "draft"));
  const load = useCallback(async () => {
    try {
      const loadedWorkflowCatalog = await request<WorkflowCatalog>(
        "review/workflow-catalog",
      );
      setWorkflowCatalog(loadedWorkflowCatalog);
      if (readOnly) {
        const item = await request<Pipeline>(
          `review/pipelines/select${runtimeProfileID ? `?profile_id=${runtimeProfileID}` : ""}`,
        );
        const active = item.versions.find(
          (candidate) => candidate.status === "published",
        );
        setPipelines([item]);
        if (active) open(active, item, false, loadedWorkflowCatalog);
        return;
      }
      const items = await request<Pipeline[]>("review/pipelines");
      setPipelines(items);
    } catch (e) {
      if (e instanceof Error && e.message === "AUTH") onAuthError();
      else setError(e instanceof Error ? e.message : String(e));
    }
  }, [request, onAuthError, readOnly, runtimeProfileID, manualApproval]);
  useEffect(() => {
    void load();
  }, [load]);
  const stageType = (key: string) => catalog.find((item) => item.key === key);
  const buildNode = useCallback(
    (stage: Stage, index: number): Node<StudioNodeData> => {
      const fallback = defaultPositions[index] ?? {
        x: 160 + (index % 4) * 360,
        y: 160 + Math.floor(index / 4) * 290,
      };
      return {
        id: stage.key,
        type: "studio",
        position: {
          x: Number(stage.config?.x ?? fallback.x),
          y: Number(stage.config?.y ?? fallback.y),
        },
        data: {
          stage,
          editable,
          removable: !stage.required,
          runtime: runtimeEvents[stage.key],
          active: activeStage === stage.key,
          select: () => {
            setSelectedNode(stage.key);
            setSelectedEdge(null);
            setStatusOpen(false);
          },
          openLogs: () => {
            if (stage.key === "pre-publicacao") onOpenPreReview?.();
            else
              onOpenLogs?.(stage.key, stage.name, runtimeLogs[stage.key] || []);
          },
          update: (patch) =>
            setNodes((all) =>
              all.map((n) =>
                n.id === stage.key
                  ? {
                      ...n,
                      data: { ...n.data, stage: { ...n.data.stage, ...patch } },
                    }
                  : n,
              ),
            ),
          remove: () => {
            setNodes((all) => all.filter((n) => n.id !== stage.key));
            setEdges((all) =>
              all.filter(
                (e) => e.source !== stage.key && e.target !== stage.key,
              ),
            );
            setTriggers((all) =>
              all.map((trigger) =>
                trigger.target_stage_key === stage.key
                  ? { ...trigger, enabled: false, target_stage_key: "" }
                  : trigger,
              ),
            );
          },
        },
      };
    },
    [
      editable,
      runtimeEvents,
      runtimeLogs,
      activeStage,
      onOpenLogs,
      onOpenPreReview,
      setNodes,
      setEdges,
    ],
  );
  function open(
    versionToOpen: Version,
    parent: Pipeline,
    _editingMode = editable,
    availableWorkflowCatalog = workflowCatalog,
  ) {
    setPipelineID(parent.id);
    setVersion(versionToOpen);
    setName(parent.name);
    onPipelineName?.(parent.name);
    setDescription(parent.description);
    setProfileID(String(versionToOpen.profile_id ?? parent.profile_id ?? ""));
    setSchedulerMaxRuns(
      versionToOpen.scheduler_max_runs || defaultSchedulerMaxRuns,
    );
    const loadedTriggers = versionToOpen.triggers ?? [];
    const defaultTarget =
      versionToOpen.stages.find(
        (stage) => stage.executor_key === "preparation",
      )?.key ?? versionToOpen.stages[0]?.key ?? "";
    setTriggers(
      loadedTriggers.length
        ? normalizeTriggers(loadedTriggers)
        : defaultTriggers(defaultTarget),
    );
    const byID = new Map(
      versionToOpen.stages.map((stage) => [stage.id, stage.key]),
    );
    const stages = versionToOpen.stages.map((stage) => {
      const metadata = stageType(stage.stage_type_key);
      const kind = processorKind({
        ...stage,
        processor_kind: stage.processor_kind ?? metadata?.processor_kind,
      });
      const processor = availableWorkflowCatalog.processors.find(
        (item) => item.key === kind,
      );
      const inputContract = stage.input_contract ?? metadata?.input_contract;
      const outputContract = stage.output_contract ?? metadata?.output_contract;
      const catalogInput = availableWorkflowCatalog.contracts.find(
        (contract) => contract.key === inputContract?.key,
      );
      const catalogOutput = availableWorkflowCatalog.contracts.find(
        (contract) => contract.key === outputContract?.key,
      );
      return {
        ...stage,
        processor_kind: kind,
        route_mode: stage.route_mode ?? stage.routing_mode ?? "all_matches",
        routing_mode: stage.routing_mode ?? stage.route_mode ?? "all_matches",
        join_mode: stage.join_mode ?? "each_arrival",
        config: {
          ...schemaDefaults(processorConfigSchema(processor)),
          ...stage.config,
        },
        input_contract: inputContract
          ? {
              ...metadata?.input_contract,
              ...inputContract,
              schema:
                inputContract.schema ??
                metadata?.input_contract?.schema ??
                catalogInput?.schema,
            }
          : undefined,
        output_contract: outputContract
          ? {
              ...metadata?.output_contract,
              ...outputContract,
              schema:
                outputContract.schema ??
                metadata?.output_contract?.schema ??
                catalogOutput?.schema,
            }
          : undefined,
      } satisfies Stage;
    });
    setNodes(stages.map(buildNode));
    const stageByKey = new Map(stages.map((stage) => [stage.key, stage]));
    const ports = (source: string, target: string) => ({
      sourceHandle: `output-${portSide(stageByKey.get(source)!, "output")}`,
      targetHandle: `input-${portSide(stageByKey.get(target)!, "input")}`,
    });
    const transitions = versionToOpen.transitions
      .filter((item) => item.from_stage_id && item.to_stage_id)
      .map((item, index) => {
        const source = byID.get(item.from_stage_id!)!,
          target = byID.get(item.to_stage_id!)!;
        return {
          id: String(item.id ?? index),
          source,
          target,
          ...ports(source, target),
          label: `${item.type} · ${ruleSummary(item.rule, item.condition_key)}`,
          data: item,
          animated: item.type !== "success",
          className: isTechnicalTransition(item.type)
            ? "studio-technical-edge"
            : "studio-business-edge",
          style: {
            stroke: contractColor(stageByKey.get(source)?.output_contract?.key),
          },
        };
      });
    setEdges(transitions);
  }
  function create() {
    setPipelineID(null);
    setVersion(null);
    setName("");
    setDescription("");
    setProfileID("");
    setSchedulerMaxRuns(defaultSchedulerMaxRuns);
    const keys = [
      "preparation",
      "planner",
      "reviewer",
      "consolidator",
      "verification",
      "formatting",
      "publication",
    ];
    const stages = keys.map((key, index) => newStage(key, index));
    setTriggers(defaultTriggers(stages[0]?.key));
    setNodes(stages.map(buildNode));
    setEdges(
      stages.slice(0, -1).map((s, i) => ({
        id: `e-${i}`,
        source: s.key,
        target: stages[i + 1].key,
        label: "success · always",
        className: "studio-business-edge",
        data: {
          type: "success",
          condition_key: "always",
          max_traversals: 1,
          priority: 0,
        },
      })),
    );
  }
  function discard() {
    if (version && current) open(version, current);
    else create();
    setError("");
  }
  function newStage(typeKey: string, index = nodes.length): Stage {
    const type = stageType(typeKey);
    const fixed: Record<string, [string, string, boolean, boolean]> = {
      preparation: ["preparacao", "Preparação", false, true],
      verification: ["verificacao", "Verificação", true, true],
      formatting: ["formatacao", "Formatação", true, true],
      publication: ["publicacao", "Publicação", false, true],
      error_log: ["erro", "Log de erro", false, false],
    };
    const [key, title, llm, required] = fixed[typeKey] ?? [
      `${typeKey}-${crypto.randomUUID().slice(0, 8)}`,
      type?.name ?? typeKey,
      true,
      false,
    ];
    const kind = type ? stageTypeProcessorKind(type) : (llm ? "llm" : "system");
    const processor = workflowCatalog.processors.find((item) => item.key === kind);
    const inputSchema = type?.input_contract?.schema ?? workflowCatalog.contracts.find(
      (contract) => contract.key === type?.input_contract?.key,
    )?.schema;
    const config = schemaDefaults(processorConfigSchema(processor));
    const inputFields = schemaFields(inputSchema);
    if (kind === "rule_filter" && inputFields.length)
      config.rule = defaultRule(inputFields);
    return {
      stage_type_key: typeKey,
      executor_key: type?.executor_key,
      processor_kind: kind,
      key,
      name: title,
      prompt_template: "",
      max_output_tokens: 2000,
      retry_limit: 1,
      timeout_seconds: 900,
      use_llm: kind === "llm",
      required,
      routing_mode: "all_matches",
      route_mode: "all_matches",
      join_mode: "each_arrival",
      config: { ...config, x: 100 + index * 70, y: 120 + index * 40 },
      input_contract: type?.input_contract
        ? { key: type.input_contract.key, schema: inputSchema }
        : undefined,
      output_contract: type?.output_contract
        ? {
            key: type.output_contract.key,
            schema:
              type.output_contract.schema ??
              workflowCatalog.contracts.find(
                (contract) => contract.key === type.output_contract?.key,
              )?.schema,
          }
        : undefined,
    };
  }
  function addStage(typeKey: string, position?: { x: number; y: number }) {
    const singleton = [
      "preparation",
      "verification",
      "formatting",
      "publication",
      "error_log",
    ].includes(typeKey);
    if (singleton && nodes.some((node) => node.data.stage.stage_type_key === typeKey)) {
      setError("Esta etapa só pode aparecer uma vez no fluxo.");
      return;
    }
    const stage = newStage(typeKey, nodes.length);
    if (position) stage.config = { ...stage.config, ...position };
    setNodes((all) => [...all, buildNode(stage, all.length)]);
    setSelectedNode(stage.key);
    setSelectedEdge(null);
    setStatusOpen(false);
    setError("");
  }
  const connect = useCallback(
    (connection: Connection) => {
      if (!editable) return;
      const target = nodes.find((node) => node.id === connection.target)?.data
        .stage;
      if (connection.source?.startsWith("entrypoint-")) {
        const index = Number(connection.source.replace("entrypoint-", ""));
        if (!target || !Number.isInteger(index) || !triggers[index]) return;
        setTriggers((all) =>
          all.map((trigger, triggerIndex) =>
            triggerIndex === index
              ? { ...trigger, enabled: true, target_stage_key: target.key }
              : trigger,
          ),
        );
        setError("");
        return;
      }
      const source = nodes.find((node) => node.id === connection.source)?.data
        .stage;
      if (!source || !target) return;
      if (source.key === target.key)
        return setError("Uma etapa não pode ser conectada a ela mesma.");
      if (["publication", "error_log"].includes(source.executor_key || ""))
        return setError("Etapas terminais não podem iniciar novas conexões.");
      if (
        edges.some(
          (edge) => edge.source === source.key && edge.target === target.key,
        )
      )
        return setError("Esta conexão já existe.");
      if (
        target.executor_key !== "error_log" &&
        source.output_contract?.key !== target.input_contract?.key
      )
        return setError("As portas só conectam contratos compatíveis.");
      const type = target.executor_key === "error_log" ? "failure" : "success";
      setError("");
      setEdges((all) =>
        addEdge(
          {
            ...connection,
            sourceHandle: `output-${portSide(source, "output")}`,
            targetHandle: `input-${portSide(target, "input")}`,
            id: crypto.randomUUID(),
            label: `${type} · ${type === "failure" ? "error" : "always"}`,
            className: isTechnicalTransition(type)
              ? "studio-technical-edge"
              : "studio-business-edge",
            style: { stroke: contractColor(source.output_contract?.key) },
            data: {
              type,
              condition_key: type === "failure" ? "error" : "always",
              max_traversals: 1,
              priority: 0,
            },
          },
          all,
        ),
      );
    },
    [editable, edges, nodes, setEdges, triggers],
  );
  function validateDraft(
    stages: Stage[],
    transitions: Array<{
      from_stage_key: string;
      to_stage_key: string;
      type: string;
      condition_key: string;
      rule?: Rule;
      max_traversals: number;
    }>,
  ) {
    for (const stage of stages) {
      const kind = processorKind(stage);
      const processor = workflowCatalog.processors.find((item) => item.key === kind);
      if (processor && !processor.executable)
        return `O processador ${processor.name} ainda não é executável pelo runtime.`;
      const join = workflowCatalog.join_modes.find((item) => item.key === stage.join_mode);
      if (join && !join.executable)
        return `O modo de junção ${join.key} ainda não é executável pelo runtime.`;
      if (kind === "rule_filter") {
        const extensions = Array.isArray(stage.config.include_extensions)
          ? stage.config.include_extensions.filter(
              (value): value is string =>
                typeof value === "string" && value.trim() !== "",
            )
          : [];
        const rule = stage.config.rule as Rule | undefined;
        if ((!rule || !meaningfulRule(rule)) && extensions.length === 0)
          return `A etapa ${stage.name} precisa de uma regra ou extensão de filtro válida.`;
      }
    }

    const adjacency = new Map<string, string[]>();
    transitions.forEach((transition) => {
      if (!isTechnicalTransition(transition.type))
        adjacency.set(transition.from_stage_key, [
          ...(adjacency.get(transition.from_stage_key) ?? []),
          transition.to_stage_key,
        ]);
    });
    for (const transition of transitions) {
      const technical = isTechnicalTransition(transition.type);
      if (technical && !technicalConditions.includes(transition.condition_key))
        return "Fallbacks técnicos aceitam somente error, timeout ou contract_invalid.";
      if (technical && transition.rule)
        return "Fallbacks técnicos não podem usar regras sobre o payload.";
      if (!technical && technicalConditions.includes(transition.condition_key))
        return "Conexões de negócio não podem usar categorias de fallback técnico.";
      if (
        !technical &&
        !transition.rule &&
        !businessConditions.includes(transition.condition_key)
      )
        return "A conexão de negócio precisa de uma regra ou condição válida.";
      if (
        !technical &&
        graphReaches(
          adjacency,
          transition.to_stage_key,
          transition.from_stage_key,
        ) &&
        transition.max_traversals < 1
      )
        return "Toda conexão que fecha um loop precisa de um máximo de travessias maior que zero.";
    }
    return "";
  }
  async function save(publish = false) {
    if (busy) return;
    const persistedNodes = nodes.filter(
      (node) => node.data.stage.stage_type_key !== "pre-publicacao",
    );
    const stageKeys = new Set(persistedNodes.map((node) => node.id));
    const stages = persistedNodes.map((node, index) => ({
      ...node.data.stage,
      config: stageConfigForSave(node.data.stage, node.position),
      position: index + 1,
    }));
    const transitions = edges
      .filter(
        (edge) => stageKeys.has(edge.source) && stageKeys.has(edge.target),
      )
      .map((edge, priority) => {
        const rule = edge.data?.rule as Rule | undefined;
        return {
          from_stage_key: edge.source,
          to_stage_key: edge.target,
          type: String(edge.data?.type ?? "success"),
          condition_key: String(edge.data?.condition_key ?? "always"),
          ...(rule ? { rule } : {}),
          max_traversals: Math.max(0, Number(edge.data?.max_traversals ?? 0)),
          priority,
        };
      });
    const localError = validateDraft(stages, transitions);
    if (localError) {
      setError(localError);
      setStatusOpen(true);
      return;
    }
    const body = {
      name,
      description,
      profile_id: profileID ? Number(profileID) : null,
      scheduler_max_runs: schedulerMaxRuns,
      stages,
      transitions,
      triggers,
    };
    setBusy(true);
    setError("");
    try {
      if (!version) {
        const key =
          name
            .toLowerCase()
            .replace(/[^a-z0-9]+/g, "-")
            .replace(/^-|-$/g, "") || `pipeline-${Date.now()}`;
        const created = await request<Pipeline>("review/pipelines", {
          method: "POST",
          body: JSON.stringify({ ...body, key }),
        });
        const draft = created.versions[0];
        setPipelines((all) => [...all, created]);
        open(draft, created, true);
      } else if (current) {
        const updated = await request<Pipeline>(
          `review/pipelines/${current.id}/drafts/${version.id}`,
          {
            method: "PUT",
            body: JSON.stringify(body),
          },
        );
        if (publish) {
          const published = await request<Pipeline>(
            `review/pipelines/${current.id}/drafts/${version.id}/publish`,
            { method: "POST" },
          );
          setEditing(false);
          onEditingChange?.(false);
          setMenuOpen(false);
          setPipelines([published]);
          open(published.versions[0], published, false);
        } else {
          const draft = updated.versions[0];
          setPipelines([updated]);
          open(draft, updated, true);
          setMenuOpen(true);
        }
      }
      onSaved();
    } catch (e) {
      if (e instanceof Error && e.message === "AUTH") onAuthError();
      else setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  const updateSelectedEdge = (patch: Partial<Transition>) =>
    setEdges((all) =>
      all.map((edge) => {
        if (edge.id !== selectedEdge) return edge;
        const data = { ...edge.data, ...patch } as Transition;
        return {
          ...edge,
          label: `${data.type ?? "success"} · ${ruleSummary(
            data.rule,
            data.condition_key ?? "always",
          )}`,
          className: isTechnicalTransition(data.type)
            ? "studio-technical-edge"
            : "studio-business-edge",
          data,
        };
      }),
    );
  async function beginEditing() {
    if (!current || !version || busy) return;
    setBusy(true);
    setError("");
    try {
      let draftPipeline: Pipeline;
      try {
        draftPipeline = await request<Pipeline>(
          `review/pipelines/${current.id}/clone?version_id=${version.id}`,
          { method: "POST" },
        );
      } catch (failure) {
        if (
          !(failure instanceof Error) ||
          !failure.message.includes("already has a draft")
        )
          throw failure;
        const existing = await request<Pipeline>(
          `review/pipelines/${current.id}`,
        );
        const draft = existing.versions.find(
          (candidate) => candidate.status === "draft",
        );
        if (!draft) throw failure;
        draftPipeline = { ...existing, versions: [draft] };
      }
      const draft = draftPipeline.versions.find(
        (candidate) => candidate.status === "draft",
      );
      if (!draft) throw new Error("O backend não retornou o draft da pipeline.");
      setEditing(true);
      onEditingChange?.(true);
      setPipelines([draftPipeline]);
      open(draft, draftPipeline, true);
      setMenuOpen(true);
    } catch (failure) {
      if (failure instanceof Error && failure.message === "AUTH") onAuthError();
      else setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    if (handledEditRequest.current === editRequest) return;
    handledEditRequest.current = editRequest;
    if (readOnly && current && version && !editing) {
      void beginEditing();
    }
  }, [editRequest, readOnly, current, version, editing]);
  useEffect(() => {
    if (editing && saveRequest) void save();
  }, [saveRequest]);
  useEffect(() => {
    if (editing && publishRequest) void save(true);
  }, [publishRequest]);
  useEffect(() => {
    if (editing && discardRequest) {
      setEditing(false);
      onEditingChange?.(false);
      setMenuOpen(false);
      void load();
    }
  }, [discardRequest]);
  useEffect(() => {
    setNodes((all) =>
      all.map((node) => ({
        ...node,
        data: {
          ...node.data,
          editable,
          removable: !node.data.stage.required,
        },
      })),
    );
  }, [editable, setNodes]);
  useEffect(() => {
    if (readOnly)
      setNodes((all) =>
        all.map((node) => ({
          ...node,
          data: {
            ...node.data,
            runtime: runtimeEvents[node.id],
            active: activeStage === node.id,
          },
        })),
      );
  }, [readOnly, runtimeEvents, activeStage, setNodes]);
  useEffect(() => {
    if (readOnly)
      setEdges((all) =>
        all.map((edge) => ({
          ...edge,
          animated: edge.source === activeStage || edge.target === activeStage,
          className: `studio-runtime-edge ${
            isTechnicalTransition(edge.data?.type)
              ? "studio-technical-edge"
              : "studio-business-edge"
          }`,
        })),
      );
  }, [readOnly, activeStage, setEdges]);
  useEffect(() => {
    if (editable)
      setEdges((all) =>
        all.map((edge) => {
          const source = nodes.find((node) => node.id === edge.source)?.data
            .stage;
          const target = nodes.find((node) => node.id === edge.target)?.data
            .stage;
          return source && target
            ? {
                ...edge,
                sourceHandle: `output-${portSide(source, "output")}`,
                targetHandle: `input-${portSide(target, "input")}`,
                style: {
                  ...edge.style,
                  stroke: contractColor(source.output_contract?.key),
                },
              }
            : edge;
        }),
      );
  }, [editable, nodes, setEdges]);
  const entrypointNodes: Node<EntrypointNodeData>[] = triggers.map(
    (trigger, index) => ({
      id: `entrypoint-${index}`,
      type: "entrypoint",
      position: {
        x: Number(trigger.config?.x ?? entrypointPositions[index].x),
        y: Number(trigger.config?.y ?? entrypointPositions[index].y),
      },
      draggable: editable,
      selectable: true,
      data: {
        trigger,
        editable,
        updateSource: (source) =>
          setTriggers((all) => {
            const updated = [...all];
            const previousSource = updated[index].source;
            const occupiedIndex = updated.findIndex(
              (item, itemIndex) =>
                itemIndex !== index && item.source === source,
            );
            updated[index] = { ...updated[index], source };
            if (occupiedIndex >= 0)
              updated[occupiedIndex] = {
                ...updated[occupiedIndex],
                source: previousSource,
              };
            return updated;
          }),
        toggle: () =>
          setTriggers((all) =>
            all.map((item, itemIndex) =>
              itemIndex === index
                ? {
                    ...item,
                    enabled: !item.enabled,
                    target_stage_key:
                      item.target_stage_key || nodes[0]?.id || "",
                  }
                : item,
            ),
          ),
      },
    }),
  );
  const entrypointEdges: Edge[] = triggers.flatMap((trigger, index) => {
    const target = nodes.find((node) => node.id === trigger.target_stage_key);
    return target
      ? [{
        id: `entrypoint-edge-${index}`,
        source: `entrypoint-${index}`,
        target: target.id,
        sourceHandle: "entry-output",
        targetHandle: `input-${portSide(target.data.stage, "input")}`,
        selectable: true,
        deletable: false,
        animated: trigger.enabled,
        className: "studio-entrypoint-edge",
        style: { stroke: "#e6a34e", opacity: trigger.enabled ? 1 : 0.22 },
      }]
      : [];
  });
  const formattingNode = nodes.find(
    (node) => node.data.stage.executor_key === "formatting",
  );
  const publicationNode = nodes.find(
    (node) => node.data.stage.executor_key === "publication",
  );
  const approvalMovesLeft = Boolean(
    formattingNode &&
      publicationNode &&
      publicationNode.position.x < formattingNode.position.x,
  );
  const approvalStage: Stage = {
    stage_type_key: "pre-publicacao",
    executor_key: "pre-publicacao",
    key: "pre-publicacao",
    name: "Pré-aprovação manual",
    prompt_template: "",
    max_output_tokens: 0,
    retry_limit: 0,
    timeout_seconds: 0,
    use_llm: false,
    required: false,
    processor_kind: "manual",
    routing_mode: "all_matches",
    route_mode: "all_matches",
    join_mode: "each_arrival",
    config: {
      input_side: approvalMovesLeft ? "right" : "left",
      output_side: approvalMovesLeft ? "left" : "right",
    },
    input_contract: { key: "formatted_review" },
    output_contract: { key: "formatted_review" },
  };
  const approvalNode: Node<StudioNodeData> | null =
    manualApproval && formattingNode && publicationNode
      ? {
          id: approvalStage.key,
          type: "studio",
          position: {
            x:
              (formattingNode.position.x + publicationNode.position.x) / 2,
            y:
              (formattingNode.position.y + publicationNode.position.y) / 2 +
              (Math.abs(
                formattingNode.position.x - publicationNode.position.x,
              ) < 700
                ? 280
                : 0),
          },
          draggable: false,
          selectable: false,
          data: {
            stage: approvalStage,
            editable: false,
            removable: false,
            runtime: runtimeEvents[approvalStage.key],
            active: activeStage === approvalStage.key,
            select: () => onOpenPreReview?.(),
            openLogs: () => onOpenPreReview?.(),
            update: () => undefined,
            remove: () => undefined,
          },
        }
      : null;
  const approvalEdges: Edge[] = approvalNode
    ? [
        {
          id: "formatting-pre-publicacao",
          source: formattingNode!.id,
          target: approvalNode.id,
          sourceHandle: `output-${portSide(formattingNode!.data.stage, "output")}`,
          targetHandle: `input-${portSide(approvalStage, "input")}`,
          label: "success · always",
          className: "studio-business-edge",
          selectable: false,
          deletable: false,
          style: {
            stroke: contractColor(
              formattingNode!.data.stage.output_contract?.key,
            ),
          },
        },
        {
          id: "pre-publicacao-publication",
          source: approvalNode.id,
          target: publicationNode!.id,
          sourceHandle: `output-${portSide(approvalStage, "output")}`,
          targetHandle: `input-${portSide(publicationNode!.data.stage, "input")}`,
          label: "approval",
          className: "studio-business-edge",
          selectable: false,
          deletable: false,
          style: { stroke: "#e6a34e" },
        },
      ]
    : [];
  const workflowEdges = approvalNode
    ? edges.filter(
        (edge) =>
          edge.source !== formattingNode!.id ||
          edge.target !== publicationNode!.id,
      )
    : edges;
  const renderedNodes: Node[] = [
    ...entrypointNodes,
    ...nodes,
    ...(approvalNode ? [approvalNode] : []),
  ];
  const renderedEdges = [
    ...entrypointEdges,
    ...workflowEdges,
    ...approvalEdges,
  ];
  const selectedStage = nodes.find((node) => node.id === selectedNode)?.data
    .stage;
  const selectedConnection = edges.find((edge) => edge.id === selectedEdge);
  const selectedEntrypointIndex = selectedEdge?.startsWith("entrypoint-edge-")
    ? Number(selectedEdge.replace("entrypoint-edge-", ""))
    : -1;
  const selectedConnectionTarget = nodes.find(
    (node) => node.id === selectedConnection?.target,
  )?.data.stage;
  const selectedConnectionSource = nodes.find(
    (node) => node.id === selectedConnection?.source,
  )?.data.stage;
  const selectedConnectionTechnical = isTechnicalTransition(
    selectedConnection?.data?.type,
  );
  const addableStageTypes = catalog
    .filter((type) => type.is_enabled)
    .filter(
      (type) =>
        ![
          "preparation",
          "verification",
          "formatting",
          "publication",
          "error_log",
        ].includes(type.key) ||
        !nodes.some((node) => node.data.stage.stage_type_key === type.key),
    );
  return (
    <section
      className={`pipeline-studio ${readOnly ? "runtime-studio" : ""} ${editable ? "editing" : ""}`}
    >
      {!readOnly && (
        <header className="studio-topbar">
          <div>
            <span className="eyebrow">Pipeline Studio</span>
            <h2>{name || "Novo workflow"}</h2>
          </div>
          <div>
            <button
              className="secondary-button"
              disabled={busy}
              onClick={discard}
            >
              <RotateCcw size={16} /> Descartar
            </button>
            {editable && (
              <button
                className="secondary-button"
                disabled={busy}
                onClick={() => void save()}
              >
                <Save size={16} /> Salvar
              </button>
            )}
            {editable && version && (
              <button
                className="primary-button"
                disabled={busy}
                onClick={() => void save(true)}
              >
                <Send size={16} /> Publicar
              </button>
            )}
          </div>
        </header>
      )}
      {error && <div className="banner error">{error}</div>}
      {menuOpen && (
        <aside className="studio-sidebar">
          <button
            className="studio-close"
            onClick={() => setMenuOpen(false)}
            aria-label="Fechar menu"
          >
            <X size={16} />
          </button>
          {editable && (
            <section className="studio-pipeline-fields">
              <strong>Draft</strong>
              <label>
                Nome
                <input value={name} onChange={(event) => setName(event.target.value)} />
              </label>
              <label>
                Descrição
                <textarea
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                />
              </label>
              <label>
                Profile
                <select
                  value={profileID}
                  onChange={(event) => setProfileID(event.target.value)}
                >
                  <option value="">Global</option>
                  {profiles.map((profile) => (
                    <option key={profile.id} value={profile.id}>
                      {profile.name}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                Máximo de execuções do scheduler
                <input
                  type="number"
                  min="1"
                  max={maxSchedulerMaxRuns}
                  value={schedulerMaxRuns}
                  onChange={(event) =>
                    setSchedulerMaxRuns(
                      Math.min(
                        maxSchedulerMaxRuns,
                        Math.max(1, event.target.valueAsNumber || 1),
                      ),
                    )
                  }
                />
              </label>
            </section>
          )}
          <section>
            <strong>Pontos de entrada</strong>
            {triggers.map((trigger, index) => (
              <button
                key={trigger.source}
                className={trigger.enabled ? "active" : ""}
                disabled={!editable}
                onClick={() =>
                  setTriggers((all) =>
                    all.map((item, itemIndex) =>
                      itemIndex === index
                        ? {
                            ...item,
                            enabled: !item.enabled,
                            target_stage_key:
                              item.target_stage_key || nodes[0]?.id || "",
                          }
                        : item,
                    ),
                  )
                }
              >
                {triggerIcon(trigger.source)}
                {triggerLabels[trigger.source]}
                <small>{trigger.enabled ? "No flow" : "Adicionar"}</small>
              </button>
            ))}
          </section>
          <section>
            <strong>Workflows</strong>
            {pipelines.map((p) => (
              <button
                key={p.id}
                className={p.id === pipelineID ? "active" : ""}
                disabled={readOnly}
                onClick={() => {
                  setPipelineID(p.id);
                  setVersion(null);
                  setNodes([]);
                  setEdges([]);
                }}
              >
                <GitBranch size={15} /> {p.name}
                {p.is_default && <Star size={13} />}
              </button>
            ))}
          </section>
          {current && !version && (
            <section>
              <strong>Versões</strong>
              {current.versions.map((v) => (
                <button
                  key={v.id}
                  disabled={readOnly}
                  onClick={() => open(v, current)}
                >
                  v{v.version} <small>{v.status}</small>
                </button>
              ))}
              {!current.is_default && (
                <button
                  onClick={() =>
                    void request(`review/pipelines/${current.id}/select`, {
                      method: "POST",
                    }).then(() => load())
                  }
                >
                  <Star size={15} /> Selecionar
                </button>
              )}
            </section>
          )}
          <section>
            <strong>Processadores e etapas</strong>
            {processorGroups.map((group) => {
              const processor = workflowCatalog.processors.find(
                (item) => item.key === group,
              );
              const types = addableStageTypes.filter((type) => {
                return stageTypeProcessorKind(type) === group;
              });
              return (
                <div className="studio-processor-group" key={group}>
                  <header>
                    <span>{processor?.name || processorGroupLabels[group]}</span>
                    <small>{processor?.executable ? "Executável" : "Indisponível"}</small>
                  </header>
                  {processor?.description && <p>{processor.description}</p>}
                  {types.map((type) => (
                    <button
                      key={type.key}
                      disabled={!editable || !processor?.executable}
                      draggable={editable && processor?.executable}
                      onDragStart={(event) =>
                        event.dataTransfer.setData("stage", type.key)}
                      onClick={() => editable && processor?.executable && addStage(type.key)}
                    >
                      <Plus size={14} /> {type.name}
                    </button>
                  ))}
                  {!types.length && <small>Nenhuma etapa deste tipo no catálogo.</small>}
                </div>
              );
            })}
          </section>
        </aside>
      )}
      <main
        className="studio-canvas"
        onDragOver={(e) => e.preventDefault()}
        onDrop={(e) => {
          const type = e.dataTransfer.getData("stage");
          if (!type || !editable) return;
          const position = flowInstance?.screenToFlowPosition({
            x: e.clientX,
            y: e.clientY,
          });
          addStage(type, position);
        }}
      >
        <ReactFlow
          nodes={renderedNodes}
          edges={renderedEdges}
          nodeTypes={nodeTypes}
          onInit={setFlowInstance}
          onNodesChange={(changes) => {
            changes.forEach((change) => {
              if (
                "id" in change &&
                change.id.startsWith("entrypoint-") &&
                "position" in change &&
                change.position
              ) {
                const index = Number(change.id.replace("entrypoint-", ""));
                setTriggers((all) =>
                  all.map((trigger, triggerIndex) =>
                    triggerIndex === index
                      ? {
                          ...trigger,
                          config: { ...trigger.config, ...change.position },
                        }
                      : trigger,
                  ),
                );
              }
            });
            onNodesChange(
              changes.filter(
                (change) =>
                  !("id" in change) || !change.id.startsWith("entrypoint-"),
              ) as Parameters<typeof onNodesChange>[0],
            );
          }}
          onEdgesChange={(changes) =>
            onEdgesChange(
              changes.filter(
                (change) =>
                  !("id" in change) ||
                  !change.id.startsWith("entrypoint-edge-"),
              ),
            )
          }
          onConnect={connect}
          nodesDraggable={editable}
          nodesConnectable={editable}
          edgesReconnectable={editable}
          elementsSelectable
          deleteKeyCode={null}
          onEdgeClick={(_, edge) => {
            setSelectedEdge(edge.id);
            setSelectedNode(null);
            setStatusOpen(false);
          }}
          fitView
        >
          <Background gap={18} size={1} />
          <Controls />
        </ReactFlow>
        <button
          className="studio-menu-toggle"
          onClick={() => setMenuOpen(true)}
          title="Abrir menu"
        >
          <PanelLeftOpen size={17} />
        </button>
        <div className="studio-hint">
          <MousePointer2 size={14} /> Configure Entrypoints, arraste etapas e
          conecte portas compatíveis.
        </div>
        <button
          className={`studio-status ${error ? "error" : "ok"}`}
          onClick={() => {
            setStatusOpen((value) => !value);
            setSelectedNode(null);
          }}
          title="Status da pipeline"
        >
          <CircleAlert size={16} /> {error ? "Atenção" : editable ? "Editando" : "Carregada"}
        </button>
      </main>
      {(selectedStage || selectedEdge || statusOpen) && (
        <aside className="studio-inspector">
          <button
            className="studio-close"
            onClick={() => {
              setSelectedNode(null);
              setSelectedEdge(null);
              setStatusOpen(false);
            }}
            aria-label="Fechar detalhes"
          >
            <X size={16} />
          </button>
          {selectedStage ? (
            <StageInspector
              stage={selectedStage}
              editable={editable}
              runtime={runtimeEvents[selectedStage.key]}
              workflowCatalog={workflowCatalog}
              onUpdate={(patch) =>
                setNodes((all) =>
                  all.map((node) =>
                    node.id === selectedStage.key
                      ? {
                          ...node,
                          data: {
                            ...node.data,
                            stage: { ...node.data.stage, ...patch },
                          },
                        }
                      : node,
                  ),
                )
              }
            />
          ) : !selectedEdge ? (
            <>
              <strong>Status da pipeline</strong>
              <div className="studio-error-note">
                <CircleAlert size={15} />{" "}
                {error ||
                  "Pipeline carregada. Rotas failure e fallback só podem apontar para Log de erro."}
              </div>
            </>
          ) : null}
          {selectedEdge && selectedEntrypointIndex >= 0 ? (
            <section className="studio-edge">
              <strong>Conexão do Entrypoint</strong>
              <label>
                Origem
                <input
                  disabled
                  value={triggerLabels[triggers[selectedEntrypointIndex].source]}
                />
              </label>
              <label>
                Destino
                <select
                  disabled={!editable}
                  value={triggers[selectedEntrypointIndex].target_stage_key}
                  onChange={(event) =>
                    setTriggers((all) =>
                      all.map((trigger, index) =>
                        index === selectedEntrypointIndex
                          ? {
                              ...trigger,
                              enabled: true,
                              target_stage_key: event.target.value,
                            }
                          : trigger,
                      ),
                    )
                  }
                >
                  {nodes.map((node) => (
                    <option key={node.id} value={node.id}>
                      {node.data.stage.name}
                    </option>
                  ))}
                </select>
              </label>
              <button
                disabled={!editable}
                onClick={() => {
                  setTriggers((all) =>
                    all.map((trigger, index) =>
                      index === selectedEntrypointIndex
                        ? {
                            ...trigger,
                            enabled: false,
                            target_stage_key: "",
                          }
                        : trigger,
                    ),
                  );
                  setSelectedEdge(null);
                }}
              >
                <Trash2 size={14} /> Remover conexão
              </button>
            </section>
          ) : selectedEdge ? (
            <section className="studio-edge">
              <strong>
                {selectedConnectionTechnical
                  ? "Fallback técnico"
                  : "Conexão de negócio"}
              </strong>
              <label>
                Tipo
                <select
                  disabled={!editable}
                  value={String(
                    selectedConnection?.data?.type ?? "success",
                  )}
                  onChange={(event) => {
                    const type = event.target.value;
                    const technical = isTechnicalTransition(type);
                    updateSelectedEdge({
                      type,
                      condition_key: technical ? "error" : "always",
                      rule: undefined,
                    });
                  }}
                >
                  {(selectedConnectionTarget?.executor_key === "error_log"
                    ? ["failure", "fallback"]
                    : ["success", "skip"]
                  ).map((v) => (
                    <option key={v}>{v}</option>
                  ))}
                </select>
              </label>
              <label>
                Condição
                <select
                  disabled={!editable}
                  value={String(
                    selectedConnection?.data?.condition_key ?? "always",
                  )}
                  onChange={(e) =>
                    updateSelectedEdge({ condition_key: e.target.value })
                  }
                >
                  {(selectedConnectionTechnical
                    ? technicalConditions
                    : businessConditions
                  ).map((v) => (
                    <option key={v}>{v}</option>
                  ))}
                </select>
              </label>
              {selectedConnectionTechnical ? (
                <p className="studio-edge-kind-note">
                  Esta rota reage a falhas do runtime e não avalia o payload.
                </p>
              ) : (
                <RuleBuilder
                  disabled={!editable}
                  rule={selectedConnection?.data?.rule as Rule | undefined}
                  schema={
                    selectedConnectionSource?.output_contract?.schema ??
                    stageType(selectedConnectionSource?.stage_type_key ?? "")
                      ?.output_contract?.schema
                  }
                  onChange={(rule) => updateSelectedEdge({ rule })}
                />
              )}
              <label>
                Máximo de travessias
                <input
                  disabled={!editable}
                  min="0"
                  type="number"
                  value={Number(
                    selectedConnection?.data?.max_traversals ?? 1,
                  )}
                  onChange={(event) =>
                    updateSelectedEdge({
                      max_traversals: Number.isFinite(event.target.valueAsNumber)
                        ? Math.max(0, event.target.valueAsNumber)
                        : 0,
                    })
                  }
                />
                <small>Obrigatório e maior que zero somente quando a conexão fecha um loop.</small>
              </label>
              <button
                disabled={!editable}
                onClick={() => {
                  setEdges((all) =>
                    all.filter((edge) => edge.id !== selectedEdge),
                  );
                  setSelectedEdge(null);
                }}
              >
                <Trash2 size={14} /> Remover conexão
              </button>
            </section>
          ) : null}
        </aside>
      )}
    </section>
  );
}
