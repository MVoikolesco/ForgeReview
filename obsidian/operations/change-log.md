# Change log

## 2026-07-18

- Ativada a área de Configurações do console com tabs dedicadas para visão geral, perfis, pipeline e tipos de etapa. A UI usa cards e modais responsivos, replica a seleção efetiva do runtime e consome o novo endpoint agregado e somente leitura `GET /api/admin/review/settings`.
- Migrado o pipeline para o modelo banco-first da 2.0. A migration 012 adiciona contratos, tipos, definições, versões, etapas, transições, snapshots, execuções, artifacts e reservas de publicação. O seed materializa o fluxo atual e versões equivalentes para profiles existentes; `PipelineEngine` substitui a ordem hardcoded, resolve executors registrados e preserva `Result`, `review_steps`, aprovação manual e publicação Gitea. Publicações recebem marca idempotente e estados incertos são reconciliados no Gitea após janela de segurança.
- Restaurado o pipeline de review multi-etapas sobre a arquitetura Gin: preparação e filtros, planejamento de grupos, revisão com retentativa de contrato, consolidação, verificação, formatação, fallbacks determinísticos e metadata de progresso persistida. O serviço continua usando a fila, políticas, publicação e pré-publicação atuais.
- Restaurado o pipeline de review multiestágio no worker Gin sem alterar rotas, fila Redis, publicação manual ou o cliente Gitea. Os adapters Ollama, OpenAI-compatible e Gemini agora expõem chat bruto limitado por estágio; metadados de chamadas e eventos detalhados persistem em `review_steps` e são retornados pela observabilidade.
- O Compose monta `web-admin/` no workspace do alvo `development`, com `node_modules` em volume nomeado e polling habilitado, para que alterações locais acionem o hot reload do Next.js sem impactar o runtime de produção.
- O registro HTTP foi modularizado: `internal/http/router.go` conserva o único ponto de composição em `RegisterRoutes`; os domínios `health`, `webhook`, `reviews` e `admin` foram movidos para pacotes próprios, cada qual com `router.go` e `RegisterRoutes`. Os contratos de paths, métodos e autenticação foram preservados e cobertos em `internal/http/router_test.go`. Ver [[architecture/system-map|mapa do sistema]].

## 2026-07-17

- Implementado backend novo com Gin, separado do legado histórico.
- Auditoria comparou `7779b3a` com `e1227cb` e identificou regressões de contratos, worker, policies, Gitea e pipeline.
- Corrigidos auth da API versionada, assets Next, webhook form, migration 009, resolução Gitea, validação de IA, cancelamento, heartbeat e continuidade do worker.
- Restauradas integrações administrativas de teste, organizações, repositórios, PRs e catálogo real Ollama/OpenRouter.
- Setup de conexão passou a proteger a API key, persistir parâmetros e concluir conexão/modelo/profile/policy em uma transação.
- Adicionada `pending_reviews` e fluxo approve/reject/rerun compatível com o painel.
- Compose passou a subir `web` com alvo `development`/`production` conforme `APP_ENVIRONMENT`.
- Documentação ampliada em [[architecture/system-map|mapa]], [[architecture/data-model|dados]], [[architecture/migration-audit|auditoria]] e [[operations/runbook|runbook]].

Limitações atuais e itens não reimplementados estão em [[architecture/migration-audit|Auditoria da migração]].
