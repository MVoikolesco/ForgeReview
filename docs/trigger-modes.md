# Typed trigger modes

Trigger cards accept `config.mode: "manual" | "api" | "webhook"`. A missing
mode is the legacy-compatible `manual` mode. Each execution persists the selected
trigger key and runs only nodes reachable from that trigger, including when
multiple trigger branches converge.

## Authenticated API trigger

Publish the workflow, authenticate with the normal session cookie, then send one
JSON object to the selected API trigger:

```http
POST /api/workflows/{workflow_key}/triggers/{trigger_node_key}/executions
Content-Type: application/json
Cookie: forgreview_session=...

{"pull_request":{"owner":"acme","repo":"api","number":42}}
```

Editors and admins receive `202 Accepted`. The request is rejected unless the
workflow has a published version and the selected trigger is in `api` mode.

## Signed Gitea webhook

An admin registers a published `webhook` trigger through
`POST /api/webhook-registrations`. `secret` is a one-time input encrypted with
the ForgeReview encryption key; list and create responses expose only
`secret_configured`.

Configure Gitea to send pull-request events to:

```text
POST {base_url}/webhooks/gitea/{registration_key}
```

ForgeReview requires `Content-Type: application/json`, `X-Gitea-Event:
pull_request`, a nonempty `X-Gitea-Delivery`, and the hexadecimal HMAC-SHA-256
digest of the exact body in `X-Gitea-Signature`. Bodies are limited to 1 MiB. A
delivery retry with the same registration, delivery ID, and body returns the
original execution without enqueueing again; reusing that ID for another body
returns `409 Conflict`.

Example signature and request:

```sh
body='{"action":"synchronized","number":42,"repository":{"name":"api","owner":{"login":"acme"}},"pull_request":{"number":42}}'
signature="$(printf '%s' "$body" | openssl dgst -sha256 -hmac "$WEBHOOK_SECRET" -hex | cut -d' ' -f2)"
curl -X POST "$FORGEREVIEW_URL/webhooks/gitea/gitea-review" \
  -H 'Content-Type: application/json' \
  -H 'X-Gitea-Event: pull_request' \
  -H 'X-Gitea-Delivery: sample-delivery-1' \
  -H "X-Gitea-Signature: $signature" \
  --data-binary "$body"
```

The allowlisted execution context is canonicalized as
`pull_request.owner`, `pull_request.repo`, and `pull_request.number`. Fetch reads
that typed runtime target and emits the fetched pull request to publish. Neither
card accepts fixed `owner`/`repo`/`pull_request` configuration.

Import [`ForgeReview.postman_collection.json`](ForgeReview.postman_collection.json)
for login, API-trigger, registration, and signed-webhook samples. Fill collection
variables locally; the collection contains no credentials.
