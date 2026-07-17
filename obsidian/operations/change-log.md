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

## 2026-07-16 — Gitea connection lifecycle UI

- **Outcome:** Reworked Gitea registration into the product stepper pattern and added a configured-connection summary with organization/repository visibility, edit, and remove actions.
- **Scope:** `web-admin/src/components/gitea`, `internal/admin/gitea.go`, `internal/admin/handler_test.go`.
- **Validation:** `go test ./internal/admin ./internal/store`; `web-admin npm run lint`; `web-admin npm run build` passed. Build retains the existing multiple-lockfile workspace warning.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../architecture/feature-map|Feature Map]], [[../decisions/log|Decision Log]].
- **Limitations:** The organization shown after reload is derived from the owner of the saved repository mapping; an empty selection has no persisted organization label.

## 2026-07-16 — Shared stepper and card visual system

- **Outcome:** Extracted the reusable stepper modal shell, fixed modal body scrolling for long model/repository lists, styled its scrollbar, and standardized primary card surfaces.
- **Scope:** `web-admin/src/components/stepper`, wizard styles/components, `web-admin/src/styles/_mixins.scss`, connection dashboard, execution surfaces, and Gitea styles.
- **Validation:** `web-admin npm run lint`; `web-admin npm run build`; and `go test ./...` passed. Build retains the existing multiple-lockfile workspace warning.
- **Notes updated:** [[../architecture/overview|Architecture]].
- **Limitations:** Specialized controls, badges, node states, and modal chrome retain their intentional interaction-specific styling.

## 2026-07-16 — Manual review dispatch and execution history

- **Outcome:** Replaced the console primary new-connection action with an authenticated manual review flow, added open-PR validation and queue publishing, accessible feedback, and historical execution selection without live polling overwrites.
- **Scope:** `internal/gitea`, `internal/admin`, manual-review/console/executions components, and styles.
- **Validation:** `gofmt`; `go test ./...`; `web-admin npm run lint`; `web-admin npm run build` passed. Build retains the existing multiple-lockfile workspace warning.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../features/index|Features]], [[../decisions/log|Decision Log]].
- **Limitations:** Run metadata is inferred from existing sanitized directory names; no log format change was introduced and no browser test suite exists.

## 2026-07-16 — Manual review visual contrast correction

- **Outcome:** Restored readable text and consistent blue identity colors across the manual-review modal and shared stepper controls in light and dark themes.
- **Scope:** `web-admin/src/app/globals.scss`, `web-admin/src/styles/_tokens.scss`, `web-admin/src/styles/_ui.scss`, `web-admin/src/components/connections/connection-wizard.scss`, and `web-admin/src/components/manual-review/manual-review-modal.scss`.
- **Validation:** `web-admin npm run lint` and `web-admin npm run build` passed after the contrast follow-up. Build retains the existing multiple-lockfile workspace warning.
- **Notes updated:** This change log.
- **Limitations:** No browser visual regression suite exists; final confidence depends on the static checks and manual theme inspection.

## 2026-07-16 — Ollama local and cloud integrations

- **Outcome:** Added Ollama local and Ollama Cloud connection modes, optional Bearer authentication, cloud catalog access, safe cloud unload behavior, and a shared chat metadata contract with OpenRouter.
- **Scope:** `internal/ai`, `internal/ollama`, `internal/openrouter`, `internal/agents/reviewer.go`, `internal/admin/setup.go`, Ollama tests, README, and the connection wizard.
- **Validation:** `gofmt`; `go test ./...`; `web-admin npm run lint` passed.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../architecture/feature-map|Feature Map]], [[../decisions/log|Decision Log]].
- **Limitations:** Cloud behavior depends on the configured Ollama-compatible service and its API-key permissions; no browser test suite exists.

