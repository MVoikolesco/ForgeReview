import { Position } from "@xyflow/react";
import type { ReviewSettingsStageType } from "@/lib/contracts";
import type { Rule } from "./pipeline-types";
import type { Stage, Trigger } from "./pipeline-studio-types";

export const triggerLabels = {
  webhook: "Webhook Gitea",
  api: "POST API",
  manual: "Disparo manual",
};

export const triggerSources = Object.keys(triggerLabels) as Trigger["source"][];

export const processorGroups = ["llm", "rule_filter", "transform_merge", "system"] as const;

export const processorGroupLabels = {
  llm: "Modelo de IA",
  rule_filter: "Filtro",
  transform_merge: "Transformação / combinação",
  system: "Sistema",
};

export const requiredStages = [
  { key: "preparation", label: "Preparação" },
  { key: "verification", label: "Verificação" },
  { key: "formatting", label: "Formatação" },
  { key: "publication", label: "Publicação" },
];

export const transitionTypeLabels: Record<string, string> = {
  success: "Sucesso",
  skip: "Desvio",
  failure: "Falha",
  fallback: "Contingência",
};

export const conditionLabels: Record<string, string> = {
  always: "Sempre",
  has_findings: "Há achados",
  no_findings: "Sem achados",
  partial_result: "Resultado parcial",
  confidence_below_threshold: "Confiança abaixo do limite",
  error: "Erro",
  timeout: "Tempo esgotado",
  contract_invalid: "Contrato inválido",
};

export function contractColor(key?: string) {
  if (key?.includes("findings")) return "#46c892";
  if (key?.includes("review")) return "#ab7cff";
  if (key?.includes("diff")) return "#4d91ff";
  if (key?.includes("error")) return "#e25f67";
  return "#e6a34e";
}

export function portSide(stage: Stage, kind: "input" | "output") {
  const configured = stage.config?.[`${kind}_side`];
  if (configured === "left" || configured === "right") return configured;
  if (
    kind === "output" &&
    ["consolidator", "verification", "formatting"].includes(stage.executor_key || "")
  )
    return "left";
  if (
    kind === "input" &&
    ["verification", "formatting", "publication"].includes(stage.executor_key || "")
  )
    return "right";
  return kind === "input" ? "left" : "right";
}

export function handlePosition(side: string) {
  return side === "left" ? Position.Left : Position.Right;
}

export function processorKind(stage: Stage) {
  if (stage.executor_key === "rule_filter" || stage.executor_key === "transform_merge")
    return stage.executor_key;
  const kind = stage.processor_kind || (stage.use_llm ? "llm" : "system");
  return kind === "deterministic" ? "system" : kind;
}

export function stageProcessorLabel(stage: Stage) {
  return processorGroupLabels[processorKind(stage) as keyof typeof processorGroupLabels] ?? "Processador";
}

export function stageTypeProcessorKind(type: ReviewSettingsStageType) {
  if (type.executor_key === "rule_filter" || type.executor_key === "transform_merge")
    return type.executor_key;
  if (type.processor_kind) return type.processor_kind === "deterministic" ? "system" : type.processor_kind;
  return ["preparation", "publication", "error_log"].includes(type.executor_key)
    ? "system"
    : "llm";
}

export function transitionLabel(type: string, ruleSummary: (rule?: Rule, fallback?: string) => string, rule?: Rule, condition = "always") {
  return `${transitionTypeLabels[type] ?? type} · ${ruleSummary(
    rule,
    conditionLabels[condition] ?? condition,
  )}`;
}
