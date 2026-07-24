# Feature Map

- Studio interaction completion: searchable accessible card addition and drag/drop
  (arrow navigation, Enter activation, Escape close) plus canvas search
  (Enter focus, Escape close), contextual canvas tools
  for cards, edges, and panes, safely positioned dismissible context menus,
  dark themed React Flow navigation controls, safe status progress, anchored
  card editing, semantic undo/redo, and bounded loop concurrency (1–4). See
  [[Architecture]].

- POC: preserved under `POC/`.
- Local identity and RBAC: login, logout, and current-user endpoints use
  bcrypt-backed SQLite users and signed, revocable HttpOnly sessions. Explicit
  bootstrap email/password configuration creates only the first admin and never
  modifies a populated users table. Viewers read safe summaries, editors
  save/publish/run drafts, and admins manage integrations, model profiles, and
  users (including the admin user list/create API). The responsive dark login
  gate explains first-run configuration without displaying secrets and separates
   invalid credentials from an unavailable backend.
- Security operations: admins use `/admin` and protected user/audit APIs to
  create users, adjust roles, and activate/deactivate accounts. Self-destructive
  changes and removal of the final active admin are blocked; authorization
  changes revoke sessions. Safe audit records cover these administrative changes
  and sensitive lifecycle actions. CORS origins and proxy trust are explicit
  environment allowlists.
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
- Card editor: one Studio-owned dialog popover edits supported node configuration
  and selects active Gitea or reusable model connections. Card controls, context
  menus, and validation issues open the same editor; it flips/clamps beside its
  React Flow card, retains draft edits on dismissal, and becomes a narrow-screen
  bottom sheet. The review template projects the full initial review graph onto
  the canvas.
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
- Shared navigation: standard authenticated pages use a responsive AppShell with
  active route state, semantic truncating breadcrumbs, per-page action slots, and
  role-aware visibility (viewer: overview/pipelines; editor: Studio; admin:
  integrations/administration). Studio retains its dense toolbar and compact
  return affordance. Dashboard and Administration now present operational
  readiness, lifecycle, user, audit, loading, error, and empty states in the
  same visual system.
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
    failed, cancelled, or dead-lettered. Transient failures retry at most three
    times; permanent/uncertain and exhausted failures enter DLQ, where an admin
    can create an audited replay execution. Due and stale-running work is
    recoverable when a Redis wake-up is missed.
- Publication reconciliation: transport/5xx/response ambiguity becomes a durable
  `uncertain` record. Admin reconciliation looks up the Gitea idempotency marker,
  completing a match or permitting retry only after an absent marker.
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
- Pipeline version deletion and published Studio defaults: administrators can
  confirm definitive deletion for each draft or archived version from
  `/pipelines`; published versions require another version to be published
  first. Retained execution, publication, webhook, or audit evidence blocks it
  with a dependency-specific message.
  Published entries open by workflow key and Studio resolves the current
  published version. Bare `/studio` opens the official published review pipeline
  when available, otherwise retains a safe local empty canvas.
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
  while publish and save/run stay visible. Template, role-appropriate Pipelines
  and integration management, transfer, validation, selection removal, and
  draft-save actions share one accessible overflow menu. The shared header uses
  reduced responsive edge padding and collapses navigation before compact
  desktop/tablet widths to preserve the Studio controls. Optional manual-trigger
  JSON is entered in a focus-managed run modal and is never embedded in
  persisted card data.
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
- Live execution feedback: SQLite stores safe running/terminal node progress;
  Studio applies intermediate polling reports, animates only edges entering the
  active card, and distinguishes running, completed, partial, and failed cards
  with restrained badges and borders.
- Informative Gitea reviews: publication includes status, elapsed time, model,
  optional provider-reported token usage, deterministic finding summary, final
  completion text, and the existing idempotency marker. Unknown telemetry is
  omitted. The final manual-trigger experience remains pending user modelling.
- Accessible selection controls: a shared polished switch replaces native
  checkbox presentation across Inspector booleans and repository/model
  multi-selection. Resource rows show provider identity, selection count,
  hover/selected state, focus, disabled behavior, and reduced-motion support.
- Complete card contracts: publication settings remain on `publish`; model
  sampling, timeout, fallback, and cost reservation reach controlled adapters;
  transform operations and variable namespaces are bounded; conditions expose
  eight match branches; merge supports all/any/quorum and partial timeout; and
  `workflow` runs an immutable published version through its named interface.
- Configurable response contracts: `validate` can embed a bounded Draft 2020-12
  JSON Schema, while an immutable backend catalog supplies findings, generic
  object/array, and review-summary presets. Studio can select, format, restore,
  or duplicate a preset and blocks invalid workflow actions; the backend
  recompiles and enforces the contract. Workflows without a schema retain the
  legacy `Finding[]` path.
- Verifiable findings: the official reviewer emits `CandidateFinding[]`; a
  separately configured `candidate_validator` evaluates candidates one at a
  time and exposes safe decision counts. Only `CONFIRMED` becomes a publishable
  finding. Rejections, context needs, and unobservable candidates remain in
  internal retained execution evidence.
- Stable finding identity: fetch propagates repository/base-commit identity,
  the system creates semantic SHA-256 fingerprints, and duplicate candidates
  are removed before validator cost and again at root consolidation. Studio
  execution facts expose only the duplicate count, not candidate content.
- Versioned review checklists: an immutable backend catalog supplies closed
  check sets, Studio copies a selected version into reviewer/validator cards,
  and runtime rejects unlisted check IDs without another model call.
