# Architecture

The new application separates `backend/` (Gin, SQLite and workflow domain) from
`frontend/` (Next.js Studio). The workflow catalog is backend-controlled and
definitions connect typed card ports. See `docs/architecture.md`.

SQLite also stores controlled integration records: key, name, provider type,
safe transport configuration, AES-256-GCM ciphertext, and status. The master
key is loaded only from `FORGEREVIEW_ENCRYPTION_KEY` and must be canonical
base64 for 32 bytes. The runner receives provider adapters explicitly and
decrypts an active integration's secret only at execution. `fetch` uses the
Gitea PR reader and `model` selects OpenAI-compatible or Ollama chat. API
summaries expose only `secret_configured`, never ciphertext or plaintext. See
[[Decision Log]] and [[Feature Map]].

Workflow definitions are append-only saved versions. SQLite groups their safe
metadata by workflow key for `GET /api/workflows`; each summary includes its
numeric version, creation time, and `draft`, `published`, or `archived` status.
Publishing validates the stored draft within a transaction, archives the prior
published version for that key, and then promotes the draft. Stored definitions
have no mutable HTTP endpoint. See [[Decision Log]] and [[Feature Map]].

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
to read SQLite. The in-process queue is an explicit local/test fallback only.

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
only execution status/times, workflow identity/version, and matching configured
fetch/publish PR coordinates. The `limit` is validated from 1 through 100. It excludes execution input, nodes, errors,
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
