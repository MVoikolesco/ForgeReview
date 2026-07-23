# Architecture

The new application separates `backend/` (Gin, SQLite and workflow domain) from
`frontend/` (Next.js Studio). The workflow catalog is backend-controlled and
definitions connect typed card ports. See `docs/architecture.md`.

SQLite also owns local identity: users have bcrypt password hashes and a
`viewer`, `editor`, or `admin` role. An empty users table can bootstrap exactly
the trimmed, lowercase email in `FORGEREVIEW_BOOTSTRAP_ADMIN_EMAIL` using
`FORGEREVIEW_BOOTSTRAP_ADMIN_PASSWORD`; Compose requires both values. Startup
fails when either is absent on first run, while later environment changes are
ignored once any user exists and cannot overwrite an account. Sessions use a
short-lived HMAC-signed, HttpOnly/SameSite-Lax cookie plus a revocable SQLite
nonce; no browser token persistence is used. See [[Decision Log]] and
[[Feature Map]].

SQLite also stores controlled integration records: key, name, provider type,
safe transport configuration, AES-256-GCM ciphertext, and status. The master
key is loaded only from `FORGEREVIEW_ENCRYPTION_KEY` and must be canonical
base64 for 32 bytes. The runner receives provider adapters explicitly and
decrypts an active integration's secret only at execution. `fetch` uses the
Gitea PR reader and `model` selects OpenAI-compatible or Ollama chat. API
summaries expose only `secret_configured`, never ciphertext or plaintext. See
[[Decision Log]] and [[Feature Map]].

Connection setup validates unsaved credentials only through a server-side,
ten-second-bounded provider adapter and returns sanitized outcomes. Active
connections can discover safe Gitea organization repositories or provider model
names; SQLite transactionally replaces repository selections or LLM model
profiles. Admins own connection lifecycle; editors may manage selections and
viewers are read-only. OpenAI-compatible URL construction preserves an existing
`/v1` suffix for OpenRouter. See [[Decision Log]] and [[Feature Map]].

Gitea discovery is organization-scoped: validation returns safe organization
names, and a separate candidate or active-connection request returns repositories
only after one organization is selected. Validation returns discovered
Ollama/OpenRouter models, which are selected rather than entered as free text.
All candidate calls remain server-side and one-time secrets never enter API
responses or browser persistence. See [[Integrations Upgrade]].

Workflow definitions are append-only saved versions. SQLite groups their safe
metadata by workflow key for `GET /api/workflows`; each summary includes its
numeric version, creation time, and `draft`, `published`, or `archived` status.
Publishing validates the stored draft within a transaction, archives the prior
published version for that key, and then promotes the draft. Stored definitions
have no mutable HTTP endpoint. See [[Decision Log]] and [[Feature Map]].

Studio-only clone/import/export uses the existing draft-save API. The v1
`forgereview.workflow` envelope contains a definition only; the client validates
its schema, catalog graph, typed ports, duplicate keys, and unsafe credential
field names before applying it. `workflow.Validate` repeats graph validation on
every save and rejects secret/ciphertext/password/token/API-key configuration
fields, preventing them from entering immutable version history. Clone creates
a distinct workflow key/name and only disambiguates duplicate graph keys. See
[[Decision Log]] and [[Feature Map]].

Server startup idempotently seeds `official-gitea-pr-review` as published
version 1 when that key has no published version. Its immutable definition is
the full scoped review graph and uses `model_profile`; its Gitea publish-card
defaults retain `allow_autonomous_rejection: false` and `COMMENT` for medium
severity. Existing official versions and all user keys remain unchanged. The
published version is discoverable through `GET /api/workflows` and loadable at
`GET /api/workflow-versions/:id`. See [[Decision Log]] and [[Feature Map]].

Docker Compose exposes the new frontend on port 3010 and backend on port 8088;
SQLite and Redis use named volumes.

The controlled PR-review path after `fetch`/`model` is local to the workflow
runner: `filter` → `group` → `validate` → `response_filter` → `consolidate` →
`format`. Findings have the stable `path`, positive `line`, `comment`, and
`severity` contract; validation routes failures through `validate.invalid`.
`consolidate.comments` is a collecting port, so it waits for every declared
incoming finding list. See [[Feature Map]] and `docs/architecture.md`.

