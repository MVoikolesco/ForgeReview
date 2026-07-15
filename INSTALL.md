# Instalacao e Execucao

Este guia mostra como configurar e executar o Gitea AI Reviewer em ambiente local ou servidor usando Docker Compose.

## Requisitos

- Docker e Docker Compose.
- Um usuario/bot no Gitea com token de API.
- Um servidor Ollama acessivel pela API HTTP.
- Um modelo disponivel no Ollama, por exemplo `deepseek-coder:6.7b` ou outro modelo configurado em `.env`.

## Configuracao do .env

Copie o arquivo de exemplo:

```sh
cp .env.example .env
```

Edite o `.env`:

```env
GITEA_BOT_USERNAME=ia-reviewer
GITEA_URL=http://seu-gitea:3000
GITEA_TOKEN=token_do_usuario_bot

OLLAMA_URL=http://seu-ollama:11434
OLLAMA_MODEL=deepseek-coder:6.7b

REVIEW_PROMPT_CONFIG_PATH=./config/review-prompts.yaml
REVIEW_MAX_BLOCK_CHARS=4000
REVIEW_MAX_FILES_PER_BLOCK=2
REVIEW_CONCURRENCY=1
OLLAMA_TIMEOUT_SECONDS=900
REVIEW_PUBLISH_MANUAL_REVIEWS=false
REVIEW_ALLOW_AUTONOMOUS_REJECTION=false
```

### Variaveis do .env

| Variavel | Uso |
| --- | --- |
| `APP_MODE` | Define o modo do processo: `api` ou `worker`. No Docker Compose isso ja e configurado por servico. |
| `PORT` | Porta interna da API. No Compose ela e exposta como `8088:8080`. |
| `SERVICE_NAME` | Nome logico do servico. |
| `VERSION` | Versao exibida/configurada para o servico. |
| `LOG_LEVEL` | Nivel de log. Use `debug` durante implantacao inicial. |
| `GITEA_BOT_USERNAME` | Usuario do Gitea que deve receber o pedido de review no Pull Request. |
| `GITEA_URL` | URL base do Gitea. |
| `GITEA_TOKEN` | Token do usuario/bot que busca diff e publica review. Nao compartilhe esse valor. |
| `REDIS_ADDR` | Endereco do Redis usado pela fila. No Compose use `redis:6379`. |
| `REDIS_PASSWORD` | Senha do Redis, se existir. |
| `REDIS_DB` | Banco Redis. Padrao `0`. |
| `REDIS_STREAM` | Nome do stream de jobs. |
| `REDIS_GROUP` | Consumer group do worker. |
| `REDIS_CONSUMER` | Nome do consumidor. Use um valor unico se rodar mais de um worker no futuro. |
| `DIFF_LOG_DIR` | Pasta onde o worker grava diff, prompts e respostas. |
| `REVIEW_PROMPT_CONFIG_PATH` | Caminho do YAML de prompts e stacks. |
| `OLLAMA_URL` | URL base do Ollama. |
| `OLLAMA_MODEL` | Modelo usado para revisar os blocos. |
| `OLLAMA_TEMPERATURE` | Temperatura do modelo. Valores baixos reduzem variacao. |
| `OLLAMA_TOP_P` | Amostragem nucleus/top-p do modelo. |
| `OLLAMA_REPEAT_PENALTY` | Penalidade de repeticao. |
| `OLLAMA_NUM_CTX` | Janela de contexto solicitada ao modelo. |
| `OLLAMA_NUM_THREADS` | Threads usadas pelo modelo quando suportado. |
| `OLLAMA_NUM_PREDICT` | Limite de tokens gerados por chamada. |
| `OLLAMA_KEEP_ALIVE` | Tempo para manter o modelo carregado. |
| `OLLAMA_TIMEOUT_SECONDS` | Timeout maximo de cada chamada ao Ollama. |
| `REVIEW_MAX_BLOCK_CHARS` | Tamanho maximo aproximado de cada bloco de diff enviado ao modelo. |
| `REVIEW_MAX_FILES_PER_BLOCK` | Quantidade maxima de arquivos por bloco. |
| `REVIEW_CONCURRENCY` | Mantenha `1`. Campo reservado para paralelismo de agentes/reviews, ainda em desenvolvimento. |
| `REVIEW_PUBLISH_MANUAL_REVIEWS` | Publica no Gitea os reviews solicitados pelo endpoint manual `/review`. Padrao `false`; o fluxo de webhook continua publicando normalmente. |
| `REVIEW_ALLOW_AUTONOMOUS_REJECTION` | Permite que o bot publique `APPROVED` ou `REQUEST_CHANGES` quando o review indicar isso. Padrao `false`; nesse caso o resultado e publicado como `COMMENT`, preservando o status, o corpo e os comentarios inline. |

