# ForgeReview

ForgeReview recebe solicitações de review do Gitea, enfileira os jobs no Redis e executa a análise com a integração de IA selecionada no SQLite. Ollama e OpenRouter estão funcionais; outros providers cadastrados retornam erro explícito até terem cliente implementado.

## Pipeline de revisão

O worker usa o pipeline v2 de múltiplas etapas:

```text
diff do PR -> planner -> reviewer por grupo -> consolidator -> verifier -> formatter -> Gitea
```

Cada etapa é uma chamada independente ao provider e troca dados internos em JSON. O pipeline não mantém conversa crescente nem busca contexto adicional no Gitea; usa apenas o diff recebido, arquivos alterados, stacks detectadas por `config/review-prompts.yaml`, configurações e respostas intermediárias do job em memória.

O formato publicado permanece compatível com o contrato atual:

```json
{
  "comments": [
    {"file":"app/file.go","line":10,"severity":"alta","decision_reason":"...","comment":"..."}
  ],
  "final_review": {
    "gitea_event":"REQUEST_CHANGES",
    "status":"reprovado",
    "summary":"Resumo objetivo.",
    "observations":""
  },
  "metadata": {"pipeline_version":"2"}
}
```

`metadata` é opcional para consumidores. Prompts, diffs completos e respostas brutas não são persistidos quando `log_sensitive_data` está desabilitado.

Configurações operacionais principais:

```text
REVIEW_PLANNER_MAX_OUTPUT_TOKENS=2000
REVIEW_GROUP_MAX_OUTPUT_TOKENS=3500
REVIEW_CONSOLIDATOR_MAX_OUTPUT_TOKENS=3500
REVIEW_VERIFIER_MAX_OUTPUT_TOKENS=2500
REVIEW_FORMATTER_MAX_OUTPUT_TOKENS=2500
REVIEW_CONTEXT_SAFETY_MARGIN_TOKENS=2048
REVIEW_MIN_PUBLISH_CONFIDENCE=0.75
REVIEW_MAX_PARALLEL_GROUPS=1
REVIEW_MEDIUM_SEVERITY_EVENT=REQUEST_CHANGES
REVIEW_PARTIAL_EVENT=COMMENT
```

Essas opções também existem em `review_policies` para profiles configurados no SQLite. Quando um profile é carregado, a policy do banco prevalece sobre os defaults de ambiente. O fluxo antigo foi removido; o pipeline v2 é o único fluxo de revisão.

O limite de contexto do modelo é tratado separadamente do limite de saída. Antes de cada chamada, o worker estima tokens de entrada de forma conservadora e envia ao provider o menor valor entre o limite configurado da etapa e a saída disponível no contexto. Quando o diff excede o orçamento de contexto, o conteúdo é truncado em limite de linha, marcado no prompt com `TRUNCADO` e a revisão fica parcial, sem aprovação automática.

## Subida rápida

1. Copie `.env.example` para `.env`.
2. Configure `GITEA_URL`, `GITEA_TOKEN`, `GITEA_BOT_USERNAME` e altere `ADMIN_PASSWORD`.
3. Se usar OpenRouter, preencha `OPENROUTER_API_KEY`; para Ollama Cloud, `OLLAMA_API_KEY`. O painel armazena somente o nome da variável, nunca o segredo.
4. Execute:

```sh
docker compose up -d --build
```

5. Aguarde os serviços ficarem saudáveis e abra `http://localhost:8088`.

Credenciais iniciais do painel:

```text
Usuário: admin
Senha:   change-me
```

Elas são controladas por `ADMIN_USERNAME` e `ADMIN_PASSWORD`. Troque a senha antes de expor a porta.

## Configuração inicial do review

Abra **Configurar IA** no painel. O assistente executa um fluxo linear:

1. Escolha Ollama ou OpenRouter.
2. Informe a conexão e valide o acesso.
3. Escolha um modelo retornado pelo catálogo real do provider.
4. Ajuste apenas os parâmetros aplicáveis à integração escolhida.
5. Defina o profile padrão e a policy de revisão.
6. Revise e conclua.

A conclusão usa uma única transação SQLite: conexão, modelo, parâmetros, profile e policy são gravados juntos ou nenhum registro é alterado. Os cadastros individuais continuam disponíveis em **Avançado**.

