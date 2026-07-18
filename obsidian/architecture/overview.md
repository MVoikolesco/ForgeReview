# Arquitetura atual

O backend novo é independente do conteúdo de `OldGoProject/`. `cmd/api` e `cmd/server` compartilham `internal/app`; o modo de execução é definido por `APP_MODE`.

- `internal/config`: leitura e validação centralizadas de ambiente.
- `internal/database`: SQLite, migrations embutidas e seed de providers.
- `internal/http`: Gin, CORS, recovery, logging, Basic Auth, handlers e adaptadores de resposta.
- `internal/review`: domínio, repository, service, split de diff e persistência de steps.
- `internal/queue/redis`: Redis Streams isolado dos handlers.
- `internal/providers`: interface comum e clientes Ollama, OpenAI-compatible e Gemini.
- `internal/integrations/gitea`: diff e publicação de review.

O frontend continua consumindo as rotas administrativas legadas, enquanto `/api/v1` oferece o contrato versionado para novas integrações.
