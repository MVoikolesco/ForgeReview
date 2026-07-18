# Mapa do sistema

Relaciona cada fronteira, responsabilidade e ponto de reaproveitamento. A fonte de verdade de comportamento é o código; esta nota descreve o estado observado em 2026-07-17.

## Processos

| Processo | Entrada | Responsabilidade | Dependências | Saída |
| --- | --- | --- | --- | --- |
| `cmd/api` | `APP_MODE=api` | abre SQLite, migra/seed, expõe Gin | SQLite, Redis | HTTP |
| `cmd/server` | `APP_MODE=worker` | exige schema e consome Stream | SQLite, Redis, Gitea, IA | publicação/status |
| `web` | `APP_ENVIRONMENT` | Next dev ou Next production | API pela rede Compose | painel |
| Redis | Stream `REDIS_STREAM` | entrega jobs e métricas | volume Redis | jobs |
| SQLite | `DATABASE_DSN` | configuração e estado de reviews | volume `/data` | dados persistentes |

## Dependências internas

- `internal/app`: composição de configuração, banco, queue, service, provider e HTTP.
- `internal/config`: ambiente e defaults; só aceita SQLite e `APP_MODE` `api`/`worker`.
- `internal/database`: migrations embutidas em `internal/database/migrations/*.sql`, seed de quatro providers e verificação de schema.
- `internal/http`: router Gin, Basic Auth, CORS, recovery, log, handlers e envelopes.
- `internal/review`: `Service` coordena o fluxo; `Repository` persiste reviews, steps, policies, prompts e pendências.
- `internal/queue/redis`: Redis Streams, consumer group, heartbeat, ack e métricas.
- `internal/integrations/gitea`: diff, publicação, catálogo de organizações/repos/PRs e resolução de instância/token.
- `internal/providers`: contrato `LLMProvider` e adaptadores Ollama, OpenAI-compatible/OpenRouter e Gemini.
- `internal/security`: AES-GCM para secrets administrativos.

## Fluxo de dados

```text
Gitea webhook / painel
        |
        v
Gin -> reviews + review_steps -> Redis XADD
                                      |
                                      v
                         worker XREADGROUP / XACK
                                      |
                    resolver Gitea -> diff -> splitDiff
                                      |
                           provider -> JSON validado
                                      |
                         resultado SQLite / pending_reviews
                                      |
                      aprovação -> Gitea pull review
```

## Regras de fronteira

- O API é o único processo autorizado a migrar e seedar o banco.
- O worker não inicia sem a tabela `reviews`.
- Jobs sem `review_id` ainda são aceitos: o worker cria um ID e persiste o job como legado de fila.
- Jobs com `gitea_instance_id` usam essa instância; sem ID, primeiro tenta o repositório cadastrado e depois a instância default habilitada; sem configuração, cai no cliente de ambiente.
- O token cifrado do banco é preferido. Cliente de ambiente só é fallback quando não há instância resolvida.
- O resultado final não expõe secrets e o CRUD troca ciphertext por flags `api_key_configured`.

## Pontos de reaproveitamento

- `internal/contracts/review.go`: contrato comum entre provider, domínio e publicação.
- `internal/integrations/gitea/client.go`: cliente HTTP pode ser usado em handlers e worker.
- `internal/providers/provider.go`: novos providers devem implementar `LLMProvider`; OpenAI-compatible cobre Groq e OpenRouter.
- `internal/review/repository.go`: novo estado ou artefato de review deve ser persistido aqui, não diretamente em handlers.
- `web-admin/src/lib/admin-client.ts`: único cliente de Basic Auth do painel.
- `web-admin/src/lib/contracts.ts`: tipos compartilhados do frontend administrativo.
- Commits anteriores a `7779b3a`: contêm validação de contrato, resolver, worker resiliente, cliente Gitea e handlers reais que servem como referência, mas exigem adaptação aos tipos atuais.
