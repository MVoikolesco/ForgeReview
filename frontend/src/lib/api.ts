import type {
  CardType,
  CardExecutionLogResponse,
  CoverageSummary,
  ExecutionEvent,
  ExecutionSummary,
  Integration,
  ModelProfile,
  NewIntegration,
  Discovery,
  Repository,
  ResponseContract,
  ReviewContractVersion,
  ReviewChecklist,
  WorkflowDefinition,
  PublishedWorkflow,
  WorkflowSummary,
  WorkflowVersionSummary,
  WebhookRegistration,
} from "./types";
import type { ExecutionReport } from "./types";

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

const executionRunStatuses = new Set(["running", "completed", "failed", "partial"]);
const coverageStatuses = new Set([
  "PLANNED",
  "COMPLETED",
  "CONFIRMED",
  "REJECTED",
  "NEEDS_CONTEXT",
  "NOT_OBSERVABLE",
  "NOT_APPLICABLE",
]);

const normalizeCoverage = (
  value: unknown,
): ExecutionReport["coverage"] | undefined => {
  if (!value || typeof value !== "object" || Array.isArray(value))
    return undefined;
  const summary = value as Record<string, unknown>;
  const count = (key: string) =>
    typeof summary[key] === "number" && summary[key] >= 0
      ? (summary[key] as number)
      : 0;
  const items = Array.isArray(summary.items)
    ? summary.items.flatMap((value) => {
        if (!value || typeof value !== "object" || Array.isArray(value))
          return [];
        const item = value as Record<string, unknown>;
        if (
          typeof item.check_id !== "string" ||
          typeof item.status !== "string" ||
          !coverageStatuses.has(item.status)
        )
          return [];
        const itemCount = (key: string) =>
          typeof item[key] === "number" && item[key] >= 0
            ? (item[key] as number)
            : 0;
        return [{
          execution_id: itemCount("execution_id"),
          scope_key: typeof item.scope_key === "string" ? item.scope_key : "",
          node_key: typeof item.node_key === "string" ? item.node_key : "",
          contract_key:
            typeof item.contract_key === "string" ? item.contract_key : "",
          contract_version: itemCount("contract_version"),
          check_id: item.check_id,
          category: typeof item.category === "string" ? item.category : "",
          minimum_context:
            typeof item.minimum_context === "string"
              ? item.minimum_context
              : "",
          planned: item.planned === true,
          status: item.status as NonNullable<
            ExecutionReport["coverage"]
          >["items"][number]["status"],
          candidates_generated: itemCount("candidates_generated"),
          candidates_validated: itemCount("candidates_validated"),
          confirmed: itemCount("confirmed"),
          rejected: itemCount("rejected"),
          needs_context: itemCount("needs_context"),
          not_observable: itemCount("not_observable"),
          not_applicable: itemCount("not_applicable"),
          attempts: itemCount("attempts"),
          duration_ms: itemCount("duration_ms"),
        }];
      })
    : [];
  return {
    planned: count("planned"),
    completed: count("completed"),
    incomplete: count("incomplete"),
    confirmed: count("confirmed"),
    needs_context: count("needs_context"),
    not_observable: count("not_observable"),
    items,
  };
};

/** Keeps Studio usable when a compatible server report omits safe progress. */
export const normalizeExecutionReport = (value: unknown): ExecutionReport => {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new APIError("O servidor retornou um relatório de execução inválido.");
  const report = value as Record<string, unknown>;
  if (typeof report.status !== "string" || !report.status.trim())
    throw new APIError("O servidor retornou um relatório de execução inválido.");
  const runs = Array.isArray(report.runs) ? report.runs : [];
  let incomplete = !Array.isArray(report.runs);
  const safeRuns: ExecutionReport["runs"] = [];
  for (const run of runs) {
    if (!run || typeof run !== "object" || Array.isArray(run)) {
      incomplete = true;
      continue;
    }
    const item = run as Record<string, unknown>;
    if (typeof item.node_key !== "string" || !item.node_key || typeof item.status !== "string" || !executionRunStatuses.has(item.status)) {
      incomplete = true;
      continue;
    }
    safeRuns.push({
      node_key: item.node_key,
      status: item.status as ExecutionReport["runs"][number]["status"],
      ...(typeof item.scope_key === "string" && item.scope_key ? { scope_key: item.scope_key } : {}),
    });
  }
  return {
    ...(typeof report.execution_id === "number" ? { execution_id: report.execution_id } : {}),
    status: report.status,
    runs: safeRuns,
    ...(normalizeCoverage(report.coverage)
      ? { coverage: normalizeCoverage(report.coverage) }
      : {}),
    ...(incomplete ? { contractIssue: "A execução foi aceita, mas o servidor retornou progresso incompleto. O status seguro continua visível; atualize o Studio ou contate o administrador." } : {}),
  };
};

export type ExecutionStart = {
  execution_id?: number;
  report?: ExecutionReport;
  error?: string;
  status?: string;
};

const normalizeExecutionStart = (value: unknown): ExecutionStart => {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new APIError("O servidor retornou uma resposta de execução inválida.");
  const result = value as Record<string, unknown>;
  if ("report" in result && result.report !== undefined)
    return {
      ...(typeof result.execution_id === "number" ? { execution_id: result.execution_id } : {}),
      ...(typeof result.status === "string" ? { status: result.status } : {}),
      ...(typeof result.error === "string" ? { error: result.error } : {}),
      report: normalizeExecutionReport(result.report),
    };
  if (typeof result.execution_id !== "number" || typeof result.status !== "string")
    throw new APIError("O servidor retornou uma resposta de execução inválida.");
  return {
    execution_id: result.execution_id,
    status: result.status,
    ...(typeof result.error === "string" ? { error: result.error } : {}),
  };
};

export const getCards = async () =>
  (await request<CardType[]>("/api/cards")).map(normalizeCard);
export const getResponseContracts = () =>
  request<ResponseContract[]>("/api/response-contracts");
export const getReviewChecklists = () =>
  request<ReviewChecklist[]>("/api/review-checklists");
export const getReviewContracts = () =>
  request<ReviewContractVersion[]>("/api/review-contracts");
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
  request<unknown>(`/api/workflow-versions/${versionID}/executions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ trigger_node: triggerNode, payload }),
  }).then(normalizeExecutionStart);
export const executePublishedWorkflow = (
  versionID: number,
  triggerNode: string,
  payload: Record<string, unknown>,
) =>
  request<unknown>(`/api/published-workflow-versions/${versionID}/executions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ trigger_node: triggerNode, payload }),
  }).then(normalizeExecutionStart);
export const getExecution = (executionID: number) =>
  request<unknown>(`/api/executions/${executionID}`).then(normalizeExecutionReport);
export const getExecutionCoverage = (executionID: number) =>
  request<CoverageSummary>(`/api/executions/${executionID}/coverage`);
export const getCardExecutionLogs = (executionID: number, nodeKey: string) =>
  request<CardExecutionLogResponse>(
    `/api/executions/${executionID}/cards/${encodeURIComponent(nodeKey)}/logs`,
  );
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
