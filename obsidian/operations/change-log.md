# Change Log

Record meaningful changes using this structure:

## YYYY-MM-DD — Change title

- **Outcome:** What changed for users or operators.
- **Scope:** Components and paths affected.
- **Validation:** Commands and observed results.
- **Notes updated:** Related vault notes.
- **Limitations:** Remaining constraints or risks.

## 2026-07-17 — Sincronizar cards entre execuções

- **Outcome:** O fluxo troca corretamente os cards ao iniciar outra PR ou selecionar uma execução histórica; reexecuções da mesma PR não acumulam eventos da execução anterior.
- **Scope:** `web-admin/src/components/executions/executions-flow.tsx`, `internal/agents/review_logs.go` e `internal/admin/observability.go`.
- **Validation:** `gofmt`, `go test ./...`, `web-admin npm run lint`, `web-admin npm run build` e `git diff --check` passaram. O build mantém o aviso existente de múltiplos lockfiles do Next.js.
- **Notes updated:** [[../architecture/overview|Architecture]] e [[../features/index|Features]].
- **Limitations:** Não há suíte de testes de navegador; a validação visual final depende da execução do painel no ambiente configurado.

## 2026-07-17 — Compatibilizar respostas de modelos Ollama

- **Outcome:** Chamadas estruturadas desabilitam thinking e aceitam respostas no campo `response` quando o endpoint compatível não retorna `message.content`; respostas realmente vazias continuam falhando explicitamente.
- **Scope:** `internal/ollama/client.go` e `internal/ollama/client_test.go`.
- **Validation:** `gofmt`, testes direcionados de Ollama/agente/pipeline e `git diff --check` passaram.
- **Notes updated:** [[../architecture/overview|Architecture]].
- **Limitations:** Modelos ou servidores que retornem somente raciocínio sem conteúdo final continuam inválidos para os contratos JSON do pipeline.

## 2026-07-17 — Armazenar artefatos temporários de review no Redis

- **Outcome:** Progresso, logs, diff, prompts, respostas e decisões manuais deixam de depender de arquivos graváveis e passam a usar o Redis compartilhado com TTL de 12 horas.
- **Scope:** `internal/reviewlog`, `internal/queue/redis`, agente reviewer, endpoints administrativos, Compose, entrypoint, `build-prod.sh` e documentação.
- **Validation:** `gofmt`, `go test ./...`, `web-admin npm run lint`, `git diff --check` e validação do Compose passaram quando executados no ambiente.
- **Notes updated:** [[../architecture/overview|Architecture]] e este Change Log.
- **Limitations:** Artefatos expiram após 12 horas e não são destinados a auditoria permanente; o SQLite continua necessário para configuração.

## 2026-07-17 — Corrigir resolução do modelo padrão

- **Outcome:** O perfil padrão passa a usar a cadeia atual de provider, conexão e modelo padrão; a seleção de perfil por repositório também ficou determinística.
- **Scope:** `internal/reviewconfig/provider.go` e `internal/reviewconfig/provider_test.go`.
- **Validation:** `gofmt`, `go test ./...`, `web-admin npm run lint` e `git diff --check` passaram.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../features/index|Features]], [[../decisions/log|Decision Log]].
- **Limitations:** Perfis específicos de repositório continuam usando o modelo explicitamente associado e não acompanham defaults globais.

## 2026-07-17 — Aprovação manual antes da publicação de reviews

- **Outcome:** Reviews manuais podem pausar em `Pré-publicação`; o operador visualiza o payload final e autoriza, nega ou solicita nova execução pelo modal do fluxo.
- **Scope:** `internal/review`, `internal/agents`, `internal/admin/pending_review.go`, fluxo de execuções e política do console.
- **Validation:** `gofmt`, `go test ./...`, `git diff --check`, `web-admin npm run lint` e `web-admin npm run build` passaram. O build mantém o aviso existente de múltiplos lockfiles do Next.js.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../features/index|Features]], [[../decisions/log|Decision Log]].
- **Limitations:** A aprovação depende da execução ainda disponível no diretório de logs e não há suíte de testes de navegador.

## 2026-07-17 — Pacote de produção com frontend estático

- **Outcome:** `build-prod.sh` passou a compilar backend e Next.js, validar o frontend e gerar uma pasta `Prod/` autocontida para o Compose.
- **Scope:** `build-prod.sh`, `README.md`, Dockerfile e Compose gerados, artefatos `web/`, entrypoint, persistência de dados/logs e notas de arquitetura.
- **Validation:** `bash -n build-prod.sh`, execução do script com validação de artefatos, `docker compose config --quiet` quando Docker estiver disponível.
- **Notes updated:** [[../architecture/overview|Architecture]] e este Change Log.
- **Limitations:** A compilação da imagem ainda depende de Docker no ambiente de destino; o script não executa o deploy.

