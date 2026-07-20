# Runbook operacional

## Subir

1. Copiar `.env.example` para `.env` e trocar credenciais, URL/token Gitea e chave AES de 32 bytes.
2. Escolher `APP_ENVIRONMENT=development` para hot reload ou `production` para build/`next start`.
3. Em desenvolvimento, executar `docker compose -f docker/compose.yml -f docker/compose.development.yml up -d --build`; o Go usa Air e o Next continua no serviço `web`.
4. Em produção, executar `bash production/build.sh` e depois, no artefato, `docker compose -f production/build-result/compose.yaml up -d --build --force-recreate`.
5. Verificar `docker compose -f production/build-result/compose.yaml ps`, `curl http://localhost:8088/health` e `http://localhost:3000`.

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
- Quando a etapa `revisao` falhar em todos os grupos, o erro da review identifica cada grupo e a causa retornada pelo provider. Consultar também os steps persistidos para distinguir erro de conexão de contrato JSON inválido.
- Redis local fica no serviço `redis`, porta `6379` no host, banco `0`, stream `gitea:review-jobs` e grupo `gitea-reviewers`; a fila contém jobs, não a configuração do provider.
- Se aparecer `provider credential could not be decrypted`, a `GITEA_TOKEN_ENCRYPTION_KEY` do API e do worker não corresponde à usada para salvar a credencial, ou não possui 32 bytes; recadastrar a credencial após corrigir a variável.

## Persistência e segurança

- `config_data` contém SQLite e é compartilhado por API/worker.
- `redis_data` contém o stream persistente.
- Não montar `.env` dentro das imagens.
- Não retornar nem logar `api_key_ciphertext`, `token_ciphertext` ou valores de API key.
- Antes de expor, colocar TLS, restringir CORS e substituir Basic Auth por uma camada de identidade apropriada.

## Falhas conhecidas

- Se a API falhar em migration, não iniciar o worker; corrigir schema e reiniciar API.
- Se um provider falhar, a etapa de revisão retenta até o `RetryLimit`; se todos os grupos falharem, o job fica `falhou`, o worker continua e a métrica `failed` incrementa.
- Se o resultado manual ficar `aguardando_autorizacao`, usar o modal de pré-publicação; approve publica, reject cancela e rerun enfileira novamente.
- Se o Next não subir, verificar se `APP_ENVIRONMENT` é exatamente `development` ou `production` e se o alvo correspondente existe no `web-admin/Dockerfile`.
- Se a API repetir `exec /usr/local/bin/review-bot: no such file or directory`, o servidor está usando um artefato/container antigo. Copiar novamente todo `production/build-result`, executar `docker compose -f compose.yaml up -d --build --force-recreate` e confirmar que o Compose usa `command: ["/app/server"]`; não reutilizar containers antigos.
- O aviso `Memory overcommit must be enabled` do Redis é do kernel do host. Em Linux, habilitar com `sudo sysctl -w vm.overcommit_memory=1` e persistir `vm.overcommit_memory = 1` em `/etc/sysctl.conf`; isso é independente da falha de inicialização da API.
- O Air observa extensões Go e arquivos de configuração permitidos, mas `web-admin`, `node_modules`, `data` e `logs` ficam excluídos do watch do backend.
- O override de desenvolvimento mantém `redis`, `api`, `worker` e `web`; `air_tmp` guarda somente os binários temporários do Air fora do bind mount do código.
