# ForgeReview Workflow Studio Architecture

## Foundation

`backend/` is a standalone Gin application. `frontend/` is a standalone Next.js
Studio. SQLite is the source of truth; the store boundary isolates SQL from
workflow domain and HTTP handlers. Redis carries only execution IDs;
the execution input remains in SQLite.

## Workflow contract

A workflow version stores a graph of nodes and typed edges. A card type is
registered by the backend and supplies its ports and capabilities; a saved
workflow only selects a registered type and its configuration. The initial
catalog includes the requested 20 cards grouped by input, data,
transformation, control, AI, validation, result, output and infrastructure.

The backend validates node keys, card types, declared ports and contract
compatibility before creating a draft version. Published version immutability,
workers and retry policies remain later increments.

## Current API

- `GET /health`: backend health.
- `GET /api/cards`: registered card catalog and typed ports.
- `POST /api/integrations`: create an active or disabled Gitea, OpenAI-compatible
  or Ollama integration. Config is limited to `base_url` and, for models,
  `model`; `secret_reference` must be an environment-variable name.
- `GET /api/integrations`: list safe integration summaries. The secret reference
  and any credential value are never returned.
- `POST /api/workflows`: validate and persist a new draft version.
- `GET /api/workflow-versions/:id`: load an immutable saved definition.
- `POST /api/workflow-versions/:id/executions`: create an execution for a
  stored definition. Without a dispatcher it runs synchronously and returns a
  completed report. With a dispatcher it enqueues the execution ID and returns
  `202 Accepted` with `{execution_id,status:"queued"}`.
- `GET /api/executions/:id`: load persisted node states, input references and
  output tokens.

## Execution Foundation

The workflow runner starts cards without required inputs, forwards typed tokens
through declared edges, and starts a downstream card only after all required
ports receive a token. Each completed or failed card is persisted in the
execution report. The safe local executors currently cover trigger, transform,
variable, log, cache, filter, group, template, condition, merge, validate,
response_filter, consolidate and format. A collecting input port waits for all
of its declared incoming edges; the `consolidate.comments` port uses this so
all available finding lists are merged before formatting. A `fetch`
card requires `config.integration`, `owner`, `repo` and `pull_request`; it uses
the injected Gitea adapter to read PR metadata, file changes and diff. A `model`
card requires `config.integration` and uses the injected OpenAI-compatible or
Ollama chat adapter. Both require an active stored integration and resolve the
credential only from the environment variable named by its stored
`secret_reference`.

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
  `consolidate` combines all incoming comment lists and applies the same
  deterministic deduplication. Finding order is path, line, descending severity,
  then comment.
- `format` emits `formatted_review`: a destination-neutral payload with the
  sorted findings and fixed summary counters (`total`, `low`, `medium`, `high`,
  `critical`).
- `publish` consumes `formatted_review` and requires `config.integration`,
  `owner`, `repo`, and positive `pull_request`. The integration must be active
  and Gitea. Its writer posts one controlled PR issue comment; arbitrary URLs,
  bodies, and credentials cannot be supplied by the workflow.

## Publication idempotency

Before the Gitea request, SQLite creates a unique publication attempt keyed by
the workflow execution ID, workflow version ID, and publish node key. Attempts
move from `pending` to `completed` with a safe provider receipt, or to
`retryable` after a known failure. A duplicate completed execution returns the
stored receipt instead of posting again; a duplicate pending attempt does not
post. The Gitea writer also sends the key in `X-ForgeReview-Idempotency-Key` and
as an HTML comment marker. Credentials and provider response bodies are never
stored in the attempt record or returned by APIs.

## Asynchronous execution dispatch

When `FORGEREVIEW_REDIS_URL` is set, startup verifies Redis and starts a worker
using its list-backed execution-ID queue. The worker atomically claims a queued
SQLite execution before running it, making duplicate queue deliveries harmless.
`GET /api/executions/:id` remains the status API and reports `queued`, `running`,
`completed`, or `failed` plus completed node reports. If Redis is unavailable,
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

## Studio validation flow

The Studio loads the card catalog from `GET /api/cards`, lets an administrator
add cards and draw port connections, and saves the current canvas as a new
workflow version. `Salvar e executar` runs a local verification graph and paints
each card as draft, running, completed or failed from the persisted execution
report. With Redis dispatch enabled, the Studio polls the execution endpoint
until the worker completes it.

## POC boundary

`POC/` is read-only. It documents mature Gitea, provider, review and prompt
behavior but is not imported by the new backend or frontend.
