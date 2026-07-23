# ForgeReview Workflow Studio Architecture

## Foundation

`backend/` is a standalone Gin application. `frontend/` is a standalone Next.js
Studio. SQLite is the source of truth; the store boundary isolates SQL from
workflow domain and HTTP handlers. Redis carries only execution IDs;
the execution input remains in SQLite.

## Workflow contract

## Studio interaction model

Studio keeps client-only serializable definition history for semantic graph edits.
Selection, menus, viewport movement, and safe execution-status updates are not
history entries. The searchable card library supports accessible click-add and
native drag/drop converted with React Flow's `screenToFlowPosition`. Card editing
uses a focus-managed responsive modal, independent of canvas selection. Card
progress uses only safe node status, and loop concurrency is bounded from 1 to 4.

A workflow version stores a graph of nodes and typed edges. A card type is
registered by the backend and supplies its ports and capabilities; a saved
workflow only selects a registered type and its configuration. The initial
catalog includes the requested 20 cards grouped by input, data,
transformation, control, AI, validation, result, output and infrastructure.

The backend validates node keys, card types, declared ports and contract
compatibility before creating a draft version. Each save creates a new draft
version; no API mutates a saved definition. Publishing validates the stored
definition again and atomically promotes that draft while archiving the prior
published version for the same workflow key. Only one published version can
exist per workflow key.

Studio can clone, import, and export definitions without a new API. Export uses
the versioned `forgereview.workflow` v1 JSON envelope containing only a graph
definition. Import validates the envelope, catalog availability, node/edge keys,
typed ports, and credential-like configuration fields locally before replacing
the canvas; draft saves repeat graph validation on the server and reject secret,
ciphertext, password, token, or API-key fields. Clone obtains existing workflow
keys, creates a distinct `-copy` identity/name, and only suffixes duplicate
node or edge keys. These actions are available to editors and admins only.

Startup calls the store's idempotent official-review seed. It creates and
publishes `official-gitea-pr-review` version `1` only when no published version
exists for that key; it never modifies user workflow keys or an existing
official version. The definition is the full scoped review graph and uses the
same `model_profile` convention as the Studio. Its Gitea `publish` card keeps
`allow_autonomous_rejection: false` and `medium_severity_event: "COMMENT"` by
default. `GET /api/workflows` identifies the published version and
`GET /api/workflow-versions/:id` returns its immutable graph.

## Current API

### Local identity, sessions, and roles

ForgeReview uses SQLite `users` records with bcrypt password hashes and the
`viewer`, `editor`, and `admin` roles. Compose requires both
`FORGEREVIEW_BOOTSTRAP_ADMIN_EMAIL` and
`FORGEREVIEW_BOOTSTRAP_ADMIN_PASSWORD`; use a trimmed, lowercase email address.
On an empty users table, startup creates exactly that configured email as the
first `admin`. The password is used to produce a bcrypt hash and is not
persisted. Startup fails on an empty users table when either value is absent.

Bootstrap is first-run-only. Once any row exists in `users`, startup does not
create, rename, reset, or otherwise overwrite a user, even if either bootstrap
environment value changes. This is intentional and means there is no hidden
default administrator or password-reset path. For routine recovery, sign in as
another administrator and create a replacement user through `POST /api/users`.
If all administrator access is lost, an operator must first take a backup and
perform a deliberate, documented local-database recovery (remove the user and
session records only when a complete identity reset is intended), then restart
with the chosen explicit bootstrap email and password. Existing workflows and
integrations are not bootstrap credentials and must not be used as one.

The browser receives an `HttpOnly`, `SameSite=Lax` session cookie with explicit
`Expires` and `Max-Age` attributes. Its opaque
session nonce is stored in SQLite, while its user ID, expiry, and nonce are
HMAC-SHA-256 signed with `FORGEREVIEW_SESSION_SIGNING_KEY` (minimum 32
characters). Sessions expire after eight hours by default (or the positive Go
duration in `FORGEREVIEW_SESSION_TTL`) and logout deletes the server-side nonce.
This supports local HTTP hosting without browser token persistence. Gin runs in
release mode unless `FORGEREVIEW_GIN_MODE=debug` is explicit and trusts no
forwarded proxy headers by default. Deployments behind a reverse proxy must set
`FORGEREVIEW_TRUSTED_PROXIES` to only its IP addresses/CIDRs; invalid values fail
startup. Deployments
behind HTTPS should terminate TLS at the reverse proxy and set the cookie secure
attribute there before exposing the instance beyond localhost.

