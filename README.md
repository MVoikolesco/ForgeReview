# Gitea AI Reviewer

Projeto em Go para receber webhooks do Gitea e enfileirar jobs de revisao.

Nesta fase a API valida o webhook, cria um `ReviewJob`, publica no Redis Streams e responde rapido. Um worker separado consome a fila, busca o diff completo do Pull Request no Gitea, separa o diff por arquivo, divide em blocos menores e envia sequencialmente para um Ollama remoto/local.

O worker gera reviews parciais por bloco, consolida um review final, grava logs fisicos com diff/prompt/respostas e publica o review no PR via API do Gitea com comentarios inline e evento final (`APPROVED`, `COMMENT` ou `REQUEST_CHANGES`).

## Estrutura

```text
.
+-- cmd/
|   +-- server/
|       +-- main.go
+-- internal/
|   +-- agents/
|       +-- agent.go
|       +-- registry.go
|       +-- reviewer.go
|       +-- reviewer_test.go
|   +-- config/
|       +-- config.go
|   +-- diff/
|       +-- parser.go
|       +-- parser_test.go
|   +-- gitea/
|       +-- client.go
|       +-- client_test.go
|   +-- ollama/
|       +-- client.go
|       +-- client_test.go
|   +-- queue/
|       +-- queue.go
|       +-- redis/
|           +-- redis_queue.go
|   +-- review/
|       +-- blocker.go
|       +-- blocker_test.go
|       +-- prompts.go
|       +-- promptconfig/
|   +-- webhook/
|       +-- handler.go
|       +-- health.go
|       +-- handler_test.go
|   +-- worker/
|       +-- worker.go
|       +-- worker_test.go
+-- config/
|   +-- review-prompts.yaml
+-- Dockerfile
+-- docker-compose.yml
+-- go.mod
+-- go.sum
+-- Makefile
+-- prompts/
|   +-- base/
|   +-- stacks/
+-- README.md
```

## Configuracao

```sh
cp .env.example .env
```

Variaveis principais:

```env
APP_MODE=api
PORT=8080
SERVICE_NAME=gitea-ai-reviewer
VERSION=0.1.0
LOG_LEVEL=debug

GITEA_BOT_USERNAME=ia-reviewer
GITEA_URL=http://10.1.1.25:3000
GITEA_TOKEN=token_do_usuario_ia_reviewer

REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_STREAM=gitea:review-jobs
REDIS_GROUP=gitea-reviewers
REDIS_CONSUMER=worker-1

DIFF_LOG_DIR=/logs/diffs
REVIEW_PROMPT_CONFIG_PATH=./config/review-prompts.yaml

OLLAMA_URL=http://192.168.0.50:11434
OLLAMA_MODEL=deepseek-coder:6.7b
OLLAMA_TEMPERATURE=0.1
OLLAMA_TOP_P=0.85
OLLAMA_REPEAT_PENALTY=1.1
OLLAMA_NUM_CTX=4096
OLLAMA_NUM_THREADS=2
OLLAMA_NUM_PREDICT=400
OLLAMA_KEEP_ALIVE=5m
OLLAMA_TIMEOUT_SECONDS=900

REVIEW_MAX_BLOCK_CHARS=4000
REVIEW_MAX_FILES_PER_BLOCK=2
REVIEW_CONCURRENCY=1
```

Para o Ollama remoto informado nos testes locais, use:

```env
OLLAMA_URL=http://10.1.10.77:11434
```

## Rodando com Docker

```sh
docker compose up --build
```

Servicos:

- `redis`: Redis 7 com AOF habilitado.
- `api`: exposta em `http://localhost:8088`, publica jobs no Redis.
- `worker`: consome jobs do Redis, busca o diff no Gitea, chama o Ollama e salva logs fisicos em `./logs/diffs`.

## Ollama remoto

O worker usa `OLLAMA_URL` para chamar o endpoint:

```text
POST {OLLAMA_URL}/api/chat
```

Teste de conectividade com PowerShell:

```powershell
Invoke-RestMethod -Method Post `
  -Uri "http://10.1.10.77:11434/api/chat" `
  -ContentType "application/json" `
  -Body '{"model":"deepseek-coder:6.7b","stream":false,"messages":[{"role":"user","content":"Responda apenas OK"}]}'
