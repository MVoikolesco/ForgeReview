# Contratos HTTP

## Integração

- `GET /health` retorna `status`, `service`, `version` e `uptime_seconds`.
- `POST/GET /webhook` aceita JSON, query `payload` e form `payload`; só enfileira `action=review_requested` para `GITEA_BOT_USERNAME`. Outros eventos retornam `{status:"ignored"}`.
- `POST /review` recebe `{url}` e aceita apenas URL HTTP(S) com quatro segmentos `/owner/repo/pulls/numero`.

## Administração legada

`/api/admin/*` exige `Authorization: Basic ...`. O painel espera listas JSON diretas, objetos diretos e `{error: string}` em falhas. Secrets enviados em `api_key`/`token` são removidos do retorno; o retorno usa `api_key_configured` quando aplicável.

Recursos CRUD: `ai/providers`, `ai/connections`, `ai/models`, `ai/model-parameters`, `review/profiles`, `review/prompts`, `review/policies`, `gitea/instances` e `repositories`.

Rotas especiais consumidas pelo frontend:

- `GET /status`, `GET /observability/metrics`, `/logs`, `/reviews`, `/progress`.
- `GET /observability/stage-logs?name=...&stage=...` retorna tentativas persistidas, artefatos e metadados detalhados de uma etapa.
- `POST /setup/catalog`, `/setup/complete`, `/setup/add-model`.
- `POST /reviews/manual`.
- `GET /review/settings` agrega profiles, policies efetivas, prompt ativo,
  pipelines publicados, stages ordenados e catálogo de tipos/contratos para a
  tela administrativa somente leitura.
- `GET /review/workflow-catalog` retorna contratos controlados, processors,
  adapters de Entrypoint e modos de roteamento/junção, indicando quais recursos
  já são executáveis pelo runtime.
- Drafts de pipeline persistem `route_mode`/`join_mode` em stages e `rule`,
  `max_traversals` e prioridade em transitions. Regras usam apenas operadores
  controlados e campos existentes no JSON Schema do contrato de saída.
- `GET /reviews/pending?name=...` e `POST /reviews/pending/{approve|reject|rerun}?name=...`.
- `POST /gitea/instances/test`, `/:id/test`, `/:id/organizations`, `/:id/repositories`, `/:id/pull-requests`.

## API versionada

Todas as rotas `/api/v1/*` exigem Basic Auth e usam `{success,data,error}`. Rotas: list/create reviews, get review/status/steps/result, reprocess e cancel. Resultado inexistente responde `RESULT_NOT_READY`; review ausente responde `NOT_FOUND`.

## Resultado e steps

- `comments[]` preserva `file`, `line`, `severity`, `decision_reason` e `comment`.
- `final_review` preserva `gitea_event`, `status`, `summary` e `observations`.
- O backend recalcula `gitea_event`/`status` a partir dos achados válidos e normaliza `summary`/`observations` para não publicar texto contraditório com a decisão final.
- O parser das respostas de stage aceita fences Markdown e tenta reparar JSON truncado por fechamento ausente antes de falhar o contrato da etapa.
- Achados de review aceitam `end_line` opcional no contrato interno, alinhado aos prompts técnicos que podem retornar intervalos de linhas; isso evita rejeitar a consolidação por campo desconhecido.
- `GET /observability/progress` já retorna todos os eventos persistidos em `review_steps`; o flow usa esses eventos no modal de diagnóstico por etapa, sem criar uma segunda fonte de logs no Redis.
- Logs detalhados de prompt/resposta só são persistidos quando `review_policies.enable_detailed_stage_logs=1` para o profile em uso.
- Edições do perfil/policy usam `PATCH` parcial; o painel não reenvia valores inalterados.
- Steps internos são traduzidos para stages legados do flow; [[data-model|mapeamento completo]].
- Estados de pré-publicação são `aguardando_autorizacao` no banco e `waiting` no painel.
- Quando a review já está `concluido`, a observabilidade adiciona um evento terminal sintético de `publicacao` para o painel não permanecer preso no último `review_step` intermediário.
- O frontend também trata `pre-publicacao` como concluída quando já existe evento em `publicacao`, para o card histórico não permanecer em 90% após a autorização manual.
