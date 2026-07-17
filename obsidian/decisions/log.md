# Decision Log

Record durable decisions using this structure:

## YYYY-MM-DD — Decision title

- **Context:** Why a decision was required.
- **Decision:** What was chosen.
- **Rationale:** Why this option was selected.
- **Consequences:** Benefits, costs, risks, and follow-up work.
- **Affected paths:** Relevant repository paths.
- **Related notes:** Links such as [[Architecture]] or [[Feature Map]].

## 2026-07-16 — Protect Gitea tokens at rest

- **Context:** The admin console needs to test and persist Gitea credentials without exposing secrets through CRUD responses.
- **Decision:** Store new tokens as AES-GCM ciphertext using `GITEA_TOKEN_ENCRYPTION_KEY`; accept tokens only on POST/PATCH/test and retain `token_env_name` as a legacy fallback.
- **Rationale:** This limits secret exposure while allowing existing deployments to keep working during migration.
- **Consequences:** Operators must provision a 32-byte encryption key for new or rotated credentials; legacy environment-backed instances remain supported.
- **Affected paths:** `internal/admin/gitea.go`, `internal/store/migrations/008_gitea_token_ciphertext.sql`, `internal/gitea/client.go`, `web-admin/src/components/gitea/gitea-area.tsx`.
- **Related notes:** [[Architecture]], [[Feature Map]]

## 2026-07-16 — Keep AI defaults explicit and scoped

- **Context:** Managing a connection or selecting a model must not silently change the global provider route, and destructive changes must preserve foreign-key references.
- **Decision:** Scope connection defaults by provider, expose add-model mode from connection details, require explicit model-default selection, and execute model/connection deletion transactionally with promotion or clear conflict responses.
- **Rationale:** This prevents surprising routing changes and avoids broken review profiles while reusing the existing catalog and wizard UX.
- **Consequences:** Removing the last referenced model/connection may be rejected until a replacement exists; operators must explicitly choose global/provider defaults.
- **Affected paths:** `internal/admin/handler.go`, `internal/admin/setup.go`, `web-admin/src/components/connections`, `web-admin/src/components/console/console.tsx`.
- **Related notes:** [[Architecture]], [[Feature Map]], [[../operations/change-log|Change Log]]

## 2026-07-16 — Dispatch manual reviews through the admin boundary

- **Context:** Operators need to review a selected open PR on demand without using the public webhook path.
- **Decision:** Use a `StepperModal` catalog flow and authenticated `POST /api/admin/reviews/manual`; validate the PR with the selected saved Gitea client and publish `Manual: true` plus `GiteaInstanceID`.
- **Rationale:** This reuses existing credential protection and queue wiring while preventing arbitrary instance/repository/PR combinations from entering the queue.
- **Consequences:** The selected Gitea token must be available to the API; dispatch is asynchronous and reports accepted queueing rather than completion.
- **Affected paths:** `internal/admin/manual_review.go`, `internal/admin/gitea.go`, `internal/gitea/client.go`, `web-admin/src/components/manual-review`, `web-admin/src/components/console/console.tsx`.
- **Related notes:** [[../architecture/overview|Architecture]], [[../features/index|Features]], [[../operations/change-log|Change Log]]

## 2026-07-16 — Resolve Gitea clients at job execution

- **Context:** The worker previously used one environment-backed Gitea client even when the admin catalog contained selected repositories and instances.
- **Decision:** Carry an optional `GiteaInstanceID` in Redis jobs and resolve credentials by explicit instance, repository selection, then default instance; preserve environment fallback for legacy jobs.
- **Rationale:** This is the smallest integration that activates existing catalog data without changing the frontend or exposing tokens to the queue.
- **Consequences:** A configured instance must have decryptable ciphertext or its legacy token environment variable; workers may run without Gitea environment variables when catalog configuration is available.
- **Affected paths:** `internal/queue`, `internal/queue/redis`, `internal/gitea/resolver.go`, `internal/agents/reviewer.go`, `cmd/server/main.go`.
- **Related notes:** [[Architecture]], [[Feature Map]], [[../operations/change-log|Change Log]]

## 2026-07-16 — Keep one manageable Gitea connection

- **Context:** The Gitea screen mixed registration with ongoing management and allowed ambiguous multiple-instance setup.
- **Decision:** Present the configured connection as a summary card, reuse the existing stepper for edits, and allow a new registration only after deletion; delete repository mappings transactionally with the instance.
- **Rationale:** This makes the lifecycle explicit and prevents orphaned repository selections.
- **Affected paths:** `internal/admin/gitea.go`, `internal/admin/handler_test.go`, `web-admin/src/components/gitea`.
- **Related notes:** [[Architecture]], [[Feature Map]], [[../operations/change-log|Change Log]]

## 2026-07-16 — Use one Ollama contract for local and cloud

