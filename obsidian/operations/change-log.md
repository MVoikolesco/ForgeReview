# Change Log

Record meaningful changes using this structure:

## YYYY-MM-DD — Change title

- **Outcome:** What changed for users or operators.
- **Scope:** Components and paths affected.
- **Validation:** Commands and observed results.
- **Notes updated:** Related vault notes.
- **Limitations:** Remaining constraints or risks.

## 2026-07-16 — AI connection model lifecycle

- **Outcome:** Added connection-scoped model catalog additions, explicit default actions, destructive confirmations, transactional model/connection deletion, and active replacement promotion.
- **Scope:** `internal/admin/handler.go`, `internal/admin/setup.go`, admin tests, and connection dashboard/detail/wizard components and styles.
- **Validation:** `gofmt`; `go test ./...`; `web-admin npm run lint`; `web-admin npm run build` passed. Build reports the existing multiple-lockfile workspace-root warning.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../architecture/feature-map|Feature Map]], [[../decisions/log|Decision Log]].
- **Limitations:** Catalog availability still depends on the provider endpoint and configured environment-backed credentials; no browser test suite exists in this repository.

## 2026-07-16 — Gitea administration flow

- **Outcome:** Added authenticated Gitea catalog calls, admin registration/testing/listing/selection endpoints, encrypted token persistence, and a console area for the workflow.
- **Scope:** `internal/gitea`, `internal/admin`, migration 008, and `web-admin/src/components/gitea` plus console contracts/navigation.
- **Validation:** `gofmt`; `go test ./...`; `web-admin npm run lint`; `web-admin npm run build` compiled successfully, with the build command exceeding the 120-second tool timeout during finalization.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../architecture/feature-map|Feature Map]], [[../decisions/log|Decision Log]].
- **Limitations:** No worker contract change; only the existing environment fallback is retained.

## 2026-07-16 — Worker Gitea instance routing

- **Outcome:** Worker reviews now resolve the configured Gitea client per job, repository selection, or default instance while retaining the environment fallback for old jobs.
- **Scope:** `internal/queue`, Redis serialization, `internal/gitea/resolver.go`, reviewer agent, worker startup, and worker validation.
- **Validation:** `gofmt`; `go test ./...` passed.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../architecture/feature-map|Feature Map]], [[../decisions/log|Decision Log]].
- **Limitations:** Webhooks continue to enqueue a zero instance ID; resolution uses the saved repository mapping/default instance at worker execution time.

## 2026-07-16 — Enabled Gitea default resolution

- **Outcome:** Unmapped repositories now select only enabled Gitea instances, preferring the default before the lowest id.
- **Scope:** `internal/gitea/resolver.go` and its regression test.
- **Validation:** `gofmt`; `go test ./...`.
- **Notes updated:** [[../architecture/overview|Architecture]].

