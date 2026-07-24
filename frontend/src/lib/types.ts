export type Port = {
  key: string;
  label: string;
  contract: string;
  required: boolean;
  collect_all?: boolean;
};

export type CardType = {
  key: string;
  name: string;
  category: string;
  description: string;
  inputs: Port[];
  outputs: Port[];
  error_output?: Port;
  available?: boolean;
  unavailable_reason?: string;
};

export type ResponseContract = {
  key: string;
  name: string;
  description: string;
  version: number;
  schema: Record<string, unknown>;
  editable: false;
};

export type ReviewCheck = {
  check_id: string;
  description: string;
  category: string;
  minimum_context: string;
};

export type ReviewChecklistSnapshot = {
  key: string;
  name: string;
  version: number;
  items: ReviewCheck[];
};

export type ReviewChecklist = ReviewChecklistSnapshot & {
  description: string;
  editable: false;
};

export type ReviewContractVersion = {
  id: number;
  key: string;
  version: number;
  name: string;
  description: string;
  response_schema: Record<string, unknown>;
  checklist: ReviewChecklistSnapshot;
  official: boolean;
};

export type CardStatus =
  | "idle"
  | "running"
  | "completed"
  | "failed"
  | "partial";

export type CardData = {
  key: string;
  type: string;
  name: string;
  category: string;
  inputs: Port[];
  outputs: Port[];
  errorOutput?: Port;
  config: Record<string, unknown>;
  status: CardStatus;
};

export type WorkflowDefinition = {
  key: string;
  name: string;
  description: string;
  interface?: {
    trigger_node_key?: string;
    inputs: WorkflowInterfaceField[];
    outputs: WorkflowInterfaceField[];
  };
  nodes: Array<{
    key: string;
    type: string;
    name: string;
    config: Record<string, unknown>;
    position: { x: number; y: number };
  }>;
  edges: Array<{
    key: string;
    from_node: string;
    from_port: string;
    to_node: string;
    to_port: string;
  }>;
};

export type WorkflowInterfaceField = {
  key: string;
  label: string;
  contract: string;
  required?: boolean;
  node_key?: string;
  port_key?: string;
};

export type WorkflowMetadata = Pick<
  WorkflowDefinition,
  "key" | "name" | "description" | "interface"
>;

export type WorkflowExportEnvelope = {
  format: "forgereview.workflow";
  version: 1;
  definition: WorkflowDefinition;
};

export type WorkflowVersionStatus = "draft" | "published" | "archived";

export type WorkflowVersionSummary = {
  version_id: number;
  version: number;
  status: WorkflowVersionStatus;
  created_at: string;
};

export type WorkflowSummary = {
  key: string;
  name: string;
  description: string;
  versions: WorkflowVersionSummary[];
};

export type PublishedWorkflow = {
  version_id: number;
  definition: WorkflowDefinition;
};

export type ExecutionReport = {
  execution_id?: number;
  status: string;
  runs: Array<{
    node_key: string;
    scope_key?: string;
    status: "running" | "completed" | "failed" | "partial";
  }>;
  coverage?: CoverageSummary;
  /** Present only when a compatible safe report could not be fully decoded. */
  contractIssue?: string;
};

export type CoverageRecord = {
  execution_id: number;
  scope_key: string;
  node_key: string;
  contract_key: string;
  contract_version: number;
  check_id: string;
  category: string;
  minimum_context: string;
  planned: boolean;
  status:
    | "PLANNED"
    | "COMPLETED"
    | "CONFIRMED"
    | "REJECTED"
    | "NEEDS_CONTEXT"
    | "NOT_OBSERVABLE"
    | "NOT_APPLICABLE";
  candidates_generated: number;
  candidates_validated: number;
  confirmed: number;
  rejected: number;
  needs_context: number;
  not_observable: number;
  not_applicable: number;
  attempts: number;
  duration_ms: number;
};

export type CoverageSummary = {
  planned: number;
  completed: number;
  incomplete: number;
  confirmed: number;
  needs_context: number;
  not_observable: number;
  items: CoverageRecord[];
};

export type ExecutionEvent = {
  id: number;
  execution_id: number;
  kind: "execution" | "node";
  status: string;
  node?: { node_key: string; scope_key?: string; status: CardStatus };
  created_at: string;
};

export type ExecutionCardLog = {
  id: number;
  execution_id: number;
  node_key: string;
  scope_key?: string;
  status: string;
  started_at?: string;
  finished_at?: string;
  duration_ms?: number;
  error?: string;
  facts?: Record<string, unknown>;
  inputs?: unknown;
  outputs?: unknown;
};

export type CardExecutionLogResponse = {
  events: ExecutionEvent[];
  entries: ExecutionCardLog[];
};

export type Integration = {
  key: string;
  name: string;
  type: "gitea" | "openai" | "ollama";
  config: { base_url: string; model?: string };
  secret_configured: boolean;
  status: "active" | "disabled";
};

export type NewIntegration = Omit<Integration, "secret_configured"> & {
  secret: string;
};

export type ModelProfile = {
  key: string;
  name: string;
  integration_key: string;
  model: string;
  status: "active" | "disabled";
};

export type Repository = {
  integration_key: string;
  owner: string;
  name: string;
};
export type Discovery = {
  organizations?: string[];
  repositories?: Repository[];
  models?: string[];
};

export type ExecutionSummary = {
  execution_id: number;
  status: "queued" | "running" | "completed" | "failed" | string;
  started_at: string;
  finished_at?: string;
  workflow: { key: string; name: string; version: number };
  review?: { owner: string; repo: string; pull_request: number };
};

export type WebhookRegistration = {
  key: string;
  name: string;
  workflow_key: string;
  trigger_node_key: string;
  active: boolean;
  secret_configured: boolean;
};