### Sobre REVIEW_CONCURRENCY

Nao altere `REVIEW_CONCURRENCY` por enquanto.

A intencao desse campo e permitir, no futuro, mais de um agente/review em paralelo, revisando mais de um PR por vez. Isso exige capacidade de modelo/infraestrutura adequada e, em cenarios com provedor pago, plano Pro ou equivalente. A funcionalidade ainda esta em desenvolvimento; hoje o valor seguro e suportado e:

```env
REVIEW_CONCURRENCY=1
```

Aumentar esse valor antes do suporte estar pronto pode gerar concorrencia inesperada, mais chamadas ao modelo e comportamento dificil de auditar.

## Como Rodar

Com o `.env` configurado, suba a stack:

```sh
docker compose up --build
```

Servicos:

- `redis`: Redis 7 com AOF habilitado.
- `api`: exposta em `http://localhost:8088`.
- `worker`: consome a fila, chama o Gitea, chama o Ollama e publica o review.

Logs fisicos ficam em:

```text
logs/diffs/
```

## Teste de Saude

```sh
curl http://localhost:8088/health
```

Resposta esperada:

```json
{
  "status": "ok"
}
```

## Configurando o Webhook no Gitea

Crie um webhook no repositorio ou organizacao apontando para:

```text
http://seu-host:8088/webhook
```

O endpoint aceita payload JSON no body, `payload=...` em form ou `payload=...` na query string.

O evento relevante e o pedido de review para o usuario definido em `GITEA_BOT_USERNAME`. Quando o evento e aceito, a API publica um job no Redis e retorna:

```json
{
  "status": "accepted"
}
```

Eventos fora do escopo retornam:

```json
{
  "status": "ignored"
}
```

## Testando o Ollama

PowerShell:

```powershell
Invoke-RestMethod -Method Post `
  -Uri "http://seu-ollama:11434/api/chat" `
  -ContentType "application/json" `
  -Body '{"model":"deepseek-coder:6.7b","stream":false,"messages":[{"role":"user","content":"Responda apenas OK"}]}'
```

## Prompts e Stacks

Os prompts sao recarregados a cada review. Alterar arquivos em `prompts/` ou `config/review-prompts.yaml` nao exige rebuild quando o volume do Compose esta ativo:

```yaml
stacks:
  go:
    prompt: prompts/stacks/go.md
    file_patterns:
      - "*.go"
      - "go.mod"
```

Para adicionar uma stack:

1. Crie `prompts/stacks/minha-stack.md`.
2. Cadastre a stack em `config/review-prompts.yaml`.
3. Defina `file_patterns` que identifiquem arquivos dessa stack.

Patterns aceitam `*` e `**`, por exemplo:

```yaml
file_patterns:
  - "app/**/*.php"
  - "src/**/*.tsx"
  - ".github/workflows/**/*.yml"
```

### Contrato dos Prompts

Nao modifique a estrutura de entrada e resposta base do review sem alterar o processamento em Go.

O sistema depende de um contrato rigido para parsear a resposta do modelo e publicar comentarios no Gitea. Em especial, preserve o formato das secoes:

- `STATUS`
- `ACHADOS_CONCRETOS`
- `CONTRATOS_DECLARADOS`
- `REFERENCIAS_A_VERIFICAR`
- `REGRAS_DE_VALIDACAO`
- `COMENTARIOS_INLINE`
- `REVISAO_FINAL`
- `EVENTO_GITEA`

Voce pode ajustar regras de revisao dentro dos `.md`, adicionar stacks e alterar `file_patterns`, mas nao deve instruir o modelo a responder em outro formato.

## Rodando Testes

Com Go instalado:

```sh
go test ./...
```

Com Docker:

```sh
docker run --rm -v "${PWD}:/app" -w /app golang:1.22-alpine go test ./...
```

## Build Manual

```sh
go build -o bin/gitea-agents ./cmd/server
```

Ou usando o Makefile:

```sh
make build
make test
make docker
```
