import type {
  CardType,
  ExecutionSummary,
  Integration,
  ModelProfile,
  NewIntegration,
  Discovery,
  Repository,
  WorkflowDefinition,
  PublishedWorkflow,
  WorkflowSummary,
  WorkflowVersionSummary,
  WebhookRegistration,
} from "./types";

export const apiURL =
  process.env.NEXT_PUBLIC_API_URL || "http://localhost:8088";

export class APIError extends Error {
  constructor(
    message: string,
    readonly status?: number,
    readonly reason?: string,
  ) {
    super(message);
    this.name = "APIError";
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${apiURL}${path}`, {
      credentials: "include",
      ...init,
    });
  } catch {
    throw new APIError("Não foi possível alcançar o servidor ForgeReview.");
  }
  const payload =
    response.status === 204
      ? undefined
       : ((await response.json()) as T & { error?: string; reason?: string });
  if (!response.ok) {
    const error = new APIError(
      payload?.error || "Não foi possível concluir a operação.",
      response.status,
      payload?.reason,
    );
    if (response.status === 401 && path !== "/api/auth/login")
      window.dispatchEvent(new CustomEvent("forgereview:session-expired"));
    throw error;
  }
  return payload as T;
}

export const normalizeCard = (card: CardType): CardType => ({
  ...card,
  inputs: Array.isArray(card.inputs) ? card.inputs : [],
  outputs: Array.isArray(card.outputs) ? card.outputs : [],
  error_output: card.error_output,
});

export const getCards = async () =>
  (await request<CardType[]>("/api/cards")).map(normalizeCard);
export const getHealth = () => request<{ status: string }>("/health");
export type CurrentUser = {
  id: number;
  email: string;
  role: "viewer" | "editor" | "admin";
};
export const getCurrentUser = () => request<CurrentUser>("/api/auth/me");
export type ManagedUser = CurrentUser & { active: boolean };
export type AuditEntry = {
  id: number;
  actor_id: number;
  action: string;
  target: string;
  metadata: Record<string, unknown>;
  created_at: string;
};
export const getUsers = () => request<ManagedUser[]>("/api/users");
export const createUser = (
  email: string,
  password: string,
  role: CurrentUser["role"],
) =>
  request<ManagedUser>("/api/users", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password, role }),
  });
export const updateUser = (
  id: number,
  role: CurrentUser["role"],
  active: boolean,
) =>
  request<ManagedUser>(`/api/users/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ role, active }),
  });
export const getAuditLog = () => request<AuditEntry[]>("/api/audit-log");
export const login = (email: string, password: string) =>
  request<CurrentUser>("/api/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
export const logout = () =>
  request<void>("/api/auth/logout", { method: "POST" });
export const getIntegrations = () =>
  request<Integration[]>("/api/integrations");
export const createIntegration = (integration: NewIntegration) =>
  request<Integration>("/api/integrations", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(integration),
  });
export const validateIntegration = (integration: NewIntegration) =>
  request<Discovery & { status: string }>("/api/integrations/validate", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(integration),
  });
export const discoverCandidateRepositories = (
  integration: NewIntegration,
  organization: string,
) =>
  request<Discovery>("/api/integrations/discover-repositories", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ...integration, organization }),
  });
export const discoverResources = (key: string, organization?: string) =>
  request<Discovery>(
    `/api/integrations/${encodeURIComponent(key)}/discover${organization ? `?organization=${encodeURIComponent(organization)}` : ""}`,
  );
export const getResources = (key: string) =>
  request<Discovery>(`/api/integrations/${encodeURIComponent(key)}/resources`);
export const replaceRepositories = (key: string, repositories: Repository[]) =>
  request<void>(`/api/integrations/${encodeURIComponent(key)}/repositories`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ repositories }),
  });
export const replaceModels = (key: string, models: string[]) =>
  request<ModelProfile[]>(
    `/api/integrations/${encodeURIComponent(key)}/models`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ models }),
    },
  );
export const disableIntegration = (key: string) =>
  request<void>(`/api/integrations/${encodeURIComponent(key)}/disable`, {
    method: "POST",
  });
export const deleteIntegration = (key: string) =>
  request<void>(`/api/integrations/${encodeURIComponent(key)}`, {
    method: "DELETE",
  });
export const updateIntegration = (
  key: string,
  update: {
    name: string;
    config: { base_url: string };
    status: "active" | "disabled";
    secret?: string;
  },
) =>
  request<Integration>(`/api/integrations/${encodeURIComponent(key)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(update),
  });
export const getModelProfiles = () =>
  request<ModelProfile[]>("/api/model-profiles");
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
export const getPublishedWorkflow = (key: string) =>
  request<PublishedWorkflow>(
    `/api/workflows/${encodeURIComponent(key)}/published`,
  );
export const deleteWorkflowVersion = (versionID: number) =>
  request<void>(`/api/workflow-versions/${versionID}`, {
    method: "DELETE",
  });
export const getWorkflowVersion = (versionID: number) =>
  request<WorkflowDefinition>(`/api/workflow-versions/${versionID}`);
export const publishWorkflow = (versionID: number) =>
  request<WorkflowVersionSummary>(
    `/api/workflow-versions/${versionID}/publish`,
    {
      method: "POST",
    },
  );
export const executeWorkflow = (
  versionID: number,
  triggerNode: string,
  payload: Record<string, unknown>,
) =>
  request<{
    execution_id?: number;
    report?: import("./types").ExecutionReport;
    error?: string;
    status?: string;
  }>(`/api/workflow-versions/${versionID}/executions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ trigger_node: triggerNode, payload }),
  });
export const executePublishedWorkflow = (
  versionID: number,
  triggerNode: string,
  payload: Record<string, unknown>,
) =>
  request<{
    execution_id?: number;
    report?: import("./types").ExecutionReport;
    error?: string;
    status?: string;
  }>(`/api/published-workflow-versions/${versionID}/executions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ trigger_node: triggerNode, payload }),
  });
export const getExecution = (executionID: number) =>
  request<import("./types").ExecutionReport>(`/api/executions/${executionID}`);
export const getExecutions = (limit = 10) =>
  request<ExecutionSummary[]>(`/api/executions?limit=${limit}`);
export const getWebhookRegistrations = () =>
  request<WebhookRegistration[]>("/api/webhook-registrations");
export const saveWebhookRegistration = (registration: {
  key: string;
  name: string;
  workflow_key: string;
  trigger_node_key: string;
  secret: string;
  active: boolean;
}) =>
  request<WebhookRegistration>("/api/webhook-registrations", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(registration),
  });
