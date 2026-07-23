# Feature Map

- POC: preserved under `POC/`.
- Local identity and RBAC: login, logout, and current-user endpoints use
  bcrypt-backed SQLite users and signed, revocable HttpOnly sessions. Explicit
  bootstrap email/password configuration creates only the first admin and never
  modifies a populated users table. Viewers read safe summaries, editors
  save/publish/run drafts, and admins manage integrations, model profiles, and
  users (including the admin user list/create API). The responsive dark login
  gate explains first-run configuration without displaying secrets and separates
  invalid credentials from an unavailable backend.
- Workflow catalog: initial backend foundation implemented.
- Studio canvas: initial visual prototype implemented.
- Studio validation: saves and executes a local card graph, then reflects node
  states from the execution report on the React Flow canvas.
- Integration modal: submits a one-time password-masked Token/API key for Gitea
  and reusable model connections, with no browser storage or later display.
- Connection wizard: validates Gitea, Ollama local/cloud and OpenRouter before
  selection. Gitea selects an organization then only its repositories; LLM setup
  offers searchable multi-select discovered models (no normal-path free text).
  Persisted selections reload in resource-management ModalShell flows.
- Card inspector: edits supported node configuration and selects active Gitea
  or reusable model connections. The review template projects the full initial
  review graph onto the canvas.
- Integrations: create and list Gitea, OpenAI-compatible and Ollama connection
  records using AES-256-GCM ciphertext in SQLite; APIs expose only safe
  `secret_configured` state. Admins validate unsaved connections server-side,
  own safe connection lifecycle, and can discover resources after creation;
  editors can transactionally manage selected active repositories/models, while
  viewers are read-only.
- Frontend routes: `/` is the operational dashboard with safe connection health,
  official review readiness/safety, and recent PR-context execution summaries;
  `/studio` contains the workflow editor; `/integrations` provides the reusable
  connection wizard and connection/model list.
- Integration lifecycle UX: connection details, edit, resource management,
  disable, and delete are focus-managed ModalShell flows with busy, success, and
  sanitized error feedback; modal and spinner transitions respect reduced motion.
- Runner: executes local card graphs by typed inputs and persists node reports;
  active configured Gitea fetch and OpenAI-compatible/Ollama model cards run
  through injected adapters.
- Corrective model retry: a directly connected `model -> validate` pair can
  retry invalid model JSON in the same root or loop-child scope. The model card
  exposes bounded retry limit/delay controls; reports retain attempt/status
  metadata without adding plaintext prompt, response, or secret data.
- Scoped loop runner: `group -> loop` can process each group through downstream
  cards in a separate deterministic scope, then emits terminal-card aggregates.
  The official review template scopes `template -> model -> validate ->
  response_filter` per group and routes `loop.results` to root `consolidate ->
  format -> publish`, so formatting and publication happen once per execution.
  Scope keys are present in execution reports and persisted node runs.
- Controlled review processing: fetched Gitea files can be extension- and
  generated-artifact-filtered, bounded into deterministic groups, and used to
  validate model finding JSON. Valid findings can be severity-filtered,
  deduplicated, consolidated, and emitted as a deterministic formatted-review
  payload.
- Controlled publication: the `publish` card accepts `formatted_review` and
  creates one native Gitea PR review with deterministic inline observations and
  a policy-controlled final event, only through an active configured integration,
  writer adapter, and durable execution-bound idempotency record.
- Async execution: configured Redis dispatches persisted execution IDs to a
  worker; execution status remains queryable while queued, running, completed,
  or failed. Persisted queued work is recoverable when a Redis wake-up is missed.
- Typed triggers: each trigger can be manual, authenticated API, or signed Gitea
  webhook. Manual execution targets a selected card and accepts an optional JSON
  test payload; API execution targets a published workflow and trigger under
  editor/admin RBAC; webhook delivery is HMAC-verified and idempotent. Only the
  selected trigger branch runs, and canonical PR context flows dynamically into
  fetch and publication without requiring fixed PR coordinates.