- **Context:** The existing Ollama client only supported a local unauthenticated endpoint, while the review agent coupled metadata to that client.
- **Decision:** Keep one `ollama` provider, select local or cloud through the connection URL and optional API-key environment-variable reference, and share chat metadata through `internal/ai`.
- **Rationale:** This preserves existing local installations, avoids a duplicate provider implementation, and keeps routing independent from provider-specific response types.
- **Consequences:** Cloud requires a runtime environment variable and network access; model unload is intentionally skipped for authenticated cloud connections.
- **Affected paths:** `internal/ai`, `internal/ollama`, `internal/openrouter`, `internal/agents/reviewer.go`, `internal/admin/setup.go`, `web-admin/src/components/connections/connection-wizard.tsx`.
- **Related notes:** [[../architecture/overview|Architecture]], [[../architecture/feature-map|Feature Map]], [[../operations/change-log|Change Log]]

## 2026-07-17 — Externalizar instruções da pipeline sem stacks

- **Context:** A pipeline stateless dependia de regras hardcoded e de composição de prompts por stack, dificultando edição e permitindo contexto desigual entre estágios.
- **Decision:** Exigir quatro Markdown configurados em YAML e enviar o mesmo diff canônico a todos os estágios LLM; remover seleção/composição por stack e templates legados.
- **Rationale:** Centraliza instruções, schemas e regras editáveis, preservando no Go somente a intercalação de dados dinâmicos e os controles determinísticos.
- **Consequences:** Arquivo ausente ou vazio interrompe a execução explicitamente; divergências de importação precisam de prova de importação e uso alterados no diff. O formatter não pode publicar comentário de importação ou alias sem achado validado correspondente no mesmo arquivo e linha.
- **Affected paths:** `config/review-prompts.yaml`, `prompts/*.md`, `internal/review/promptconfig`, `internal/review/pipeline`.
- **Related notes:** [[../architecture/overview|Architecture]], [[../architecture/feature-map|Feature Map]]

## 2026-07-17 — Persistir chaves de IA exclusivamente no SQLite

- **Context:** Chaves de IA em variáveis de ambiente impediam rotação segura e criavam comportamento implícito entre API e worker.
- **Decision:** Até v1.0, `ai_connections.api_key_ciphertext` substitui o contrato legado. Chaves são write-only, AES-256-GCM com AAD por conexão/campo e usam a mesma `GITEA_TOKEN_ENCRYPTION_KEY` dos tokens Gitea. Não há leitura, migração automática ou fallback de `.env` para IA.
- **Rationale:** Uma única fonte de verdade evita substituição de ciphertext entre conexões e falha explicitamente quando a credencial não é utilizável.
- **Consequences:** Backup do SQLite é obrigatório antes do rollout; API deve migrar e receber/rotacionar chaves antes do worker, que usa a mesma chave mestra. Conexões autenticadas antigas sem chave persistida são desabilitadas; Ollama local continua sem chave.
- **Affected paths:** `internal/secrets`, `internal/store/migrations/009_ai_connection_secret.sql`, `internal/admin`, `internal/reviewconfig`, `internal/agents`, `web-admin/src/components/connections`.
- **Related notes:** [[../architecture/overview|Architecture]], [[../operations/change-log|Change Log]]

## 2026-07-17 — Classificar autenticação por conexão de IA

- **Context:** `ai_providers.auth_type` classifica o provider Ollama como sem autenticação e não distingue sua rota local da rota Cloud, permitindo que worker e catálogo omitisse a chave Cloud.
- **Decision:** Persistir `ai_connections.requires_auth` como classificação imutável e definida pelo servidor. Setup marca Ollama Cloud e OpenRouter; a migration 009 também deriva autenticação de todo `ai_providers.auth_type` diferente de `none` (incluindo Gemini e Groq), com metadados de endpoint/nome somente para distinguir Ollama Cloud do local. Catálogo, teste administrativo e worker usam exclusivamente esse campo para decriptar e enviar Bearer; o catálogo local do Ollama descarta qualquer chave submetida.
- **Rationale:** A decisão por conexão preserva Ollama local sem credencial e elimina a inferência insegura por `auth_type` do provider.
- **Consequences:** Uma conexão autenticada com ciphertext ausente ou inválido é rejeitada antes da chamada ao provider; operadores de conexões legadas precisam informar a chave e reativá-las. Ollama local não aceita nem transmite Bearer, mesmo quando o cliente envia uma chave.
- **Affected paths:** `internal/store/migrations/009_ai_connection_secret.sql`, `internal/admin`, `internal/reviewconfig`, `web-admin/src/components/connections/connection-wizard.tsx`.
- **Related notes:** [[../architecture/overview|Architecture]], [[../architecture/feature-map|Feature Map]]

