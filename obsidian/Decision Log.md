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

## 2026-07-22: Open immutable versions as new Studio drafts

Pipeline versions link to Studio by version ID, where the frontend loads the
immutable definition and rebuilds its visual graph from the current card
catalog. The loaded definition's key, name, and description are retained for
all save paths, so editing an existing version always appends a draft to the
same workflow rather than silently creating or mutating another version.
Catalog gaps stop hydration instead of rendering a partial graph. The Studio
identifies the source version and marks unsaved changes, including a browser-exit
warning. See [[Architecture]] and [[Feature Map]].

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

## 2026-07-22: Idempotent official review-pipeline seed

The server ensures a published `official-gitea-pr-review` workflow at startup,
instead of requiring a manual first publication. The store checks for a
published version under that dedicated key and otherwise creates the full review
graph as the next version and publishes it in one transaction. This leaves user
workflow keys, official drafts, and an existing published official version
untouched across restarts. The seed follows current model-profile configuration
and explicitly defaults its native Gitea publication to `COMMENT` without
autonomous rejection. Discovery remains on the existing workflow-list and
version-load APIs rather than adding a Studio-only path. See [[Architecture]]
and [[Feature Map]].

## 2026-07-22: Dashboard-safe operational summaries

The root route is an operational dashboard rather than a redirect. Its execution
feed reads a bounded endpoint that joins execution lifecycle metadata to its
stored workflow version and derives PR coordinates only from consistent fetch and
publish configuration. The response deliberately excludes workflow definitions,
node runs, execution input, errors, integration settings, and credentials. The
dashboard derives official-pipeline readiness and conservative publication state
from the published definition plus safe connection/profile summaries. See
[[Architecture]] and [[Feature Map]].

## 2026-07-22: Local signed sessions with revocable SQLite state

ForgeReview uses bcrypt password hashes in SQLite and bootstraps one administrator
only while the users table is empty. A session cookie is HttpOnly and SameSite
Lax; its user/expiry/nonce claims are HMAC-SHA-256 signed, and the nonce is also
persisted so logout and expiry can be enforced server-side. This avoids browser
token persistence while remaining practical for a local self-hosted HTTP setup.
Viewer access is read-only, editor access is limited to workflow drafts and
execution, and integration/model/user management requires admin. See
[[Architecture]] and [[Feature Map]].

## 2026-07-22: Explicit first-admin bootstrap without overwrite

Compose requires `FORGEREVIEW_BOOTSTRAP_ADMIN_EMAIL` and
`FORGEREVIEW_BOOTSTRAP_ADMIN_PASSWORD`; when `users` is empty, ForgeReview
creates the first admin with exactly the configured trimmed, lowercase email and
a bcrypt password hash. It refuses an empty first-run configuration. Once any
user exists, bootstrap returns without changing users, so environment edits can
never silently replace an administrator. Lost-all-admin recovery is therefore a
deliberate, backed-up local identity/session reset followed by a new bootstrap;
there is no default credential or automatic reset. See [[Architecture]] and
[[Feature Map]].

## 2026-07-22: Local, safe workflow-definition transfer

Clone, import, and export remain Studio-local operations and reuse the existing
append-only draft-save endpoint. Export uses an explicit v1 envelope rather
than a raw database version; import validates its graph against the current card
catalog before changing the canvas. The backend rejects credential-like config
field names on every draft validation, so immutable workflow history cannot
become a secret or ciphertext transport. Clone always receives a distinct
workflow identity and only resolves duplicate node/edge keys where present.
Editors and admins own these actions; viewers remain read-only. See
[[Architecture]] and [[Feature Map]].

## 2026-07-22: Direct Studio graph removal with advisory local validation

The Studio intercepts React Flow's usual Delete/Backspace behavior and removes
the current selection directly. Card-header delete follows the same no-dialog
behavior and filters incident edges; viewers cannot invoke either path. A
catalog-aware client issue list blocks save, publish, and execution only for
failures the current graph can determine, and links an issue back to its card
where possible. The API remains the authority because it validates the persisted
definition against current server rules. See [[Architecture]] and [[Feature Map]].

## 2026-07-22: Stable controlled-canvas selection callbacks

React Flow invokes `onSelectionChange` from an effect whose dependencies include
the callback. The Studio therefore keeps its selection callback stable and
preserves prior selection-ID state for repeated reports; derived error-edge
props are memoized as well. This prevents a controlled hydration/update from
continually triggering React state updates while retaining card and edge
selection behavior. See [[Architecture]] and [[Feature Map]].

## 2026-07-22: Card actions stay outside persisted workflow data

React Flow card edit/delete behavior is provided by a memoized React context,
not callbacks placed in `CardData`. Ordinary selection updates only selection
state; edit explicitly opens the Inspector and pane clicks close it. This keeps
serialized workflow definitions deterministic and avoids callback identity
changes feeding the controlled canvas. The hidden Inspector also releases its
desktop grid column, while responsive widths use an overlay. Manual execution
payload belongs to the run modal and is sent only with that execution request.
See [[Architecture]] and [[Feature Map]].

## 2026-07-22: Scoped integration resource discovery

Gitea resource discovery separates organization listing from repository listing,
and the repository adapter accepts exactly one selected organization rather than
enumerating every organization. Unsaved candidate setup follows the same sequence
after server-side validation; active resource management reloads persisted
selection. LLM validation returns provider-discovered models and the normal UI
only persists selected models as profiles, eliminating free-text model entry.
This keeps credentials one-time and server-side while making the selection path
explicit. See [[Architecture]] and [[Feature Map]].

## 2026-07-22: Typed trigger modes and canonical event input

A trigger declares `manual`, authenticated `api`, or signed `webhook` mode; a
missing mode remains compatible with historical manual workflows. Every
execution persists the selected trigger node and the runner schedules only its
reachable branch. API triggers target a published workflow and node under RBAC.
Gitea webhook registrations use a distinct encrypted signing secret, exact-body
HMAC-SHA-256 verification, and a delivery ledger that deduplicates identical
deliveries and rejects ID collisions. Provider payloads are reduced to canonical
owner/repository/PR coordinates before persistence. New review graphs pass that
typed target from trigger to fetch and from fetch to publish, while old immutable
versions retain fixed-coordinate fallback. See [[Architecture]] and
[[Feature Map]].

## 2026-07-23: Truthful catalog and provider request contracts

Catalog availability is explicit: the unsupported `workflow` card remains
discoverable but disabled and backend validation refuses definitions containing
it. `merge.inputs` uses `collect_all` and emits all branch values only after all
active incoming edges deliver. Model `max_tokens` is bounded to 1–128,000 with a
2,000 default and translated to each provider's request shape. Review event and
autonomous-rejection policy is valid only on `publish`, preventing `fetch` from
advertising settings it cannot execute. See [[Architecture]] and [[Feature Map]].

## 2026-07-23: One accessible switch language

Boolean settings and resource multi-selection share one native-input-backed
`ToggleSwitch` rather than exposing browser checkbox styling. Provider-labelled
resource cards add selection counts and selected/hover feedback while preserving
keyboard, focus, disabled, screen-reader, and reduced-motion behavior. Viewer
authorization remains read-only independently of visual presentation. See
[[Architecture]] and [[Feature Map]].
