# Runbook operacional

## Subir

1. Copiar `.env.example` para `.env` e trocar credenciais, URL/token Gitea e chave AES de 32 bytes.
2. Escolher `APP_ENVIRONMENT=development` para `npm run dev` ou `production` para build/`next start`.
3. Executar `docker compose up -d --build`.
4. Verificar `docker compose ps`, `curl http://localhost:8088/health` e `http://localhost:3000`.

## Ordem de inicialização

- Redis fica saudável primeiro.
- API aplica migrations e seed; só então fica saudável.
- Worker depende da API e exige schema pronto.
- Web depende da API e usa `http://api:8080` para rewrites internos.

## Diagnóstico

- API: `docker compose logs --tail=100 api`.
- Worker: `docker compose logs --tail=100 worker`.
- Next: `docker compose logs --tail=100 web`.
- Redis e jobs: `/api/admin/observability/metrics` autenticado.
- Fluxo e steps: `/api/admin/observability/progress?name=<review_id>` autenticado.

## Persistência e segurança

- `config_data` contém SQLite e é compartilhado por API/worker.
- `redis_data` contém o stream persistente.
- Não montar `.env` dentro das imagens.
- Não retornar nem logar `api_key_ciphertext`, `token_ciphertext` ou valores de API key.
- Antes de expor, colocar TLS, restringir CORS e substituir Basic Auth por uma camada de identidade apropriada.

## Falhas conhecidas

- Se a API falhar em migration, não iniciar o worker; corrigir schema e reiniciar API.
- Se um provider falhar, o job fica `falhou`, o worker continua e a métrica `failed` incrementa.
- Se o resultado manual ficar `aguardando_autorizacao`, usar o modal de pré-publicação; approve publica, reject cancela e rerun enfileira novamente.
- Se o Next não subir, verificar se `APP_ENVIRONMENT` é exatamente `development` ou `production` e se o alvo correspondente existe no `web-admin/Dockerfile`.
