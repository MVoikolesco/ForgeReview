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
compatibility before creating a draft version. Each save creates a new draft
version; no API mutates a saved definition. Publishing validates the stored
definition again and atomically promotes that draft while archiving the prior
published version for the same workflow key. Only one published version can
exist per workflow key.

## Current API

- `GET /health`: backend health.
- `GET /api/cards`: registered card catalog and typed ports.
- `POST /api/integrations`: create an active or disabled Gitea, OpenAI-compatible
  or Ollama integration. Config is limited to `base_url` and, for models,
  `model`; `secret_reference` must be an environment-variable name.
- `GET /api/integrations`: list safe integration summaries. The secret reference
  and any credential value are never returned.
- `POST /api/workflows`: validate and persist a new draft version.
- `GET /api/workflows`: list workflow keys with their latest name/description
  and all version summaries (`version_id`, `version`, `created_at`, and
  `status`). Status is one of `draft`, `published`, or `archived`.
- `GET /api/workflow-versions/:id`: load an immutable saved definition.
- `POST /api/workflow-versions/:id/publish`: atomically publish a draft and
  archive the previous published version for that key. It returns the published
  version summary. A missing version returns `404`, a non-draft returns `409`,
  and an invalid persisted definition returns `422` without changing statuses.
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

The production frontend image runs `next start` from the compiled `.next`
artifact. It intentionally does not run `next dev`, because the final image
does not contain source files and uses `NODE_ENV=production`.

## Studio lifecycle and validation flow

The Studio loads the card catalog from `GET /api/cards`, lets an administrator
add cards and draw port connections, and saves the current canvas as a new
workflow version. `Salvar rascunho` creates a draft. `Publicar` first creates a
new draft from the current canvas and only reports publication after
`POST /api/workflow-versions/:id/publish` succeeds. `Salvar e executar` retains
the local verification flow and paints each card as draft, running, completed or
failed from the persisted execution report. With Redis dispatch enabled, the
Studio polls the execution endpoint until the worker completes it.

`/pipelines` lists the grouped workflow summaries from `GET /api/workflows`.
It displays all draft, published, and archived versions and exposes publication
only for draft versions; after a successful response it reloads the lifecycle
list so the archived and published states are current. Header navigation links
connect Studio, Pipelines, and Integrations.

The `Integrações` control opens the Studio connection modal. It lists safe
integration summaries and creates Gitea, OpenAI-compatible or Ollama records
through the integration API. Model connections are visibly grouped as reusable
models. The form accepts only the name of the environment variable holding a
credential; it never requests, displays or persists its value.

The connection flow uses a three-step Studio wizard: choose the Gitea or LLM
family, configure the URL and secret reference, then select a model or review a
Gitea connection. The only LLM choices exposed in the current UI are Ollama
local, Ollama Cloud and OpenRouter. OpenRouter uses the controlled
OpenAI-compatible adapter; both Ollama choices use the Ollama adapter.

Selecting a card opens its inspector. The inspector edits its display name and
the supported configuration fields: Gitea connection/PR coordinates for fetch
and publish, reusable model selection and output limit for model, templates,
file filters, group bounds, conditions and review validation/filter policies.
`Template review` loads the first official workflow graph into the canvas; it
requires configured Gitea and model connections before it can run externally.

## POC boundary

`POC/` is read-only. It documents mature Gitea, provider, review and prompt
behavior but is not imported by the new backend or frontend.
