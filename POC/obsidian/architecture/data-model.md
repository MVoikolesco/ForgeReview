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
| `stage_contracts` | contratos versionados de response | referenciado por stage types | catálogo controlado pelo sistema |
| `stage_types` | executor e contratos suportados | 1:N `pipeline_stages` | catálogo controlado pelo sistema |
| `pipeline_definitions` | identidade do workflow | profile opcional; 1:N versões | seed; CRUD dedicado futuro |
| `pipeline_versions` | configuração publicada imutável | N:1 definição | uma versão publicada por definição |
| `pipeline_stages` | instâncias ordenadas de processors | N:1 versão; contratos fixados e modelo opcional | prompt, modos de join/roteamento e configuração controlada |
| `pipeline_transitions` | rotas de negócio e fallback | etapas de uma versão | AST de regra, prioridade e limite de travessias |
| `pipeline_version_triggers` | Entrypoints persistidos | N:1 versão; destino por chave de stage | adapter, posição, ativação e etapa inicial |
| `workflow_processor_catalog` | processors permitidos | catálogo da aplicação | system, LLM, filtro e transformação/merge |
| `workflow_entrypoint_catalog` | tipos de evento permitidos | adapter registrado em código | configuração sem reestruturar o grafo |

## Estado de execução

| Tabela | Campos importantes | Regra |
| --- | --- | --- |
| `reviews` | `id`, owner/repository/PR, pipeline/profile vinculados, status, `job_json`, `result_json`, erro e timestamps | um registro por job; versão do pipeline fica imutável após seleção |
| `review_steps` | review, step, status, mensagem, metadata, duração, erro | append-only operacional |
| `pending_reviews` | review, job e result | existe enquanto aguarda aprovação manual |
| `pipeline_executions` | review, versão, status e snapshot JSON | uma linha por execução/rerun |
| `stage_executions` | etapa, tentativa, status, duração e metadata | auditoria ordenada da execução |
| `stage_artifacts` | tipo e payload JSON validado | output de uma execução de etapa |
| `stage_artifact_inputs` | artifacts consumidos, payload projetado e hash | proveniência N:N de joins/transições | preserva exatamente o lote recebido por cada execução |
| `review_publications` | fingerprint e estado da publicação | reserva local; reconcilia a marca `forgereview` no Gitea antes de repetir envio incerto |
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

O seed configura `preparacao`, `planejamento`, `revisao`, `consolidacao`, `verificacao`, `formatacao` e `publicacao`. O engine resolve cada executor por `stage_types.executor_key`; eventos continuam em `review_steps`, enquanto snapshots, tentativas e artifacts são persistidos nas tabelas próprias do pipeline.

## Resultado JSON

`contracts.Result` mantém `review_id`, `provider`, `model`, `agent`, `summary`, `comments`, `final_review` e metadata. Cada comentário mantém `file`, `line`, `severity`, `decision_reason` e `comment`. `final_review` mantém `gitea_event`, `status`, `summary` e `observations`.

Antes de persistir, o serviço exige arquivo/linha/comentário/razão não vazios, severidade `critica|alta|media|baixa`, resumo final e evento `APPROVE|COMMENT|REQUEST_CHANGES`. Policy sem rejeição autônoma converte `REQUEST_CHANGES` em `COMMENT`.
