import type {
  CardType,
  Integration,
  ModelProfile,
  NewIntegration,
  WorkflowDefinition,
  WorkflowSummary,
  WorkflowVersionSummary,
} from "./types";

export const apiURL =
  process.env.NEXT_PUBLIC_API_URL || "http://localhost:8088";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiURL}${path}`, init);
  const payload = (await response.json()) as T & { error?: string };
  if (!response.ok)
    throw new Error(payload.error || "Não foi possível concluir a operação.");
  return payload;
}

export const normalizeCard = (card: CardType): CardType => ({
  ...card,
  inputs: Array.isArray(card.inputs) ? card.inputs : [],
  outputs: Array.isArray(card.outputs) ? card.outputs : [],
  error_output: card.error_output,
});

export const getCards = async () =>
  (await request<CardType[]>("/api/cards")).map(normalizeCard);
export const getIntegrations = () =>
  request<Integration[]>("/api/integrations");
export const createIntegration = (integration: NewIntegration) =>
  request<Integration>("/api/integrations", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(integration),
  });
export const getModelProfiles = () => request<ModelProfile[]>("/api/model-profiles");
export const createModelProfile = (profile: ModelProfile) =>
  request<ModelProfile>("/api/model-profiles", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(profile),
  });
export const saveWorkflow = (definition: WorkflowDefinition) =>
  request<{ version_id: number; status: "draft" }>("/api/workflows", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(definition),
  });
export const getWorkflows = () => request<WorkflowSummary[]>("/api/workflows");
export const publishWorkflow = (versionID: number) =>
  request<WorkflowVersionSummary>(`/api/workflow-versions/${versionID}/publish`, {
    method: "POST",
  });
export const executeWorkflow = (versionID: number) =>
  request<{
    execution_id?: number;
    report?: import("./types").ExecutionReport;
    error?: string;
    status?: string;
  }>(`/api/workflow-versions/${versionID}/executions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ source: "studio" }),
  });
export const getExecution = (executionID: number) =>
  request<import("./types").ExecutionReport>(`/api/executions/${executionID}`);