`publish` now consumes `formatted_review` and is restricted to an active Gitea
integration and the Gitea writer adapter. It creates a native Gitea PR review
with a final event and inline comments, rather than an issue comment. SQLite records a publication attempt
before the external review request, keyed from execution ID, version ID, node
key, and child scope when present; duplicate completed or pending attempts do
not post again. Attempts have `pending`, `completed`, and `retryable` states.
See [[Decision Log]].

Formatting deterministically adds a proposed event/status and inline
observations (`path`, `body`, `new_position`) from the sorted findings. High or
critical findings propose `REQUEST_CHANGES`, but publication defaults to
`COMMENT` unless its `allow_autonomous_rejection` setting is true. Medium
findings remain `COMMENT` unless `medium_severity_event` is `REQUEST_CHANGES`.
Automated approval and uncertain-result reconciliation are intentionally pending.

LLM provider connections store only transport configuration and encrypted
credentials. `model_profiles` stores a reusable model name plus the key of its
OpenAI-compatible or Ollama connection. At execution, the runner resolves both
active records and derives the provider request configuration in memory; neither
profiles nor workflow definitions contain credentials. Legacy workflow versions
using `config.integration` on a model card remain executable. See [[Decision Log]]
and [[Feature Map]].

The runner carries `root` or child scope keys on tokens and node reports. A
`loop` consumes a list or `group.groups`, creates deterministic sequential child
scopes (`<loop-key>:000001`), and keeps downstream `loop.item` tokens in that
scope without duplicating graph nodes. After child terminal cards finish,
`loop.results` emits their ordered aggregate in `root`. It is a scope boundary:
the official review flow ends each child at `response_filter`, flattens its
finding-list aggregate in root `consolidate`, then runs `format` and `publish`
once. SQLite persists each node-run scope key and loop execution metadata. See
[[Decision Log]] and [[Feature Map]].

When a direct `model.response -> validate.response` link produces an invalid
finding list, validation may synchronously re-invoke that source model in the
same scope. The bounded configuration belongs to the model (`retry_limit` 0–3,
`retry_delay_ms` 0–60000); it uses the original prompt plus a static repair
instruction and does not create a graph back edge. Model and validation node
metadata records only attempt numbers and statuses, never repair prompts,
responses, or credentials. Exhaustion emits the normal `validate.invalid`
token. See [[Decision Log]] and [[Feature Map]].

Execution input remains in SQLite. When configured, Redis transports execution
IDs to a worker which atomically claims queued executions; status APIs continue
to read SQLite. Workers also recover persisted queued executions that missed a
Redis wake-up. Each execution stores its selected trigger key, so only that
trigger's reachable branch runs even when multiple branches later converge.
Trigger modes are `manual`, authenticated `api`, or signed `webhook`; an absent
mode remains legacy-compatible `manual`.

The runner reports node start and terminal transitions through a progress
boundary backed by SQLite. In-progress status reads expose only safe node
identity, status, and scope while execution is active; final persistence replaces
progress rows without duplication. Studio applies every polled report, animates
only edges entering a running card, and uses sober status badges and borders.
Provider adapters return safe model/token usage metadata, which publication uses
with elapsed time; prompts, responses, and credentials are not telemetry.

Gitea webhook registrations bind an encrypted one-time signing secret to a
published workflow and one webhook trigger. The public endpoint validates the
exact raw body with HMAC-SHA-256, requires Gitea event/delivery headers, and uses
a durable delivery ledger to return the original execution for identical
retries while rejecting delivery-ID collisions. It persists only canonical PR
coordinates as workflow input. `fetch` consumes that runtime target and passes
the typed pull request to `publish`. Fixed PR coordinates are rejected in both
cards. Startup migrates stored definitions by removing those fields and adding a
deterministic typed fetch-to-publish edge; ambiguous multi-fetch graphs stop with
a migration error rather than selecting an unsafe target. See [[Decision Log]]
and [[Feature Map]].