Os prompts não fazem parte do cadastro administrativo. O worker usa diretamente `config/review-prompts.yaml`, `prompts/base/review_partial.md`, `prompts/base/review_final.md` e os complementos versionados em `prompts/stacks`. Alterações de prompts ficam reservadas para uma evolução futura do produto.

Para Ollama local, o endereço padrão é `http://host.docker.internal:11434` e os modelos vêm de `/api/tags`. Ollama Cloud usa o endpoint compatível `https://ollama.com`, Bearer com a chave da variável configurada e os mesmos endpoints `/api/tags` e `/api/chat`. Para OpenRouter, a API valida `OPENROUTER_API_KEY` em `/api/v1/key` e carrega os modelos de `/api/v1/models`.

## Seleção de provider e modelo

Para cada PR, o worker consulta o SQLite usando `owner/repositório`:

```text
repositório com profile → profile do repositório
sem vínculo específico  → profile padrão
profile → modelo → conexão → provider
```

- `ollama`: usa `/api/chat`, os parâmetros Ollama cadastrados e pode descarregar o modelo local ao terminar. A variante Cloud usa o mesmo contrato com `Authorization: Bearer`; o descarregamento é no-op para não enviar a operação local ao serviço remoto.
- `openrouter`: usa a base oficial `https://openrouter.ai/api/v1`, autenticação Bearer e `/chat/completions`. O modelo é salvo pelo slug oficial retornado pelo catálogo. O limite operacional de saída é configurado separadamente (padrão: `4096`); o máximo anunciado pelo catálogo nunca é enviado automaticamente como `max_tokens`. `HTTP-Referer` e `X-OpenRouter-Title` podem ser configurados para atribuição da aplicação.
- Sem configuração completa e habilitada, o job falha com mensagem clara. Não existe fallback para configurações de review no `.env`.

Alterações administrativas entram em vigor no próximo PR; não é necessário reiniciar o worker. Ao adicionar ou trocar o valor de uma variável secreta no `.env`, recrie o worker para atualizar seu ambiente.

## Docker e SQLite

O volume `config_data` contém `/data/forgereview.db` e é compartilhado pela API e pelo worker. O entrypoint corrige as permissões do volume e depois executa o processo como usuário não-root. Apenas a API aplica migrations e seeds; o worker inicia depois que a API está saudável e apenas valida o schema.

Se uma versão anterior criou o volume com permissões incorretas, basta reconstruir e recriar os containers:

```sh
docker compose down
docker compose up -d --build --force-recreate
```

Não use `docker compose down -v` se quiser preservar as configurações já cadastradas.

## Segurança e privacidade

- O SQLite não possui tabelas para diff, código analisado, prompt final montado ou resposta completa do modelo.
- API keys e tokens são referenciados pelo nome da variável de ambiente.
- `log_sensitive_data` é falso por padrão. Quando falso, o worker não grava diff, prompts ou respostas em disco e não imprime a resposta completa no console.
- Habilitar `log_sensitive_data` cria arquivos sob `/logs/diffs`; faça isso somente durante diagnóstico controlado.
- A autenticação do painel é Basic Auth. Use HTTPS por meio de um proxy reverso em ambientes expostos.

## Endpoints principais

```text
GET  /health
POST /webhook
POST /review
GET  /api/admin/status
POST /api/admin/setup/catalog
POST /api/admin/setup/complete
GET  /api/admin/observability/metrics
GET  /api/admin/observability/logs
GET  /api/admin/observability/reviews
```

As rotas `/api/admin/*` exigem as credenciais administrativas. O painel oferece CRUD para providers, conexões, modelos, parâmetros, profiles, policies, instâncias Gitea e repositórios. A API rejeita alterações administrativas em prompts.

Review manual:

```sh
curl -X POST http://localhost:8088/review \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://gitea.example/owner/repository/pulls/123"}'
```

## Desenvolvimento

Backend:

```sh
go test ./...
go run ./cmd/server
```

Frontend:

```sh
cd web-admin
npm ci
npm run dev
```

Build completo:

```sh
docker compose build
```

Estrutura principal:

```text
cmd/server/             inicialização da API e do worker
internal/admin/         API administrativa
internal/store/         SQLite, migrations e seeds
internal/reviewconfig/  resolução por repositório/profile
internal/ollama/        cliente Ollama
internal/openrouter/    cliente OpenRouter
internal/agents/        execução do review
web-admin/              painel React componentizado
```