`POST /api/auth/login`, `POST /api/auth/logout`, and `GET /api/auth/me` provide
the session lifecycle. All application APIs require a session. Viewers may read
safe dashboard, pipeline, version, integration, model-profile, and execution
summaries. Editors may additionally save drafts, publish drafts, start
executions, and replace selected resources on active existing connections.
Only admins may create, edit, disable, or delete connections and users.

- `GET /health`: backend health.
- `GET /api/cards`: registered card catalog and typed ports.
- `POST /api/integrations`: create an active or disabled Gitea, OpenAI-compatible
  or Ollama integration. Config is limited to `base_url` and, for models,
  `model`; `secret` is a one-time Token/API key input and is never returned.
- `GET /api/integrations`: list safe integration summaries with
  `secret_configured`. Ciphertext and credential values are never returned.
- `POST /api/integrations/validate`: admin-only server-side validation of an
  unsaved connection. The provider call is bounded to ten seconds and returns
  only sanitized success/failure plus safe Gitea organization names or discovered
  LLM model names; neither candidate nor secret is persisted.
- `POST /api/integrations/discover-repositories`: admin-only, unsaved Gitea
  discovery for one selected organization. It revalidates the one-time candidate
  and returns repositories only for that organization.
- `GET /api/integrations/:key/discover`: admin/editor-only active-connection
  discovery. A Gitea request without `organization` returns organization names;
  `?organization=<name>` returns only that organization's repositories. LLM
  requests return models. `PUT` resource endpoints transactionally replace
  selections; LLM selections become reusable profiles.
- `GET/PATCH /api/integrations/:key`, `POST /disable`, and `DELETE` provide
  safe detail and admin lifecycle actions. PATCH preserves ciphertext when
  `secret` is omitted; delete conflicts when workflow history references it.
- `POST /api/workflows`: validate and persist a new draft version.
- `GET /api/workflows`: list workflow keys with their latest name/description
  and all version summaries (`version_id`, `version`, `created_at`, and
  `status`). Status is one of `draft`, `published`, or `archived`.
- `GET /api/workflow-versions/:id`: load an immutable saved definition.
- `POST /api/workflow-versions/:id/publish`: atomically publish a draft and
  archive the previous published version for that key. It returns the published
  version summary. A missing version returns `404`, a non-draft returns `409`,
  and an invalid persisted definition returns `422` without changing statuses.
- `POST /api/workflow-versions/:id/executions`: create an execution for a draft
  definition (Studio save-and-run). Without a dispatcher it runs synchronously and returns a
  completed report. With a dispatcher it enqueues the execution ID and returns
  `202 Accepted` with `{execution_id,status:"queued"}`.
- `POST /api/published-workflow-versions/:id/executions`: execute an opened
  published version without saving a draft. Draft and archived versions return
  `409`; both Studio routes require a selected manual trigger.
- `GET /api/executions/:id`: load safe persisted execution and per-node lifecycle
   states only; raw inputs and outputs are excluded.
- `GET /api/executions?limit=10`: return at most 1–100 dashboard-safe execution
  summaries (default 10). Each summary has execution status/timestamps, workflow
  identity/version, and PR coordinates only when matching fetch/publish cards in
  the stored version configure them. It never returns execution input, nodes,
  errors, integration configuration, or secrets.

## Execution Foundation

The workflow runner starts cards without required inputs, forwards typed tokens
through declared edges, and starts a downstream card only after all required
ports receive a token. Tokens and node reports carry `scope_key`; ordinary
execution uses `root`. Each completed or failed card is persisted in the
execution report. Node progress is also persisted as `running` and updated to
its terminal state, allowing the polling status API to expose safe live progress
without returning prompts, provider bodies, or secrets. The safe local executors
currently cover trigger, transform,
variable, log, cache, filter, group, loop, template, condition, merge, validate,
response_filter, consolidate and format. A collecting input port waits for all
of its declared incoming edges; `merge.inputs` and `consolidate.comments` use
this behavior. Merge emits the ordered list of all branch values instead of
running after the first branch. The catalog exposes `workflow`/subpipeline as
    unavailable, with a reason, and validation prevents it from being saved until
    an execution contract exists. A `fetch` card requires only
