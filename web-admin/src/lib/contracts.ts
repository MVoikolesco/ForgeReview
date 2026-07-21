export type Provider = {
  id: number;
  name: string;
  display_name: string;
  base_url: string;
  auth_type: string;
  is_enabled: number;
  is_default: number;
};

export type Connection = {
  id: number;
  provider_id: number;
  name: string;
  base_url: string;
  api_key_configured: boolean;
  organization_id: string;
  project_id: string;
  http_referer?: string;
  app_title?: string;
  is_default: number;
  is_enabled: number;
  created_at: string;
  updated_at: string;
};

export type Model = {
  id: number;
  connection_id: number;
  provider_model_name: string;
  display_name: string;
  context_window: number;
  max_output_tokens: number;
  supports_json: number;
  supports_tools: number;
  supports_streaming: number;
  is_default: number;
  is_enabled: number;
};

export type Profile = {
  id: number;
  name: string;
  description: string;
  model_id: number | null;
  is_default: number;
  is_enabled: number;
};

export type CatalogModel = {
  id: string;
  name: string;
  context_length: number;
  max_completion_tokens: number;
  supported_parameters: string[];
  prompt_price?: string;
  completion_price?: string;
  size?: number;
};

export type ConnectionData = {
  providers: Provider[];
  connections: Connection[];
  models: Model[];
  profiles: Profile[];
};

export type SetupDraft = {
  provider: "ollama" | "ollama-cloud" | "openrouter";
  connection: {
    name: string;
    base_url: string;
    api_key: string;
    http_referer: string;
    app_title: string;
  };
  model: CatalogModel | null;
  parameters: {
    temperature: number;
    top_p: number;
    max_output_tokens: number;
    repeat_penalty: number;
    num_ctx: number;
    num_threads: number;
    num_predict: number;
    timeout_seconds: number;
    keep_alive: string;
    unload_model_after_review: boolean;
  };
  profile: { name: string; description: string };
  policy: {
    max_block_chars: number;
    max_files_per_block: number;
    review_concurrency: number;
    review_final_retries: number;
    review_wip_pull_requests: boolean;
    review_own_pull_requests: boolean;
    publish_manual_reviews: boolean;
    allow_autonomous_rejection: boolean;
    enable_detailed_stage_logs: boolean;
    unload_model_after_review: boolean;
  };
};

export type GiteaInstance = {
  id: number;
  name: string;
  base_url: string;
  bot_username: string;
  is_enabled: number;
  is_default: number;
};
export type GiteaOrganization = { id: number; name: string; full_name: string };
export type GiteaRepository = {
  id: number;
  name: string;
  full_name: string;
  owner: { login: string };
  private: boolean;
};
export type GiteaPullRequest = {
  number: number;
  title: string;
  body: string;
  state: string;
  html_url?: string;
  user?: { login: string };
};

export type ReviewSettingsPolicy = {
  id: number;
  max_block_chars: number;
  max_files_per_block: number;
  publish_manual_reviews: boolean;
  allow_autonomous_rejection: boolean;
  enable_detailed_stage_logs: boolean;
  context_safety_tokens: number;
  minimum_confidence: number;
  max_parallel_groups: number;
  medium_severity_event: string;
  partial_event: string;
};

export type ReviewSettingsPrompt = {
  id: number;
  name: string;
  type: string;
  stack: string;
  content: string;
  version: number;
  is_active: boolean;
  updated_at: string;
};

export type ReviewSettingsProfile = {
  id: number;
  name: string;
  description: string;
  is_default: boolean;
  is_enabled: boolean;
  created_at: string;
  updated_at: string;
  model?: {
    id: number;
    name: string;
    provider: string;
    connection: string;
    is_ready: boolean;
  };
  policy?: ReviewSettingsPolicy;
  prompt?: ReviewSettingsPrompt;
};

export type ReviewSettingsStage = {
  id: number;
  key: string;
  name: string;
  position: number;
  stage_type_key: string;
  executor_key: string;
  prompt_template: string;
  model_id?: number;
  model_name?: string;
  is_model_ready: boolean;
  max_output_tokens: number;
  retry_limit: number;
  timeout_seconds: number;
  use_llm: boolean;
  is_required: boolean;
  is_type_enabled: boolean;
  input_contract?: string;
  output_contract?: string;
};

export type ReviewSettingsPipeline = {
  id: number;
  profile_id?: number;
  profile_name?: string;
  key: string;
  name: string;
  description: string;
  is_default: boolean;
  is_enabled: boolean;
  version_id: number;
  version: number;
  status: string;
  published_at: string;
  stages: ReviewSettingsStage[];
};

export type ReviewSettingsContract = {
  id: number;
  key: string;
  version: number;
  response_instruction: string;
  schema: Record<string, unknown>;
  semantic_validator_key: string;
};

export type ReviewSettingsStageType = {
  id: number;
  key: string;
  name: string;
  executor_key: string;
  processor_kind?: string;
  is_system: boolean;
  is_enabled: boolean;
  input_contract?: ReviewSettingsContract;
  output_contract?: ReviewSettingsContract;
};

export type ReviewSettingsData = {
  profiles: ReviewSettingsProfile[];
  pipelines: ReviewSettingsPipeline[];
  stage_catalog: ReviewSettingsStageType[];
};
