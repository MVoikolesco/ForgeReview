# Architecture

The new application separates `backend/` (Gin, SQLite and workflow domain) from
`frontend/` (Next.js Studio). The workflow catalog is backend-controlled and
definitions connect typed card ports. See `docs/architecture.md`.

SQLite also stores controlled integration records: key, name, provider type,
safe transport configuration, environment-variable secret reference, and
status. The runner receives provider adapters explicitly. `fetch` uses the
Gitea PR reader and `model` selects OpenAI-compatible or Ollama chat; both only
run with an active integration and resolve credentials from its named
environment variable. See [[Decision Log]] and [[Feature Map]].

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
before the external comment request, keyed from execution ID, version ID and
node key; duplicate completed or pending attempts do not post again. Attempts
have `pending`, `completed`, and `retryable` states. See [[Decision Log]].

Execution input remains in SQLite. When configured, Redis transports execution
IDs to a worker which atomically claims queued executions; status APIs continue
to read SQLite. The in-process queue is an explicit local/test fallback only.
