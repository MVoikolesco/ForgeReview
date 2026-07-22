# Feature Map

- POC: preserved under `POC/`.
- Workflow catalog: initial backend foundation implemented.
- Studio canvas: initial visual prototype implemented.
- Studio validation: saves and executes a local card graph, then reflects node
  states from the execution report on the React Flow canvas.
- Integration modal: submits a one-time password-masked Token/API key for Gitea
  and reusable model connections, with no browser storage or later display.
- Connection wizard: guides Gitea, Ollama local/cloud and OpenRouter setup in
  discrete steps consistent with the Studio visual language.
- Card inspector: edits supported node configuration and selects active Gitea
  or reusable model connections. The review template projects the full initial
  review graph onto the canvas.
- Integrations: create and list Gitea, OpenAI-compatible and Ollama connection
  records using AES-256-GCM ciphertext in SQLite; APIs expose only safe
  `secret_configured` state.
- Frontend routes: `/studio` contains the workflow editor; `/integrations`
  provides the reusable connection wizard and connection/model list; `/`
  redirects to the Studio.
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
  posts one Gitea PR comment only through an active configured integration,
  writer adapter, and durable execution-bound idempotency record.
- Async execution: configured Redis dispatches persisted execution IDs to a
  worker; execution status remains queryable while queued, running, completed,
  or failed. Synchronous starts remain available without a dispatcher.
- Workflow version lifecycle: saves remain new drafts; workflow lists expose
  version number, creation time, and draft/published/archived state. Publishing
  a valid draft atomically archives its workflow's previous published version.
- Pipeline lifecycle UI: `/pipelines` shows grouped versions with distinct
  rascunho/publicada/arquivada states and permits publication only from a draft.
  Studio has separate draft-save and publish controls; publishing saves the
  visible canvas before calling the version publish endpoint.