- Workflow version lifecycle: saves remain new drafts; workflow lists expose
  version number, creation time, and draft/published/archived state. Publishing
  a valid draft atomically archives its workflow's previous published version.
- Official review pipeline: startup seeds and publishes the complete
  `official-gitea-pr-review` graph as initial version 1 only when no published
  version exists for that key. It uses model profiles and safe native-Gitea
  publication defaults; users discover it in `GET /api/workflows` and load its
  version definition through `GET /api/workflow-versions/:id`.
- Pipeline lifecycle UI: `/pipelines` shows grouped versions with distinct
  rascunho/publicada/arquivada states and permits publication only from a draft.
  Studio has separate draft-save and publish controls; publishing saves the
  visible canvas before calling the version publish endpoint.
- Saved-version editing: every Pipeline version has an `Abrir no Studio` entry
  point. `/studio?version=:id` loads its immutable definition, hydrates current
  catalog card data, positions, configuration, and typed edges, and retains the
   original workflow identity so saves create a new draft rather than overwrite
   it. Unsaved canvas changes are visibly marked and warn before browser exit.
- Studio definition management: editors/admins can clone a graph into a distinct
  workflow identity, import/export a `forgereview.workflow` v1 JSON envelope,
  and review an accessible modal preview or validation error before replacing the
  canvas. Import/export contain no browser-persisted state, secrets, or
  ciphertext; viewers cannot modify, transfer, save, publish, or execute graphs.
- Generic card error policy: cards support fail, continue, partial, or an
  explicit typed error route. The Studio exposes the policy and only shows the
  error handle while route is selected; error paths are distinct on the canvas.
  `error_control` handles routed typed errors as terminal fail, continue, or a
   fallback output, including inside loop child scopes.
- Studio graph management: editors/admins get accessible edit/delete controls
  in each card header. Edit alone opens the otherwise-hidden Inspector; delete,
  the overflow removal action, and Delete/Backspace remove immediately, with
  incident edges removed alongside cards. Pane clicks close editing without
  coupling ordinary selection to the Inspector. Viewers receive no active card
  edit/delete controls. Before save,
  publish, or execution, Studio displays local validation status and lists locally knowable identity, graph,
  configuration, route, port, and contract issues and focuses the related card
  when possible; backend validation remains authoritative.
- Studio header and manual run: workflow identity and status remain compact,
  while publish and save/run stay visible. Template, navigation, integration,
  transfer, validation, selection removal, and draft-save actions share one
  overflow menu. Optional manual-trigger JSON is entered in a focus-managed run
  modal and is never embedded in persisted card data.
- Reusable model profiles: integrations retain provider transport and encrypted
  credentials; profiles select the model and reference an active LLM connection.
  Model cards select profiles, and the runner resolves the profile in memory
  before calling the controlled provider adapter.
- Cache card: reads, JSON-writes with a required 1–86,400 second TTL, or deletes
  Redis values through an explicit runner adapter. Cache misses emit a nil value;
  deletes emit no output. Its Inspector provides key, mode, and write-TTL fields.
- Safe execution summaries: `GET /api/executions?limit=10` exposes bounded
  status/timestamp/workflow metadata and canonical runtime PR context only;
  it excludes node data, errors, execution inputs, and secret material.
- Accessible selection controls: a shared polished switch replaces native
  checkbox presentation across Inspector booleans and repository/model
  multi-selection. Resource rows show provider identity, selection count,
  hover/selected state, focus, disabled behavior, and reduced-motion support.
- Truthful card contracts: publication event/autonomous-rejection settings live
  only on `publish`; model `max_tokens` is bounded and reaches both provider
  adapters; `merge` waits for all active incoming edges through `collect_all`;
  and unsupported `workflow`/subpipeline is visible but unavailable and cannot
  be saved; its library control shows an explicit unavailable status and reason.
