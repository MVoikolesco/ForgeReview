# Arquitetura atual

O backend Go/Gin e o painel Next.js são processos separados no Compose. `cmd/api` e `cmd/server` compartilham `internal/app`; `APP_MODE` decide se o processo migra/serve HTTP ou consome Redis. `APP_ENVIRONMENT` decide se o serviço web executa `npm run dev` ou o build de produção. Em desenvolvimento, o Compose monta `web-admin/` em `/workspace` e mantém as dependências em um volume nomeado para hot reload sem sobrescrever `node_modules`.

- `internal/config`: ambiente e defaults.
- `internal/database`: SQLite, migrations embutidas e seed.
- `internal/http`: Gin, CORS, recovery, logging, Basic Auth, handlers e respostas legadas/versionadas.
- `internal/review`: domínio, policies, pending reviews, repository e `PipelineEngine` orientado pelas versões e etapas persistidas no banco. Executors registrados implementam preparação, planejamento, revisão por grupos, consolidação, verificação, formatação e publicação.
- `internal/queue/redis`: Redis Streams, ack, heartbeat e métricas.
- `internal/providers`: Ollama, OpenAI-compatible/OpenRouter e Gemini.
- `internal/integrations/gitea`: diff, publicação, catálogo e resolver de instâncias.
- `web-admin`: Next/React, cliente Basic Auth e visualização do fluxo.

Detalhes: [[system-map|mapa do sistema]], [[data-model|modelo de dados]], [[contracts|contratos]] e [[migration-audit|auditoria]].
