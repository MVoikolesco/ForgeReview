# ForgeReview

Backend de code review automatizado para Gitea, reescrito em Go com Gin. O painel existente em `web-admin/` é mantido sem alterações de contrato.

## Arquitetura

```text
cmd/api e cmd/server
        │
internal/app ─ config ─ database (SQLite + migrations)
        ├── http (Gin, handlers, middlewares, responses)
        ├── review (service, repository, steps, contrato final)
        ├── queue/redis (Redis Streams)
        ├── integrations/gitea
        └── providers (Ollama, OpenAI-compatible/OpenRouter/Groq, Gemini)
```

`OldGoProject/` contém a implementação legada preservada como referência. O novo backend não importa nem executa código dessa pasta.

## Execução

```sh
cp .env.example .env
go test ./...
go run ./cmd/server
```

Para execução assíncrona, use dois processos com o mesmo `.env`:

```sh
APP_MODE=api go run ./cmd/api
APP_MODE=worker go run ./cmd/server
```

O Docker Compose continua usando `APP_MODE=api` no serviço HTTP e `APP_MODE=worker` no consumidor Redis. O worker exige que a API tenha aplicado as migrations primeiro.

## Contratos HTTP

As rotas legadas permanecem disponíveis: `GET /health`, `POST/GET /webhook`, `POST /review` e `/api/admin/*` com Basic Auth. O painel chama `/api/admin/` e recebe os arrays e objetos legados, incluindo `api_key_configured` sem expor secrets.

A API versionada adiciona:

```text
GET  /api/v1/reviews
POST /api/v1/reviews
GET  /api/v1/reviews/:id
GET  /api/v1/reviews/:id/status
GET  /api/v1/reviews/:id/steps
GET  /api/v1/reviews/:id/result
POST /api/v1/reviews/:id/reprocess
POST /api/v1/reviews/:id/cancel
```

Respostas novas usam `{success,data,error}`. As rotas administrativas mantêm o formato legado para não exigir alteração no frontend.

## Review e retenção

O job é publicado no Redis Stream configurado por `REDIS_STREAM`, com consumer group e consumer configuráveis. O serviço registra status e steps (`recebido`, `enfileirado`, `processando`, `buscando_diff`, `enviando_para_ia`, `recebendo_resposta_parcial`, `agregando_resultado`, `publicando_comentario`, `concluido` ou `falhou`) no SQLite.

O SQLite persiste apenas metadados, steps e resultado final. Diff bruto, prompts montados e respostas intermediárias não são persistidos pelo novo fluxo. Logs técnicos não incluem tokens; `LOG_LEVEL=debug` pode ser usado explicitamente para diagnóstico.

O resultado preserva `comments[].file`, `line`, `severity`, `decision_reason` e `comment`, além de `final_review`. O processamento divide o diff por limite de caracteres e arquivos, valida JSON, agrega comentários e remove duplicatas.

## Configuração administrativa

Providers, conexões, modelos, parâmetros, profiles, prompts, policies, instâncias Gitea e repositórios são mantidos em SQLite por migrations. As chaves de IA são aceitas somente em escrita e cifradas com AES-GCM usando `GITEA_TOKEN_ENCRYPTION_KEY` (32 bytes). Providers Gemini e Groq já são cadastrados no catálogo; seus adaptadores seguem o contrato comum e podem ser configurados pelo banco.

Prompts ativos do profile padrão são carregados pelo serviço. Se não houver prompt cadastrado, o sistema usa uma instrução mínima de compatibilidade; o prompt padrão recomendado fica em `prompts/` para cadastro operacional.

## Frontend

O frontend é Next.js/React, não Nuxt. Não foi alterado. Em desenvolvimento:

```sh
cd web-admin
npm ci
npm run dev
```

O build Docker continua compilando `web-admin` e servindo o resultado pelo backend.
