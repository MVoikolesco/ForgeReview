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