Redis also backs the workflow `cache` card through a separate explicit adapter;
it is not coupled to the execution-ID queue. Cache values cross the adapter as
JSON and can be read, written with a bounded TTL, or deleted. The adapter is
wired only after Redis is reachable; absent Redis leaves cache cards unavailable
rather than falling back to process memory. A write waits for its `value` input;
read and delete modes start without one. See [[Decision Log]].

The frontend routes are intentionally thin: `/` composes the operational
dashboard, while `/studio`, `/pipelines`, and `/integrations` compose dedicated
client workspaces. The dashboard requests health, safe connection summaries,
workflow lifecycle data, model profiles, and bounded execution summaries in
parallel. It loads the official published definition only to derive displayed
node labels, readiness, and conservative publish-policy state; it does not show
definition configuration or credentials. `GET /api/executions?limit=10` returns
only execution status/times, workflow identity/version, and canonical runtime PR
context. The `limit` is validated from 1 through 100. It excludes execution input, nodes, errors,
integration configuration, and secrets.
Studio state and API calls live in reusable frontend components and `src/lib/`;
the Pipelines workspace reads grouped safe workflow summaries and owns list
publication feedback without replacing the current lifecycle list. React Flow
cards use a shared card shell, and the
connection flow uses a shared modal shell. Styling is colocated SCSS modules
backed by `src/styles` tokens, themes, globals, and mixins. See [[Decision Log]]
and [[Feature Map]].

Every card except `error_control` has a conditional `error` output contract in
the catalog. It is enabled only by `config.on_error: "route"`, which must have
an explicit typed graph edge; it is therefore not rendered on ordinary cards.
The runner emits a scope-preserving `ErrorToken` with only a stable code, node,
and scope—not provider messages, bodies, prompts, or secrets. `error_control`
accepts that token and can fail terminally, continue, or emit a configured
 fallback result. See [[Decision Log]] and [[Feature Map]].

Studio uses controlled React Flow state for selection and removal: native
delete keys are disabled, while Studio handles Delete/Backspace directly for
the selected cards/edges. Card-header deletion also removes incident edges in
the same local update. Edit/delete callbacks are supplied through a memoized
React context rather than persisted `CardData`. The frontend also derives a
catalog-aware validation issue list before save/publish/run; it is advisory and
the backend's `workflow.Validate` remains the final persistence authority. See
[[Feature Map]] and [[Decision Log]].

React Flow selection handlers and derived error-edge props are memoized. The
selection handler ignores identical ID reports before setting Studio state, so a
controlled canvas hydration or graph update cannot re-register a listener and
create a render/update loop. See [[Decision Log]].

The Studio Inspector is conditional and absent from the default grid. Card
selection remains independent from editing; only the card edit action or a
linked validation issue opens it, while a pane click closes it. The canvas grid
claims the released width and the Inspector becomes a responsive overlay below
the desktop breakpoint. Manual trigger JSON is entered only in the focused run
modal, not stored in card data or displayed in the global header. See
[[Feature Map]] and [[Decision Log]].

Catalog cards now declare availability. `workflow`/subpipeline remains visible
but unavailable in Studio and backend validation rejects it until a runtime
contract exists. `merge.inputs` is a `collect_all` port and emits the ordered
list only after every active incoming edge has delivered. Frontend `Port` typing
exposes `collect_all`. See [[Feature Map]] and [[Decision Log]].

Model `max_tokens` defaults to 2,000 and is bounded from 1 through 128,000. The
runner adds it only to the in-memory provider configuration; OpenAI-compatible
requests receive `max_tokens` and Ollama receives `options.num_predict`.
Publication policy fields are validated only on `publish` and are rejected on
`fetch`. See [[Decision Log]].

All boolean controls and repository/model multi-selection use the shared
accessible `ToggleSwitch`. Resource choices are provider-identified selectable
cards with selection counts; switches retain native keyboard/input semantics,
visible focus and disabled states, and reduced-motion behavior. Viewer resource
management is never exposed while role state is unknown. See [[Feature Map]].