## 2026-07-17 — Classificação de segurança do endpoint Ollama

- **Outcome:** Um nome `Ollama local` não habilita mais endpoint remoto sem chave. Apenas URLs loopback permitem modo sem Bearer; endpoints remotos exigem ciphertext e o worker falha fechado quando ele falta.
- **Scope:** `internal/ai/auth.go`, create/update/setup/catalog/test em `internal/admin`, `internal/reviewconfig`, migration 009, wizard e regressões Go.
- **Validation:** 2026-07-17: `gofmt`, `go test ./...`, `web-admin npm run lint`, `web-admin npm run build` e `git diff --check` passaram. O build mantém o aviso não bloqueante de múltiplos lockfiles do Next.js.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../decisions/log|Decision Log]].
- **Limitations:** A migration não classifica loopback e exige reconfiguração de toda conexão legada; o runtime aceita somente URLs loopback verificáveis para novas conexões sem chave.

## 2026-07-17 — Corrigir caminho de prompts no runtime de produção

- **Outcome:** O worker passa a respeitar `REVIEW_PROMPT_CONFIG_PATH`, incluindo o caminho absoluto usado pelo Compose de produção; artefatos de prompts copiados para `Prod/` são normalizados como legíveis.
- **Scope:** `internal/config/config.go`, `internal/config/config_test.go` e `build-prod.sh`.
- **Validation:** `go test ./...`, `bash -n build-prod.sh docker-entrypoint.sh` e `git diff --check` passaram.
- **Notes updated:** [[../architecture/overview|Architecture]] e este Change Log.

## 2026-07-17 — Remover mounts de prompts no worker de produção

- **Outcome:** O worker usa os prompts e o YAML incorporados à imagem, sem substituir `/app/config` e `/app/prompts` por bind mounts com permissões dependentes do host.
- **Scope:** `build-prod.sh`, `Prod/compose.yaml` e nota de arquitetura.
- **Validation:** `bash -n build-prod.sh docker-entrypoint.sh`, `docker compose config --quiet` quando Docker estiver disponível e `git diff --check`.
- **Notes updated:** [[../architecture/overview|Architecture]] e este Change Log.

## 2026-07-17 — Persistir logs físicos de todas as reviews

- **Outcome:** A opção administrativa antiga que desabilitava artefatos foi removida; a persistência temporária de progresso, processo, diff, prompts e resposta é atualmente feita no Redis com TTL de 12 horas.
- **Scope:** `internal/agents`, `internal/reviewconfig`, `internal/admin/setup.go`, contratos e wizard do painel, `README.md` e notas de arquitetura.
- **Validation:** `gofmt`, `go test ./...`, `web-admin npm run lint` e `git diff --check`.
- **Notes updated:** [[../architecture/overview|Architecture]] e este Change Log.

## 2026-07-17 — Permitir decisão manual no volume de logs

- **Outcome:** O serviço API pode gravar a decisão e o progresso ao aprovar ou recusar uma review manual pendente.
- **Scope:** `build-prod.sh`, `Prod/compose.yaml` e nota de arquitetura.
- **Validation:** `bash -n build-prod.sh`, `docker compose config --quiet` quando Docker estiver disponível e `git diff --check`.
- **Notes updated:** [[../architecture/overview|Architecture]] e este Change Log.

## 2026-07-17 — Corrigir aprovação de jobs legados sem instância

- **Outcome:** A aprovação manual resolve a instância Gitea pelo repositório ou pela instância habilitada padrão quando o job pendente não contém `GiteaInstanceID`.
- **Scope:** `internal/admin/pending_review.go` e nota de arquitetura.
- **Validation:** `gofmt` e `go test ./internal/admin ./internal/agents ./internal/gitea` passaram.
- **Notes updated:** [[../architecture/overview|Architecture]] e este Change Log.

## 2026-07-17 — Uniformizar entrada manual da API

- **Outcome:** O endpoint administrativo de envio manual também resolve a instância Gitea pelo repositório ou padrão quando `instance_id` não é enviado; o job é enfileirado já com a instância resolvida.
- **Scope:** `internal/admin/manual_review.go`, `internal/admin/pending_review.go` e regressão de admin.
- **Validation:** `gofmt` e `go test ./...`.
- **Notes updated:** [[../architecture/overview|Architecture]] e este Change Log.

## 2026-07-17 — Migração fail-closed e erros remotos sanitizados