```

O diff nao e enviado inteiro em uma unica chamada ao modelo. O worker divide por arquivos/blocos usando `REVIEW_MAX_BLOCK_CHARS` e `REVIEW_MAX_FILES_PER_BLOCK`, revisa bloco a bloco e depois faz uma chamada final de consolidacao. Se um bloco falhar por timeout ou erro do Ollama, o worker registra a falha, continua nos proximos blocos e gera review final quando pelo menos um bloco foi revisado com sucesso.

## Prompts de review

Os prompts sao carregados do filesystem a cada review. Alterar arquivos `.md` em `prompts/` ou o YAML em `config/review-prompts.yaml` nao exige rebuild; a proxima execucao do worker ja usa a configuracao nova.

Estrutura principal:

```text
prompts/
  base/
    review_partial.md
    review_final.md
  stacks/
    go.md
    laravel.md
    react.md
    solid.md
    web.md      # HTML/CSS/JS padrao
    sql.md
    docker.md
    ci.md      # CodeIgniter 4
    cicd.md    # pipelines/workflows
    laravel/
      models.md
      services.md
      controllers.md
      views.md
      dtos.md
      entities.md

config/
  review-prompts.yaml
```

O YAML mapeia projetos por `owner` e `repo`, define stacks permitidas e controla `include_docs`. Quando um projeto nao esta mapeado, o bot usa `default`.

Exemplo Go:

```yaml
projects:
  teste-bot:
    owner: mvoikolesco
    repo: teste-bot
    stacks:
      - go
      - cicd
      - docker
    include_docs: false
```

Exemplo Laravel + React:

```yaml
projects:
  portal:
    owner: Qualyagro
    repo: portal
    stacks:
      - laravel
      - react
      - sql
      - docker
      - cicd
    include_docs: false
