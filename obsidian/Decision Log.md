# Decision Log

## 2026-07-21: Rebuild from POC

The current ForgeReview implementation moved to `POC/`. The new application
does not import POC code and begins with a generic card-based workflow domain.

## 2026-07-21: Controlled adapter integrations

Integration credentials are not stored in SQLite. An integration stores an
environment-variable reference, while its JSON config is restricted to provider
endpoint and model settings. The runner requires an active integration and an
explicit Gitea, OpenAI-compatible, or Ollama adapter before external cards can
run. This keeps external traffic bounded to registered provider contracts and
leaves publication and arbitrary HTTP execution out of scope.

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