- **Outcome:** A migration 009 não usa mais heurística SQL para reconhecer loopback: toda conexão legada sem chave cifrada fica desabilitada até reconfiguração. Catálogo e teste de conexão não refletem bodies de erro de providers.
- **Scope:** `internal/store/migrations/009_ai_connection_secret.sql`, `internal/ai/auth_test.go`, `internal/admin/handler.go`, `internal/admin/setup.go`, e regressões de store/admin.
- **Validation:** 2026-07-17: `gofmt`, `go test ./...`, `web-admin npm run lint`, `web-admin npm run build` e `git diff --check` passaram. O build mantém o aviso não bloqueante de múltiplos lockfiles do Next.js.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../decisions/log|Decision Log]].
- **Limitations:** Operadores precisam reabilitar explicitamente conexões legadas, inclusive Ollama local; respostas sanitizadas preservam somente contexto e status HTTP.

## 2026-07-17 — Falha fechada na criação genérica de conexões de IA

- **Outcome:** A criação genérica agora classifica autenticação pelo `auth_type` persistido do provider, com exceção explícita apenas para Ollama local. Gemini e Groq sem chave são rejeitados antes de criar conexão ativa.
- **Scope:** `internal/admin/handler.go`, `internal/admin/handler_test.go`.
- **Validation:** 2026-07-17: `gofmt -w internal/admin/handler.go internal/admin/handler_test.go`, `go test -count=1 ./internal/admin ./internal/store`, `go test -count=1 ./...` e `git diff --check` passaram.
- **Notes updated:** [[../architecture/overview|Architecture]].
- **Limitations:** A distinção de Ollama local continua limitada aos endpoints locais e ao nome `Ollama local` aceitos pelo contrato atual.

## 2026-07-17 — Migração de chaves de IA cifradas

- **Outcome:** Chaves de IA passaram a ser write-only e persistidas cifradas; API, catálogo e worker usam exclusivamente a chave do SQLite. Ollama local permanece sem autenticação. A migração de bancos legados preserva IDs e relações de modelos, parâmetros e profiles.
- **Scope:** Migração 009, executor de migrações e teste de regressão em `internal/store`, `internal/secrets`, admin/setup/reviewconfig/worker, console e documentação de instalação.
- **Validation:** `gofmt`, `go test ./...`, `web-admin npm run lint`, `web-admin npm run build`, `PRAGMA foreign_key_check` no teste de migração e verificações de referências legadas sem fonte runtime encontrada.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../decisions/log|Decision Log]], [[../architecture/feature-map|Feature Map]].
- **Limitations:** O rollback requer restauração do backup SQLite anterior ao rollout.

## 2026-07-17 — Cobertura de autenticação para conexões legadas

- **Outcome:** A migração 009 desabilita toda conexão legada sem ciphertext, sem tentar inferir loopback por SQL; Gemini, Groq e Ollama legado são incluídos. O catálogo local do Ollama descarta uma API key fornecida e não transmite Bearer.
- **Scope:** `internal/store/migrations/009_ai_connection_secret.sql`, `internal/store/store_test.go`, `internal/admin/setup.go`, `internal/admin/setup_test.go`.
- **Validation:** 2026-07-17: `gofmt` nos Go afetados, `go test ./...`, `web-admin npm run lint`, `web-admin npm run build` e `git diff --check` passaram. O build mantém o aviso não bloqueante de múltiplos lockfiles e raiz inferida pelo Next.js.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../decisions/log|Decision Log]].
- **Limitations:** Conexões legadas, incluindo Ollama local, exigem reconfiguração explícita antes de reativação.

## 2026-07-17 — Falha fechada por autenticação de conexão de IA

- **Outcome:** Ollama Cloud e OpenRouter usam uma classificação explícita por conexão, decriptam a chave SQLite e enviam Bearer; novas conexões Ollama loopback permanecem sem chave. Toda conexão legada sem ciphertext é desabilitada e ciphertext inválido é rejeitado.
- **Scope:** `internal/store/migrations/009_ai_connection_secret.sql`, `internal/admin`, `internal/reviewconfig`, wizard de conexões e testes de regressão.
- **Validation:** 2026-07-17: `gofmt` nos Go alterados, `go test -count=1 ./...`, `go vet ./...`, `web-admin npm run lint`, `web-admin npm run build` e `git diff --check` passaram. Busca estática em Go encontrou `api_key_env_name` somente nos fixtures de migração de `internal/store/store_test.go`; não há uso runtime. Os testes cobrem ciphertext SQLite e Bearer para Ollama Cloud/OpenRouter e a ausência de segredo para Ollama local. O build mantém o aviso não bloqueante de dois lockfiles e da raiz inferida pelo Next.js.
- **Notes updated:** [[../architecture/overview|Architecture]], [[../architecture/feature-map|Feature Map]], [[../decisions/log|Decision Log]].
- **Limitations:** Conexões legadas desabilitadas exigem reconfiguração e reativação explícitas; não há fallback de ambiente para IA.

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

