# Desenvolvimento local

## Pré-requisitos

- Docker Engine ou Docker Desktop com Compose v2.
- Arquivo `.env` criado a partir de `.env.example`.
- Credenciais válidas do Gitea e do provider de IA.
- `GITEA_TOKEN_ENCRYPTION_KEY` com exatamente 32 bytes.

## Inicialização

Defina o ambiente do frontend como desenvolvimento e suba os quatro serviços:

```bash
APP_ENVIRONMENT=development docker compose \
  -f docker-compose.yml \
  -f docker-compose.development.yml \
  up -d --build --force-recreate
```

O override de desenvolvimento mantém os serviços `redis`, `api`, `worker` e
`web`. A API e o worker usam o estágio Docker `development`; o frontend usa o
target `development` do `web-admin/Dockerfile`.

Endpoints locais:

- API: `http://localhost:8088`
- Health: `http://localhost:8088/health`
- Frontend: `http://localhost:3000`
- Redis: `localhost:6379`, banco `0`

## Hot Reload

API e worker iniciam o Air com `.air.toml`. Alterações em arquivos Go e nos
arquivos de configuração permitidos recompilam o processo automaticamente.

O Air não observa:

- `web-admin`
- `node_modules`
- `data`
- `logs`
- `.git`
- `vendor`

O frontend possui seu próprio hot reload pelo Next.js no serviço `web`.
O cache `.next` fica em um volume separado (`web_next_cache`) para não ser
gerado no bind mount do Docker Desktop, evitando manifests RSC inconsistentes.
O watcher usa `WATCHPACK_POLLING`, que é a variável reconhecida pelo Next.js.

## Logs E Diagnóstico

```bash
docker compose -f docker-compose.yml -f docker-compose.development.yml logs -f api
docker compose -f docker-compose.yml -f docker-compose.development.yml logs -f worker
docker compose -f docker-compose.yml -f docker-compose.development.yml logs -f web
docker compose -f docker-compose.yml -f docker-compose.development.yml ps
```

Para verificar a fila Redis:

```bash
docker compose -f docker-compose.yml -f docker-compose.development.yml exec redis \
  redis-cli XINFO STREAM gitea:review-jobs
docker compose -f docker-compose.yml -f docker-compose.development.yml exec redis \
  redis-cli XPENDING gitea:review-jobs gitea-reviewers
```

Para investigar uma review, use o endpoint autenticado:

```text
/api/admin/observability/progress?name=<review_id>
```

## Persistência

- `config_data` mantém o SQLite, as conexões de IA, profiles e configurações.
- `redis_data` mantém o stream e o grupo de consumidores.
- `air_tmp` mantém somente os binários temporários gerados pelo Air.

Não use `docker compose down -v` durante o desenvolvimento se quiser preservar
credenciais, banco ou fila. Para parar sem apagar volumes:

```bash
docker compose -f docker-compose.yml -f docker-compose.development.yml down
```

Para reconstruir apenas o backend depois de alterar o Dockerfile ou o Air:

```bash
docker compose -f docker-compose.yml -f docker-compose.development.yml \
  up -d --build --force-recreate api worker
```

## Problemas Comuns

Se o Air tentar criar um caminho inexistente em `/app/tmp`, confirme que o
serviço foi iniciado com `docker-compose.development.yml` e que o volume
`air_tmp` está montado.

Se o Next exibir `Could not find the module ... in the React Client Manifest`,
recrie somente o cache do frontend e o serviço web:

```bash
docker compose -f docker-compose.yml -f docker-compose.development.yml down
docker compose -f docker-compose.yml -f docker-compose.development.yml run --rm --entrypoint sh web -lc 'rm -rf /workspace/.next/*'
APP_ENVIRONMENT=development docker compose -f docker-compose.yml -f docker-compose.development.yml up -d --build --force-recreate web
```

Se o provider responder `401`, revise a API key cadastrada no painel. Se
aparecer `provider credential could not be decrypted`, confirme que API e
worker usam a mesma `GITEA_TOKEN_ENCRYPTION_KEY`; depois recadastre a credencial
se essa chave tiver sido alterada.