`config.integration`; its required typed event input supplies owner, repository,
and pull-request number. It uses the injected Gitea adapter to read PR metadata,
file changes and diff. A `model`
card requires `config.integration` and uses the injected OpenAI-compatible or
   Ollama chat adapter. Its optional `max_tokens` is validated from 1 through
   128,000 (default 2,000) and reaches OpenAI-compatible `max_tokens` or Ollama
   `options.num_predict`. Both require an active stored integration and decrypt
   its configured credential only immediately before the controlled provider
   request.

### Integration secret storage

The backend requires `FORGEREVIEW_ENCRYPTION_KEY` at startup. It must be the
canonical base64 encoding of exactly 32 bytes; the key is loaded only from that
environment variable. Integration Token/API keys are AES-256-GCM encrypted with
a random nonce and their integration key as authenticated additional data before
SQLite persistence. The ciphertext is not JSON-serializable and never appears in
API responses. The workflow runtime uses the encrypted-secret manager only at
provider execution; it does not log plaintext.

Existing Studio databases with `integrations.secret_reference` are migrated by
rebuilding that table without the reference column. Those records remain listed
but unconfigured because a reference cannot be converted into a credential;
their administrator must submit a new Token/API key.

### Scoped loop execution

`loop.items` accepts a list or a `group.groups` output. It requires a positive
integer `max_iterations`; inputs above that bound fail before any child scope is
started. `concurrency` defaults to `1` and is bounded at `4`. The
validated `max_iterations`, `concurrency`, `on_error`, and completed/failed
iteration counts are retained in loop node-run metadata. Child scopes receive
deterministic keys of the form `<loop-node-key>:000001`, in input order.
Aggregation remains in that order even when child scopes run concurrently. A
runner limits card work to eight concurrent operations and serializes `fetch`,
`model`, and `publish` calls because current adapters have no provider-specific
ordering policy.

Each `loop.item` edge receives one token in its child scope, and every
downstream token remains in that scope; the graph remains a single visual graph.
After all child scopes have finished, `loop.results` emits one root-scoped list
containing outputs from each terminal scoped card in child-scope and branch
order. Its target is an explicit scope boundary: nodes reached through
`loop.results` resume in `root`, rather than being marked as scoped descendants.
The official review graph therefore makes `response_filter` terminal per group,
then sends `loop.results` to `consolidate.comments`; nested finding lists are
flattened deterministically before consolidation. `consolidate`, `format`, and
`publish` each run once in `root`. `on_error` defaults to `fail`, which stops
the loop and fails the execution on the first child error. With
`on_error: "partial"`, the failed child node-run remains failed, later scopes
continue, and `results` contains only terminal outputs that completed before
failures. Nested loops are not supported in this increment.

## Controlled PR-review cards

The cards following `fetch` and `model` are local, deterministic transforms;
they do not create comments or call an external destination.

- `filter` consumes fetched `files`. `include_extensions` and
  `exclude_extensions` are optional extension lists (with or without the leading
  dot); exclusion takes precedence. `ignore_generated` defaults to `false` and,
  when enabled, excludes files explicitly marked `generated`/`is_generated` and
  known generated paths or artifacts (`vendor`, `node_modules`, `dist`, `build`,
  `coverage`, minified, protobuf, generated and lock files). Results are ordered
  by filename.
- `group` requires positive `max_files` and `max_characters`; it creates stable
  filename-ordered groups. A file's `patch`, then `content`, then `diff` string
  determines its character count, falling back to its filename when Gitea did
  not provide changed text. `group_by_extension` defaults to `false`; when true,
  groups are partitioned and ordered by normalized extension.
- `validate` parses the model response as exactly one JSON array of findings.
  Every finding requires nonblank `path` and `comment`, a positive `line`, and
  severity `low`, `medium`, `high`, or `critical`. It emits the finding list on
  `valid` or a structured error list on `invalid`. With `validate_paths: true`,
  it also requires the optional `files` input and rejects paths absent from that
  fetched list.
- `response_filter` accepts validated lists, applies optional
  `minimum_severity` (default `low`), and removes exact duplicate findings.
  `consolidate` combines incoming finding lists, including the nested aggregate
  emitted by `loop.results`, and applies the same deterministic deduplication.
  Finding order is path, line, descending severity, then comment.
