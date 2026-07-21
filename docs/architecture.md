# ForgeReview Workflow Studio Architecture

## Foundation

`backend/` is a standalone Gin application. `frontend/` is a standalone Next.js
Studio. SQLite is the initial source of truth; the store boundary isolates SQL
from workflow domain and HTTP handlers. Redis workers and durable scheduling
remain later phases.

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
- `POST /api/workflow-versions/:id/executions`: run a stored definition with
  supplied event input and persist its report.
- `GET /api/executions/:id`: load persisted node states, input references and
  output tokens.

## Execution Foundation

The workflow runner starts cards without required inputs, forwards typed tokens
through declared edges, and starts a downstream card only after all required
ports receive a token. Each completed or failed card is persisted in the
execution report. The safe local executors currently cover trigger, transform,
variable, log, cache, filter, group, template, condition and merge. A `fetch`
card requires `config.integration`, `owner`, `repo` and `pull_request`; it uses
the injected Gitea adapter to read PR metadata, file changes and diff. A `model`
card requires `config.integration` and uses the injected OpenAI-compatible or
Ollama chat adapter. Both require an active stored integration and resolve the
credential only from the environment variable named by its stored
`secret_reference`. There is no arbitrary HTTP card or publication behavior.

## Containers

`docker compose up --build` runs the new Studio at `http://localhost:3010` and
the backend health endpoint at `http://localhost:8088/health`. SQLite state is
stored in the `workflow-data` volume. Redis is available only to the new
application on the internal Compose network and is reserved for workers,
events and coordination in later phases.

## POC boundary

`POC/` is read-only. It documents mature Gitea, provider, review and prompt
behavior but is not imported by the new backend or frontend.
