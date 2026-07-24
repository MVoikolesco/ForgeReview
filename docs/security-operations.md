# Security operations

Set `FORGEREVIEW_CORS_ALLOWED_ORIGINS` to a comma-separated list of complete
browser origins. It defaults to `http://localhost:3010`; wildcards and malformed
origins stop startup. `FORGEREVIEW_TRUSTED_PROXIES` is empty by default. Supply
only the IPs/CIDRs of reverse proxies that terminate requests.

Sessions default to an `HttpOnly`, API-host-scoped `SameSite=Lax` cookie with
explicit expiry and `Max-Age`, which supports local HTTP on separate ports. A
Studio and API on different sites require the explicit credentialed CORS origin,
`FORGEREVIEW_SESSION_COOKIE_SAME_SITE=none`, and
`FORGEREVIEW_SESSION_COOKIE_SECURE=true`. `None` without `Secure` fails startup;
do not rely on forwarded proxy headers to choose this setting.

Administrators can manage users at `/admin`. Disabling a user or changing a role
revokes every existing session. The API refuses self-demotion/self-disable and
any change that would remove the final active administrator. `GET /api/audit-log`
is admin-only and returns bounded safe metadata; secrets, passwords, tokens, and
ciphertext are excluded.

Publication transport errors, 5xx responses, and unreadable successful responses
are stored as `uncertain`; they are not automatically retried. Validation,
provider 4xx, and uncertain-publication outcomes are never automatically retried.
Admins reconcile an uncertain attempt with `POST /api/publications/:key/reconcile`
and the active Gitea integration plus PR coordinates. ForgeReview searches reviews
 for its idempotency marker: a match completes the attempt; only a confirmed absent
 marker makes it retryable.

## Execution status and sensitive retention

`GET /api/executions/:id` and both SSE feeds (`/api/execution-events` and
`/api/executions/:id/events`) expose only execution lifecycle and per-card
identity/status/scope. Events are stored before delivery and replay from
`Last-Event-ID`; raw input, node values, provider data, errors, and secrets are
excluded. Sensitive execution input/node details are retained separately for
seven days. Reprocess creates a new execution; it is not a resume. Cancellation
is available only before external publication starts.

`GET /api/executions/:id/cards/:node/logs` backs Studio's per-card execution
modal. It returns only lifecycle events. Independently, the worker appends one
safe JSONL record per card transition to
`FORGEREVIEW_LOG_DIR/execution-<id>.log` (default `./logs` in the project;
mounted at `/app/logs` in Compose).
The `log` card receives a `log_card` record. These files intentionally omit
inputs, prompts, model responses, provider bodies, and credentials.

The status endpoint always emits `runs` as an array, including queued reports
with no node progress. Studio normalizes incomplete reports without exposing
unsafe fields, retains the visible lifecycle status, and displays a contract
warning rather than failing during polling.

## Retry and dead-letter handling

Worker failures classified as transient retry no more than three times with
deterministic exponential backoff and bounded jitter. Permanent and uncertain
failures become `dead_letter`; an administrator may create a new execution from
retained input with `POST /api/executions/:id/replay`. Replay is audited. Recovery
also returns running executions stale for 15 minutes to the queue; it never
claims to resume in-memory provider work.
