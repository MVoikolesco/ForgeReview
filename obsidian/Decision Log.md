# Decision Log

## 2026-07-21: Rebuild from POC

The current ForgeReview implementation moved to `POC/`. The new application
does not import POC code and begins with a generic card-based workflow domain.

## 2026-07-21: Controlled adapter integrations

Integration credentials are encrypted in SQLite, while JSON config remains
restricted to provider endpoint and model settings. The runner requires an
active integration and an explicit Gitea, OpenAI-compatible, or Ollama adapter
before external cards can run. This keeps external traffic bounded to registered
provider contracts and leaves publication and arbitrary HTTP execution out of
scope.

## 2026-07-21: AES-GCM integration-secret persistence

`FORGEREVIEW_ENCRYPTION_KEY` is the sole runtime master-key source and must be
canonical base64 for exactly 32 bytes. AES-256-GCM encrypts each one-time
Token/API key with random nonces and authenticates the integration key as
additional data. SQLite stores ciphertext only; APIs expose only configured
state, and execution resolves plaintext immediately before a provider call.
The legacy environment-reference column is removed by migration, leaving prior
records unconfigured because no credential value exists to convert. See
[[Architecture]] and [[Feature Map]].

## 2026-07-21: Deterministic local review findings

The post-fetch/model cards operate only on values already produced inside the
workflow. Model output is accepted only as a JSON finding list with `low`,
`medium`, `high`, or `critical` severity, and invalid output follows the
declared `validate.invalid` branch. File grouping uses available per-file change
text without inventing missing content; `consolidate` waits for all declared
finding-list inputs. This makes the review payload reproducible while leaving
external PR publication out of scope. See [[Architecture]] and [[Feature Map]].

## 2026-07-21: Controlled Gitea publication and execution dispatch

Publication is limited to a `formatted_review`, an active persisted Gitea
integration, and a writer adapter. SQLite records an execution/version/node
derived idempotency key before the provider call, returning completed or pending
attempts without a duplicate post and retaining only safe receipts. Redis queues
execution IDs while SQLite retains inputs and status; workers atomically claim
queued executions. The non-durable in-process queue requires explicit local/test
opt-in, so production does not silently lose asynchronous work. See
[[Architecture]] and [[Feature Map]].

## 2026-07-21: Frontend workspace boundaries and SCSS modules

The Next.js routes only compose their workspace components. Workflow behavior is
split into typed API/workflow helpers and reusable library, canvas, card shell,
inspector, modal, and connection components. SCSS is colocated with components
and consumes shared theme/token/mixin files rather than a single Studio
stylesheet. This preserves the existing dark workflow-canvas visual language
while making routes and components independently maintainable. See
[[Architecture]] and [[Feature Map]].

## 2026-07-21: Append-only workflow version publication

Workflow saves always create a new draft rather than updating a version. The
publish operation revalidates the persisted graph inside its SQLite transaction,
archives any prior published version for the workflow key, then promotes the
target draft. This preserves a single published version while retaining prior
definitions for execution and inspection. Safe list summaries expose lifecycle
metadata from `GET /api/workflows` without embedding definitions. See
[[Architecture]] and [[Feature Map]].

## 2026-07-21: Frontend workflow lifecycle feedback

The Studio always saves the current canvas before a user-requested publication,
then reports a published version only after the publish API responds
successfully. The separate Pipelines workspace reads grouped summaries rather
than definitions and offers publish controls only on drafts. This keeps lifecycle
state visible while feedback is shown, without implying a failed or unconfirmed
publication succeeded.
See [[Architecture]] and [[Feature Map]].

## 2026-07-21: Sequential scoped loop execution

Loops use deterministic child scopes based on the ordered input index rather
than duplicating workflow nodes. `max_iterations` is mandatory, `concurrency`
is validated but currently limited to `1`, and loop metadata records the active
settings and iteration outcome. A default child failure fails the execution;
`on_error: "partial"` preserves the failed child report, continues later items,
and aggregates successful terminal outputs. This provides reproducible group
processing while reserving nested and parallel loops for a later increment. See
[[Architecture]] and [[Feature Map]].

## 2026-07-21: Loop-results root aggregation boundary

The official review flow treats `response_filter` as the terminal per-group
card. `loop.results` carries the ordered list(s) of validated finding lists to
root `consolidate`; nested lists are flattened there before deterministic
deduplication. The scoped-path calculation stops at a `loop.results` target, so
`consolidate`, `format`, and `publish` are root-only and publication cannot be
repeated for each group. See [[Architecture]] and [[Feature Map]].

## 2026-07-22: Direct model-validation corrective retry

Corrective retry is runner behavior rather than a graph cycle. Only an invalid
`validate.response` token directly produced by a model is eligible, and the
runner uses that model's original prompt in the same scope with a fixed repair
instruction. `retry_limit` defaults to zero and is capped at three; delay is
capped at 60 seconds. Each retry is revalidated and cannot schedule another
graph traversal, so an exhausted limit preserves `validate.invalid` routing.
Attempt metadata contains counts and statuses only, avoiding new persistence of
prompt, response, or secret plaintext. See [[Architecture]] and [[Feature Map]].

## 2026-07-22: Explicit, sanitized graph error routes

All executable cards use `on_error` with `fail`, `continue`, `partial`, or
`route`; a route is valid only through its declared `error` output edge, never
by a destination node key in configuration. The runner records a stable
`execution_failed` code, selected action, and scope in node-run metadata while
omitting the underlying provider error. The routed `ErrorToken` preserves the
current root or loop-child scope. `error_control` is intentionally separate: its
`on_error` chooses terminal fail, continue, or a required fallback result.
This keeps recovery visible and type-checked without exposing provider bodies
or credentials. See [[Architecture]] and [[Feature Map]].

## 2026-07-22: Reusable model profiles separate from connections

An LLM integration now represents a credential-bearing provider connection, not
a single model choice. SQLite `model_profiles` records a safe reusable model
selection tied to an OpenAI-compatible or Ollama connection. The runner resolves
an active profile and active integration, then supplies the profile model only in
the in-memory provider configuration. This avoids duplicating credentials for
each model while preserving execution of prior workflow versions that reference
an integration directly. See [[Architecture]] and [[Feature Map]].

## 2026-07-22: Explicit Redis cache-card adapter

The cache card uses a workflow-level cache interface and a Redis implementation
separate from dispatch's execution-ID queue. Its configuration requires a key and
an explicit `read`, `write`, or `delete` mode; writes additionally require a TTL
of 1–86,400 seconds. The runner serializes values as JSON, treats a miss as a
nil value, and emits no token after deletion. Redis cache wiring is conditional
on a reachable Redis service, so no hidden in-memory cache changes workflow
behavior. See [[Architecture]] and [[Feature Map]].

## 2026-07-22: Safe native Gitea review events

The publish adapter creates one native Gitea pull-request review with inline
comments derived from validated findings, instead of posting an issue comment.
Formatting proposes `REQUEST_CHANGES` for high or critical findings, but the
publisher defaults to `COMMENT` unless an administrator explicitly enables
`allow_autonomous_rejection`; medium findings default to `COMMENT` and can only
escalate through `medium_severity_event`. The system never automates approval.
Existing execution-bound idempotency remains the sole duplicate-post control;
manual approval and reconciliation after uncertain provider outcomes are pending.
See [[Architecture]] and [[Feature Map]].