- `format` emits `formatted_review`: a destination-neutral payload with the
  sorted findings, fixed summary counters (`total`, `low`, `medium`, `high`,
  `critical`), a proposed event/status, and inline observations (`path`, `body`,
  `new_position`) derived from each validated finding. High or critical findings
  propose `REQUEST_CHANGES`; all other findings propose `COMMENT`.
- `publish` consumes `formatted_review` and a required typed `pull_request`
  target. `fetch` accepts the canonical target from its trigger event and emits
  that target with the fetched PR. Fixed `owner`, `repo`, and `pull_request`
  configuration is rejected for both cards. The integration must be active
  and Gitea. Its writer creates one native Gitea PR review at
  `pulls/{number}/reviews`, with the final event and inline comments. High and
  critical proposals are downgraded to `COMMENT` unless
  `allow_autonomous_rejection` is true. Medium findings use `COMMENT` unless
  `medium_severity_event` is `REQUEST_CHANGES`. Both controls belong only to
  `publish`; definitions that place them on `fetch` are rejected. Approval is never automated.
  Arbitrary URLs, bodies, and credentials cannot be supplied by the workflow.
  The review body includes a Portuguese status and finding summary, elapsed
  execution time, resolved model identity, and token totals/breakdown when the
  provider supplies usage. Unknown telemetry is omitted rather than inferred;
  the durable idempotency marker remains in the footer.

## Publication idempotency

Before the Gitea request, SQLite creates a unique publication attempt keyed by
the workflow execution ID, workflow version ID, publish node key, and (for a
scoped run) its scope key. Root-scope keys retain the prior format. Attempts
move from `pending` to `completed` with a safe provider receipt, or to
`retryable` after a known failure. A duplicate completed execution returns the
stored receipt instead of posting again; a duplicate pending attempt does not
post. The Gitea writer also sends the key in `X-ForgeReview-Idempotency-Key` and
as an HTML comment marker. Credentials and provider response bodies are never
stored in the attempt record or returned by APIs. Manual approval and recovery
or reconciliation of an uncertain external review result remain out of scope.

## Asynchronous execution dispatch

## Operational execution status

SQLite persists safe execution/node lifecycle events before SSE delivery.
`GET /api/execution-events` supplies Dashboard activity and
`GET /api/executions/:id/events` supplies Studio activity; both support replay
through `Last-Event-ID`. Status/event payloads contain only IDs, status,
timestamps, node keys, and scope keys. Raw trigger input and node payloads reside
in separate sensitive tables and are purged after seven days. Reprocess creates a
new queued execution from retained input and must never be described as resume.

When `FORGEREVIEW_REDIS_URL` is set, startup verifies Redis and starts a worker
using its list-backed execution-ID queue. The worker atomically claims a queued
SQLite execution before running it, making duplicate queue deliveries harmless.
Before the worker starts, every still-queued SQLite execution ID is re-enqueued;
the persisted selected trigger and input therefore survive process restarts and
temporary enqueue failures.
Transient worker failures retry at most three times with deterministic exponential
backoff plus bounded jitter. Permanent and uncertain failures, and exhausted
transient retries, become `dead_letter`; only an admin can create a new replay
execution through `POST /api/executions/:id/replay`, which is audited. Startup
and periodic recovery requeue due work and running work stale for 15 minutes.
`GET /api/executions/:id` remains the status API and reports `queued`, `running`,
`completed`, `cancelled`, or `dead_letter` plus completed node reports. If Redis is unavailable,
startup fails unless local development explicitly sets
`FORGEREVIEW_ALLOW_IN_PROCESS_QUEUE=true`; that opt-in fallback is non-durable
and is also used by tests. Leaving `FORGEREVIEW_REDIS_URL` unset preserves the
synchronous API behavior.

## Containers

`docker compose up --build` runs the new Studio at `http://localhost:3010` and
the backend health endpoint at `http://localhost:8088/health`. SQLite state is
stored in the `workflow-data` volume. Redis is available only to the new
application on the internal Compose network and dispatches execution IDs to the
backend worker.

The production frontend image runs `next start` from the compiled `.next`
artifact. It intentionally does not run `next dev`, because the final image
does not contain source files and uses `NODE_ENV=production`.

