# ForgeReview

Backend de code review automatizado para Gitea, escrito em Go com Gin, Redis Streams, SQLite e um painel Next.js em `web-admin/`.

## Arquitetura

```text
web (Next.js :3000) --rewrite /api--> api (Gin :8080)
                                      |-- SQLite + migrations
                                      |-- Redis Streams
                                      |-- Gitea
                                      `-- providers Ollama/OpenAI-compatible/Gemini
worker (Go) <------------------------- Redis
```

O histórico pré-Gin está nos commits anteriores do Git. A pasta `OldGoProject/` não existe neste checkout e não é importada pelo backend atual.

## Desenvolvimento local

```sh
cp .env.example .env
go test ./...
go run ./cmd/server
cd web-admin && npm ci && npm run dev
```

Para processamento assíncrono, execute a API e o worker separadamente:

```sh
APP_MODE=api go run ./cmd/api
APP_MODE=worker go run ./cmd/server
```

## Docker Compose

`APP_ENVIRONMENT` controla o serviço `web`:

- `development`: alvo `development` e `npm run dev`.
- qualquer outro valor, por padrão `production`: build Next e `npm run start`.

```sh
APP_ENVIRONMENT=development docker compose -f docker/compose.yml -f docker/compose.development.yml up -d --build
docker compose -f docker/compose.yml up -d --build
```

Em desenvolvimento o Next é acessível em `http://localhost:3000` e reescreve `/api/*` para `api:8080`. A API continua acessível em `http://localhost:8088`. Em produção o serviço `web` executa o servidor Next; a imagem Go também contém uma exportação estática de fallback em `/app/web`.

## Contratos HTTP

Rotas públicas de integração:

- `GET /health`: estado básico do processo.
- `POST/GET /webhook`: aceita JSON, `payload` em query e `application/x-www-form-urlencoded`; só enfileira `review_requested` direcionado a `GITEA_BOT_USERNAME`.
- `POST /review`: URL de PR no formato `http(s)://host/owner/repo/pulls/numero`.

Rotas administrativas usam Basic Auth e respostas legadas para preservar o painel (`[]`/objetos em sucesso e `{error: string}` em falha). As rotas `/api/v1/*` também usam Basic Auth e o envelope `{success,data,error}`.

Rotas versionadas: `GET/POST /api/v1/reviews`, `GET /api/v1/reviews/:id`, `GET /status`, `/steps`, `/result`, `POST /reprocess` e `/cancel`.

O painel usa `/api/admin/*`, incluindo recursos CRUD, setup/catalog, observabilidade, Gitea, review manual e pré-publicação. O mapa completo está em `obsidian/architecture/contracts.md`.

## Fluxo de review

1. Webhook ou ação manual valida a entrada.
2. A API cria `reviews`, registra `enfileirado` e publica um job no Redis Stream.
3. O worker resolve a instância Gitea, busca o diff e divide por limites de policy/configuração.
4. O provider configurado responde JSON; comentários, severidade, linhas, evento e resumo são validados.
5. O resultado é salvo. Reviews manuais aguardam em `pending_reviews` quando a policy não permite publicação automática.
6. Aprovação publica o review no Gitea; rejeição cancela sem publicar; reexecução retorna ao Redis.

O SQLite não persiste diff bruto, prompts montados nem respostas intermediárias por padrão. Quando `enable_detailed_stage_logs` está ativa na policy, persiste os detalhes necessários para diagnóstico das etapas.

O build de produção fica em `production/`. Execute `make production` ou `bash production/build.sh`; o resultado autocontido é gerado em `production/build-result/`.

## Configuração

Providers, conexões, modelos, parâmetros, profiles, prompts, policies, instâncias Gitea e repositórios são tabelas SQLite. API keys/tokens são cifrados com AES-GCM usando `GITEA_TOKEN_ENCRYPTION_KEY` de 32 bytes e nunca são retornados pelo CRUD.

O `config/review-prompts.yaml` permanece como referência operacional; o prompt efetivo do novo serviço é o prompt ativo do profile padrão, com fallback mínimo no código.

## Limitações conhecidas

- O pipeline antigo de planner/consolidator/verifier/formatter foi removido na migração e ainda não foi reimplementado; o serviço atual faz blocos, chamadas, validação e agregação simples.
- Variáveis `REVIEW_*` do pipeline antigo não têm efeito no serviço atual salvo os limites básicos documentados.
- A política por repositório é consultada para limites, publicação manual e rejeição autônoma; as demais colunas antigas ainda não são executadas.
- CORS permanece permissivo (`*`) e Basic Auth deve ser protegido por HTTPS/reverse proxy fora do ambiente local.

Consulte a documentação viva em `obsidian/`, começando por `obsidian/00-index.md`.