```

Para cada bloco, o resolver carrega sempre o prompt base parcial e acrescenta apenas as stacks que casam com os arquivos do bloco. Alem disso, cada stack pode ter categorias por pasta/tipo de arquivo. Por exemplo, `app/Services/OrderService.php` carrega `prompts/stacks/laravel.md` e `prompts/stacks/laravel/services.md`; `app/Models/Order.php` carrega `laravel.md` e `laravel/models.md`.

Use `ci` para projetos CodeIgniter 4, `solid` para SolidJS e `web` para HTML/CSS/JS sem framework. Para pipelines de build/deploy, use a stack `cicd` apenas se arquivos YAML voltarem a ser revisaveis.

Quando `include_docs: false`, docs como `README.md`, `CHANGELOG.md`, `docs/**` e `*.md` ficam fora dos blocos de review. Arquivos `.yaml` e `.yml` tambem sao ignorados para evitar comentarios fracos em configuracoes extensas. Arquivos como `.env.example`, `.gitignore` e Dockerfile continuam revisaveis para riscos reais.

## Endpoints

### `GET /health`

```sh
curl http://localhost:8088/health
```

### `POST /webhook`

Recebe o payload do Gitea. O payload pode chegar como JSON no body, como form `payload=...` ou como query string `?payload=...`.

Quando o evento for valido e o reviewer for o usuario configurado em `GITEA_BOT_USERNAME`, a API publica um job no Redis e responde:

```json
{
  "status": "accepted"
}
```

Eventos ignorados retornam:

```json
{
  "status": "ignored"
}
```

## Fila

Redis Streams:

- Stream: `gitea:review-jobs`
- Consumer group: `gitea-reviewers`
- Consumer: configuravel por `REDIS_CONSUMER`, padrao `worker-1`

Publicacao:

```text
XADD gitea:review-jobs * owner repo pr_number requested_reviewer sender
```

Consumo:

```text
XREADGROUP GROUP gitea-reviewers worker-1 COUNT 1 BLOCK ... STREAMS gitea:review-jobs >
```

Apos processar com sucesso:

```text
XACK gitea:review-jobs gitea-reviewers <message-id>
```

Se o worker falhar ao buscar o diff no Gitea, o erro e retornado antes do `XACK`.

## Gitea

O worker busca o diff com:

```text
GET {GITEA_URL}/api/v1/repos/{owner}/{repo}/pulls/{prNumber}.diff
```

Headers:

```text
Authorization: token {GITEA_TOKEN}
Accept: text/plain
```

Status HTTP fora de 2xx vira erro. O token nao e escrito em logs.

## Diff

O worker busca o diff completo do PR em uma unica chamada ao Gitea e depois faz o parsing local por arquivo.

Cada arquivo alterado e representado com:

```go
type ChangedFile struct {
    Path      string
    Additions int
    Deletions int
    Patch     string
}
```

Arquivos gerados ou pouco uteis para review sao ignorados na etapa de blocos, como lockfiles, `.yaml`, `.yml`, `vendor/`, `node_modules/`, `dist/`, `build/`, `.map` e `.min.js`.

## Agentes

O worker nao conhece detalhes da revisao. Ele apenas recebe um `ReviewJob` e chama um agente:

```go
type Agent interface {
    Name() string
    Process(ctx context.Context, job ReviewJob) error
}
```

Hoje existe apenas:

```text
ReviewerAgent
```

Ele busca o diff no Gitea, separa o conteudo por arquivo, cria blocos de review, chama o Ollama e consolida as respostas. A ideia e permitir novos agentes sem mudar o worker:

```text
ReviewerAgent
SecurityAgent
DocumentationAgent
ArchitectureAgent
ChangelogAgent
```

## Logs

API:

```text
Webhook valido recebido
Reviewer: ia-reviewer
Solicitado por: marcio
Repositorio: Qualyagro/wiki
PR: #12
Acao: review_requested
Job criado: {Owner:Qualyagro Repo:wiki PRNumber:12 RequestedReviewer:ia-reviewer Sender:marcio}
```

Worker:

```text
Job recebido: {Owner:Qualyagro Repo:wiki PRNumber:12 RequestedReviewer:ia-reviewer Sender:marcio}
Agent selecionado: reviewer
diff obtido owner=Qualyagro repo=wiki pr=12 size=12345
logs fisicos do review dir=/logs/diffs/Qualyagro_wiki_pr-12
diff parseado owner=Qualyagro repo=wiki pr=12 files=2
arquivo alterado path=README.md additions=2 deletions=1 patch_size=321
prompt config carregado path=./config/review-prompts.yaml
docs ignorados owner=Qualyagro repo=wiki pr=12 files=1 include_docs=false
arquivos revisaveis owner=Qualyagro repo=wiki pr=12 files=2
blocos de review gerados owner=Qualyagro repo=wiki pr=12 blocks=2 max_chars=4000 max_files_per_block=2
prompt parcial resolvido owner=Qualyagro repo=wiki stacks=go categories=go/handlers files=2 prompt_chars=4200
enviando bloco para ollama block=1 total=2 files=2 chars=3900 model=deepseek-coder:6.7b timeout=900s
resposta ollama recebida block=1 chars=900 duration=1m12s total_elapsed=1m12s
erro ao revisar bloco com ollama block=2 total=2 files=1 chars=4100 prompt_chars=6200 timeout=900s duration=15m0s total_elapsed=16m12s err=context deadline exceeded
gerando review final partial_reviews=2 failed_blocks=1
prompt final resolvido owner=Qualyagro repo=wiki prompt_chars=2100
review final gerado chars=2345 duration=48s total_elapsed=17m0s
```

Dentro da pasta fisica do PR, o worker grava arquivos `.log` como:

```text
00-processo.log
01-diff-completo.log
02-prompt-base.log
block-001-prompt.log
block-001-resposta.log
final-prompt.log
final-resposta.log
```

Arquivos de resposta e erro incluem metadados de tempo no topo:

```text
===== METADADOS RESPOSTA OLLAMA =====
tipo=bloco
block=1
total=2
files=README.md,internal/app.go
response_chars=900
response_duration=1m12s
total_desde_inicio_ollama=1m12s
===== RESPOSTA =====
...
```

O `01-diff-completo.log` fica separado por blocos de arquivo:

```text
===== INICIO DIFF ARQUIVO 1/2 path=README.md additions=2 deletions=1 =====
diff --git a/README.md b/README.md
...
===== FIM DIFF ARQUIVO 1/2 path=README.md =====

===== INICIO DIFF ARQUIVO 2/2 path=internal/app.go additions=1 deletions=1 =====
diff --git a/internal/app.go b/internal/app.go
...
===== FIM DIFF ARQUIVO 2/2 path=internal/app.go =====
```

O stdout registra andamento e metadados. Prompt completo, diff e respostas ficam nos arquivos fisicos para facilitar os testes.

Com Docker Compose, esses arquivos ficam no host em:

```text
logs/diffs/
```

Para acompanhar:

```sh
docker logs -f gitea-ai-reviewer-api
docker logs -f gitea-ai-reviewer-worker
```
