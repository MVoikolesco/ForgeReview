# Change Log

## 2026-07-21

- Moved the existing application to `POC/`.
- Created the new Go backend foundation, SQLite workflow-version store and
  registered card catalog.
- Created the initial Next.js React Flow Studio canvas.
- Added Docker Compose for frontend (3010), backend (8088), persistent SQLite
  data and Redis.
- Added version execution APIs and the token-based workflow runner. Safe local
  cards execute only when required typed inputs are available.
- Added SQLite-backed controlled integrations plus Gitea PR-read and
  OpenAI-compatible/Ollama chat adapters. Fetch and model cards require an
  active configured integration; integration APIs return safe summaries only.
- Added deterministic local PR-review cards for fetched-file filtering and
  grouping, model finding validation with an invalid branch, severity filtering,
  consolidation, and formatted-review payloads.
- Added controlled Gitea `publish` execution for `formatted_review`, with an
  execution/version/node idempotency ledger persisted before the external
  request and safe completed/retryable states; retry claims are atomically
  re-reserved before another provider call.
- Added Redis execution-ID dispatch, an atomic-claim worker lifecycle, queued
  execution status, and an explicit in-process queue fallback for tests/local
  development. Added publication duplicate/retry-state, adapter, and
  queue-dispatch tests.
- Connected the Studio to the card catalog and workflow APIs. It now saves a
  local verification graph, starts an execution, polls queued work when needed,
  and shows persisted node states on the canvas.
- Added the Studio integration modal for Gitea, OpenAI-compatible and Ollama
  connections, including a reusable-model list without exposing credentials.
- Added card-specific configuration in the Studio inspector, type-safe canvas
  connection blocking, and the first official review-pipeline canvas template.
- Replaced the single-form integrations modal with a compact Studio stepper for
  Gitea and the supported LLM providers: Ollama local/cloud and OpenRouter.
- Corrected the production frontend container to run `next start` rather than
  `next dev`; the final image contains the compiled artifact only.
- Consolidated current delivery, limitations and remaining work in
  `docs/roadmap.md`.
- Refactored the frontend Studio into routed, reusable components and typed
  helpers. Added `/studio`, `/integrations`, root redirection, SCSS modules and
  shared styles, plus typed-connection unit coverage.
- Added backend workflow version lifecycle APIs: list safe version summaries and
  atomically publish valid drafts while archiving the previous published version.
  Added store and HTTP coverage for lifecycle transitions, invalid drafts,
  missing drafts, and list metadata.
- Added frontend lifecycle controls: separate Studio draft-save and publish
  actions, a `/pipelines` grouped version list with lifecycle badges and draft
  publication, and navigation between Studio, Pipelines, and Integrations.
- Added backend scoped loop execution: lists and file groups create sequential
  deterministic child scopes through downstream cards, aggregate terminal
  results, expose/persist scope keys, and validate iteration/concurrency/error
  policy configuration. See [[Architecture]] and [[Decision Log]].
- Wired the official review template through `group -> loop -> template ->
  model -> validate -> response_filter` in each child scope. `loop.results`
  now aggregates finding lists into root consolidation, followed by one format
  and controlled publication run; local fake-adapter coverage verifies the
  multi-group path. See [[Architecture]] and [[Feature Map]].
- Replaced integration environment-secret references with AES-256-GCM encrypted
  Token/API key persistence. The backend now requires a strict
  `FORGEREVIEW_ENCRYPTION_KEY`, migrates legacy integration tables without
  retaining references, exposes only `secret_configured`, and decrypts only for
  controlled provider execution. The connection wizard accepts a one-time
  password-masked field and does not store it in the browser. See
  [[Architecture]] and [[Decision Log]].
