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

Docker Compose exposes the new frontend on port 3010 and backend on port 8088;
SQLite and Redis use named volumes.

The controlled PR-review path after `fetch`/`model` is local to the workflow
runner: `filter` → `group` → `validate` → `response_filter` → `consolidate` →
`format`. Findings have the stable `path`, positive `line`, `comment`, and
`severity` contract; validation routes failures through `validate.invalid`.
`consolidate.comments` is a collecting port, so it waits for every declared
incoming finding list. See [[Feature Map]] and `docs/architecture.md`.

`publish` now consumes `formatted_review` and is restricted to an active Gitea
integration and the Gitea writer adapter. SQLite records a publication attempt
before the external comment request, keyed from execution ID, version ID, node
key, and child scope when present; duplicate completed or pending attempts do
not post again. Attempts have `pending`, `completed`, and `retryable` states.
See [[Decision Log]].

The runner carries `root` or child scope keys on tokens and node reports. A
`loop` consumes a list or `group.groups`, creates deterministic sequential child
scopes (`<loop-key>:000001`), and keeps downstream `loop.item` tokens in that
scope without duplicating graph nodes. After child terminal cards finish,
`loop.results` emits their ordered aggregate in `root`. It is a scope boundary:
the official review flow ends each child at `response_filter`, flattens its
finding-list aggregate in root `consolidate`, then runs `format` and `publish`
once. SQLite persists each node-run scope key and loop execution metadata. See
[[Decision Log]] and [[Feature Map]].

Execution input remains in SQLite. When configured, Redis transports execution
IDs to a worker which atomically claims queued executions; status APIs continue
to read SQLite. The in-process queue is an explicit local/test fallback only.

The frontend routes are intentionally thin: `/` redirects to `/studio`, while
`/studio`, `/pipelines`, and `/integrations` compose dedicated client workspaces.
Studio state and API calls live in reusable frontend components and `src/lib/`;
the Pipelines workspace reads grouped safe workflow summaries and owns list
publication feedback without replacing the current lifecycle list. React Flow
cards use a shared card shell, and the
connection flow uses a shared modal shell. Styling is colocated SCSS modules
backed by `src/styles` tokens, themes, globals, and mixins. See [[Decision Log]]
and [[Feature Map]].
