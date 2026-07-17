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
  api_key_env_name: string;
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
    api_key_env_name: string;
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
    log_sensitive_data: boolean;
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
