# Change Log

Record meaningful changes using this structure:

## YYYY-MM-DD — Change title

- **Outcome:** What changed for users or operators.
- **Scope:** Components and paths affected.
- **Validation:** Commands and observed results.
- **Notes updated:** Related vault notes.
- **Limitations:** Remaining constraints or risks.

## 2026-07-17 — Falha fechada na criação genérica de conexões de IA

- **Outcome:** A criação genérica agora classifica autenticação pelo `auth_type` persistido do provider, com exceção explícita apenas para Ollama local. Gemini e Groq sem chave são rejeitados antes de criar conexão ativa.
- **Scope:** `internal/admin/handler.go`, `internal/admin/handler_test.go`.
- **Validation:** Pendente da validação direcionada admin/store, suíte Go completa e verificação de diff.
- **Notes updated:** [[../architecture/overview|Architecture]].
- **Limitations:** A distinção de Ollama local continua limitada aos endpoints locais e ao nome `Ollama local` aceitos pelo contrato atual.

## 2026-07-17 — Migração de chaves de IA cifradas

- **Outcome:** Chaves de IA passaram a ser write-only e persistidas cifradas; API, catálogo e worker usam exclusivamente a chave do SQLite. Ollama local permanece sem autenticação. A migração de bancos legados preserva IDs e relações de modelos, parâmetros e profiles.
- **Scope:** Migração 009, executor de migrações e teste de regressão em `internal/store`, `internal/secrets`, admin/setup/reviewconfig/worker, console e documentação de instalação.
- **Validation:** `gofmt`, `go test ./...`, `web-admin npm run lint`, `web-admin npm run build`, `PRAGMA foreign_key_check` no teste de migração e verificações de referências legadas sem fonte runtime encontrada.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../decisions/log|Decision Log]], [[../architecture/feature-map|Feature Map]].
- **Limitations:** O rollback requer restauração do backup SQLite anterior ao rollout.

## 2026-07-17 — Cobertura de autenticação para conexões legadas

- **Outcome:** A migração 009 agora desabilita toda conexão legada autenticada sem ciphertext, com base no `auth_type` do provider; Gemini e Groq são incluídos. O catálogo local do Ollama descarta uma API key fornecida e não transmite Bearer.
- **Scope:** `internal/store/migrations/009_ai_connection_secret.sql`, `internal/store/store_test.go`, `internal/admin/setup.go`, `internal/admin/setup_test.go`.
- **Validation:** 2026-07-17: `gofmt` nos Go afetados, `go test ./...`, `web-admin npm run lint`, `web-admin npm run build` e `git diff --check` passaram. O build mantém o aviso não bloqueante de múltiplos lockfiles e raiz inferida pelo Next.js.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../decisions/log|Decision Log]].
- **Limitations:** A identificação de Ollama Cloud legado continua dependente do endpoint `https://ollama.com` ou do nome histórico `Ollama Cloud`.

## 2026-07-17 — Falha fechada por autenticação de conexão de IA

- **Outcome:** Ollama Cloud e OpenRouter agora usam uma classificação explícita por conexão, decriptam a chave SQLite e enviam Bearer; Ollama local permanece sem chave. Conexões autenticadas legadas sem ciphertext são desabilitadas e ciphertext inválido é rejeitado.
- **Scope:** `internal/store/migrations/009_ai_connection_secret.sql`, `internal/admin`, `internal/reviewconfig`, wizard de conexões e testes de regressão.
- **Validation:** 2026-07-17: `gofmt` nos Go alterados, `go test -count=1 ./...`, `go vet ./...`, `web-admin npm run lint`, `web-admin npm run build` e `git diff --check` passaram. Busca estática em Go encontrou `api_key_env_name` somente nos fixtures de migração de `internal/store/store_test.go`; não há uso runtime. Os testes cobrem ciphertext SQLite e Bearer para Ollama Cloud/OpenRouter e a ausência de segredo para Ollama local. O build mantém o aviso não bloqueante de dois lockfiles e da raiz inferida pelo Next.js.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../architecture/feature-map|Feature Map]], [[../decisions/log|Decision Log]].
- **Limitations:** A classificação de conexões Cloud legadas depende do endpoint `https://ollama.com` ou do nome histórico `Ollama Cloud`; conexões desabilitadas exigem cadastro/rotação da chave.

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

## 2026-07-17 — Pipeline com prompts editáveis e diff canônico

- **Outcome:** Substituiu prompts ativos hardcoded e composição por stack por quatro Markdown obrigatórios; cada request LLM ativo recebe o mesmo diff truncado quando aplicável. Achados de importação sem prova no diff são descartados deterministicamente.
- **Scope:** `config/review-prompts.yaml`, `prompts/`, `internal/agents/reviewer.go`, `internal/review/promptconfig`, `internal/review/pipeline`, testes, `README.md` e `CONTRIBUTING.md`.
- **Validation:** `gofmt -w ...`; `go test ./...`.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../architecture/feature-map|Feature Map]], [[../decisions/log|Decision Log]].
- **Limitations:** A evidência de divergência de importação precisa conter os trechos de importação e uso alterados; a pipeline não busca arquivos fora do diff.

## 2026-07-17 — Vincular comentários finais de importação a achados comprovados

- **Outcome:** Comentários finais de importação ou alias agora exigem um achado validado de divergência de importação no mesmo arquivo e linha, com evidência positiva no diff; achados comuns não autorizam esses comentários e comentários não relacionados permanecem inalterados.
- **Scope:** `internal/review/pipeline/pipeline.go`, `internal/review/pipeline/validation.go`, e testes da pipeline.
- **Validation:** `gofmt` e `go test ./internal/review/pipeline ./internal/agents` passaram.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../decisions/log|Decision Log]].

