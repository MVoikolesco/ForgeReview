export type Port = {
  key: string;
  label: string;
  contract: string;
  required: boolean;
};

export type CardType = {
  key: string;
  name: string;
  category: string;
  description: string;
  inputs: Port[];
  outputs: Port[];
  error_output?: Port;
};

export type CardStatus = "idle" | "running" | "completed" | "failed" | "partial";

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

export type ExecutionReport = {
  status: string;
  runs: Array<{
    node_key: string;
    status: "completed" | "failed" | "partial";
    error?: string;
  }>;
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