Copy `.env.example` to `.env` for local Compose configuration and generate the
required encryption key with `openssl rand -base64 32` and a separate session
signing key with `openssl rand -base64 48`. Before the first start, explicitly
set the bootstrap administrator email and a strong password; Compose passes all
four setup values to the backend. Never put a bootstrap password in source
control or documentation. No provider-specific integration credentials are
environment configuration.

## Studio lifecycle and validation flow

The Studio loads the card catalog from `GET /api/cards`, lets an administrator
add cards and draw port connections, and saves the current canvas as a new
workflow version. `Salvar rascunho` creates a draft. `Publicar` first creates a
new draft from the current canvas and only reports publication after
`POST /api/workflow-versions/:id/publish` succeeds. `Salvar e executar` retains
the local verification flow and paints each card as draft, running, completed or
failed from the persisted execution report. With Redis dispatch enabled, the
Studio polls the execution endpoint until the worker completes it.

`/` is the operational dashboard. It loads backend health, safe connections,
workflow summaries, model profiles, and safe execution summaries in parallel,
then loads the published official definition when available. It presents only
published graph labels and conservative publish-policy/readiness indicators;
it never renders stored configuration or credentials. `/pipelines` lists the grouped workflow summaries from `GET /api/workflows`.
It displays all draft, published, and archived versions and exposes publication
only for draft versions; after a successful response it reloads the lifecycle
list so the archived and published states are current. Each listed version has
an `Abrir no Studio` link to `/studio?version=:id`. Studio loads that immutable
definition through `GET /api/workflow-versions/:id`, reconstructs React Flow
cards from the current catalog (including persisted names, configuration,
positions, and typed edge handles), and retains its workflow identity for later
saves. An opened version is explicitly identified as immutable: every save,
publish, or run saves a new draft, never updates the source version. The Studio
marks edited canvases as unsaved and warns on browser exit. Header navigation
links connect Studio, Pipelines, and Integrations.

The `Integrações` control opens the Studio connection modal. It lists safe
integration summaries and creates Gitea, OpenAI-compatible or Ollama records
through the integration API. After validation, Gitea requires an organization
selection before it discovers and multi-selects only that organization's
repositories. Ollama/OpenRouter instead show a searchable multi-select of
discovered models; free-text model entry is not part of the normal setup path.
Resource choices are provider-identified selectable rows with a selection count
and accessible switch semantics. The shared switch also replaces native-looking
boolean controls in the Inspector and preserves focus, disabled, keyboard, and
reduced-motion behavior.
The chosen models transactionally become reusable profiles, which store only key,
display name, provider connection, model, and status. A model card stores
`model_profile`, while existing stored cards using `integration` remain supported.
The form accepts a one-time password-masked Token/API key, says that the browser
does not store it, and never displays it again.

The connection flow uses a modal wizard: choose the Gitea or LLM family,
configure and validate the URL and Token/API key, then select an organization and
scoped Gitea repositories or discovered LLM models. Resource management reloads
persisted selections into the same ModalShell flow. Details, edit, disable, and
delete are also focus-managed modal flows rather than inline list forms. The only
LLM choices exposed in the current UI are Ollama
local, Ollama Cloud and OpenRouter. OpenRouter uses the controlled
OpenAI-compatible adapter; both Ollama choices use the Ollama adapter. The
OpenAI-compatible URL normalizer accepts a versioned base such as
`https://openrouter.ai/api/v1` without appending a duplicate `/v1`.

The explicit card edit action opens its inspector; ordinary selection does not.
The inspector edits its display name and
the supported configuration fields: Gitea connection/PR coordinates for fetch
and publish, reusable model selection and output limit for model, templates,
file filters, group bounds, conditions and review validation/filter policies.
Publication event and autonomous-rejection controls appear on `publish`, not
`fetch`. Viewer fieldsets and resource controls remain disabled/read-only.
`Template review` loads the first official workflow graph into the canvas; it
connects `group -> loop -> template -> model -> validate -> response_filter`
per group. The inspector explains that `loop.item` stays in the group scope and
only `loop.results` crosses to root `consolidate -> format -> publish`, which
prevents one publication per group. Configured Gitea and model connections are
still required before it can run externally.

## POC boundary

`POC/` is read-only. It documents mature Gitea, provider, review and prompt
behavior but is not imported by the new backend or frontend.
