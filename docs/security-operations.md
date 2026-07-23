# Security operations

Set `FORGEREVIEW_CORS_ALLOWED_ORIGINS` to a comma-separated list of complete
browser origins. It defaults to `http://localhost:3010`; wildcards and malformed
origins stop startup. `FORGEREVIEW_TRUSTED_PROXIES` is empty by default. Supply
only the IPs/CIDRs of reverse proxies that terminate requests.

Administrators can manage users at `/admin`. Disabling a user or changing a role
revokes every existing session. The API refuses self-demotion/self-disable and
any change that would remove the final active administrator. `GET /api/audit-log`
is admin-only and returns bounded safe metadata; secrets, passwords, tokens, and
ciphertext are excluded.

Publication transport errors, 5xx responses, and unreadable successful responses
are stored as `uncertain`; they are not automatically retried. Definitive 4xx
provider rejections are retryable only through the controlled publication flow.
Admins reconcile an uncertain attempt with `POST /api/publications/:key/reconcile`
and the active Gitea integration plus PR coordinates. ForgeReview searches reviews
for its idempotency marker: a match completes the attempt; only a confirmed absent
marker makes it retryable.
