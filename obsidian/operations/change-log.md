# Change log

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
