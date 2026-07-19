# Modelo de dados e mapeamentos

## Configuração administrativa

| Tabela | Papel | Relações | Exposição |
| --- | --- | --- | --- |
| `ai_providers` | catálogo de provider | 1:N `ai_connections` | CRUD sem secret |
| `ai_connections` | endpoint, auth e atributos | N:1 provider; default único por provider | ciphertext vira `api_key_configured` |
| `ai_models` | modelo e capacidades | N:1 connection; default único por connection | CRUD |
| `model_parameters` | temperature, top_p, Ollama e timeout | 1:1 model | CRUD/setup |
| `review_profiles` | rota lógica de review | aponta para model; default global | CRUD |
| `review_prompts` | prompt versionado/ativo | N:1 profile | CRUD; prompt default é carregado |
| `review_policies` | limites e publicação | 1:1 profile | CRUD; limites básicos executados |
| `gitea_instances` | endpoint, bot e token | 1:N repositories | token nunca é listado |
| `repositories` | escopo e profile por repo | N:1 instância/profile | CRUD e seleção Gitea |

## Estado de execução

| Tabela | Campos importantes | Regra |
| --- | --- | --- |
| `reviews` | `id`, owner/repository/PR, status, source, `job_json`, `result_json`, erro e timestamps | um registro por job; resultado final é JSON |
| `review_steps` | review, step, status, mensagem, metadata, duração, erro | append-only operacional |
| `pending_reviews` | review, job e result | existe enquanto aguarda aprovação manual |
| `schema_migrations` | nome/aplicação | evita reaplicar migrations |

## Estados

- `recebido`: criado no repositório antes de publicação.
- `enfileirado`: publicado no Redis.
- `processando`: worker iniciou.
- `aguardando_autorizacao`: resultado pronto, aguardando aprovação.
- `concluido`: resultado salvo e publicado, ou fluxo sem publicação configurada.
- `falhou`: erro terminal com `error_message`.
- `cancelado`: cancelado pelo operador e não deve publicar.

## Steps e UI

| Step backend | Stage legado consumido pelo painel | Percentual aproximado |
| --- | --- | --- |
| `enfileirado` | `preparacao` | 5 |
| `buscando_diff` | `preparacao` | 20 |
| `enviando_para_ia` | `revisao` | 45 |
| `recebendo_resposta_parcial` | `revisao` | 60 |
| `agregando_resultado` | `consolidacao` | 80 |
| `pre-publicacao` | `pre-publicacao` | 90 / waiting |
| `publicando_comentario` | `publicacao` | 95 |
| `concluido` | `publicacao` | 100 / done |

O pipeline executa `planejamento`, revisão por grupos, `consolidacao`, `verificacao` e `formatacao`. Os eventos de cada etapa, incluindo grupo, arquivos, tentativas e falhas, são persistidos em `review_steps.metadata_json` e projetados no painel.

## Resultado JSON

`contracts.Result` mantém `review_id`, `provider`, `model`, `agent`, `summary`, `comments`, `final_review` e metadata. Cada comentário mantém `file`, `line`, `severity`, `decision_reason` e `comment`. `final_review` mantém `gitea_event`, `status`, `summary` e `observations`.

Antes de persistir, o serviço exige arquivo/linha/comentário/razão não vazios, severidade `critica|alta|media|baixa`, resumo final e evento `APPROVE|COMMENT|REQUEST_CHANGES`. Policy sem rejeição autônoma converte `REQUEST_CHANGES` em `COMMENT`.
