# Auditoria da migração Go para Gin

## Base comparada

- Atual: `7779b3a` (`refact: reestruturado para utilizar gin de maneira organizada e padronizada`).
- Anterior: pai `e1227cb` e commits anteriores no mesmo histórico.
- `OldGoProject/` é citado em documentação antiga, mas não existe no checkout.
- A migração removeu aproximadamente 12 mil linhas, incluindo pipeline, testes, resolver Gitea, worker e handlers administrativos anteriores.

## Correções aplicadas

- `/api/v1` passou a exigir o mesmo Basic Auth administrativo; antes era público.
- Assets usam `http.FileServer` em vez de `c.File("./web")`, permitindo `/_next/static/*`.
- Webhook voltou a aceitar form URL encoded com campo `payload`.
- Worker continua após falha de review, registra falha e renova heartbeat a cada 5 segundos.
- Jobs de versões anteriores sem `review_id` são materializados no SQLite.
- `error_message` é salvo também em status terminal.
- Migration 009 desativa foreign keys durante o rebuild de `ai_connections` e restaura a proteção.
- Resolver Gitea usa ID do job, repositório/default e ciphertext do banco.
- Handlers Gitea consultam `/user`, organizações, repositórios e PRs; catálogo consulta Ollama/OpenRouter em vez de retornar dados falsos.
- Policies de limites/publicação/rejeição são consultadas; cancelamento é verificado antes de IA e publicação.
- Pré-publicação foi persistida em `pending_reviews` com approve/reject/rerun compatíveis com o painel.
- Compose adicionou serviço Next e alvo de build por `APP_ENVIRONMENT`.

## Regressões que permanecem

- O pipeline multi-etapas anterior não foi restaurado. Planner, grupos, consolidator, verifier e formatter estão ausentes.
- Colunas de policy do pipeline antigo ainda existem no banco, mas não são executadas pelo novo `Service`.
- `review-prompts.yaml` e variáveis `REVIEW_PLANNER_*` não são carregados pelo serviço; só prompt SQLite/default e limites básicos têm efeito.
- O contrato `/api/v1/reviews/:id/status` retorna o objeto completo da review, não um payload de status dedicado.
- CORS global é `*`; não é adequado para exposição pública.
- Credenciais Basic Auth ficam em `sessionStorage` no painel e o Compose local usa HTTP; exigir HTTPS/reverse proxy em ambientes reais.
- O frontend permaneceu sem alteração de UI de etapas e, por isso, mostra etapas não executadas como aguardando.

## Referências reutilizáveis do legado

- `internal/gitea/client.go`: métodos HTTP e tipos de organizações/repositórios/PRs.
- `internal/gitea/resolver.go`: estratégia de resolução de instância.
- `internal/worker/worker.go`: loop resiliente e heartbeat periódico.
- `internal/review/final_response_contract.go`: validação estrita do contrato final.
- `internal/admin/setup.go`: catálogo real e setup transacional como referência de comportamento.
- `internal/admin/pending_review.go`: semântica anterior de autorização, rejeição e rerun.
- `internal/review/pipeline/*`: etapas e limites que ainda precisam de uma migração deliberada, não de cópia cega.
