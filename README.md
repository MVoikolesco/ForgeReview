# Gitea AI Reviewer

Orquestrador em Go para receber webhooks do Gitea, enfileirar pedidos de revisao e publicar comentarios automatizados em Pull Requests usando um modelo Ollama.

O projeto foi criado para manter o fluxo de review previsivel: a API responde rapido ao webhook, o worker processa o diff em blocos, o modelo gera reviews parciais, uma etapa final consolida o resultado e o Gitea recebe comentarios inline com evento final (`APPROVED`, `COMMENT` ou `REQUEST_CHANGES`).

## Recursos

- Recebe eventos de review do Gitea via webhook.
- Enfileira jobs em Redis Streams para processamento assincrono.
- Busca o diff completo do Pull Request pela API do Gitea.
- Divide o diff por arquivos e blocos configuraveis.
- Detecta stacks por `file_patterns`, sem cadastro por projeto.
- Carrega prompts Markdown em tempo de execucao, sem rebuild.
- Mantem formato rigido de resposta para gerar comentarios inline com seguranca.
- Registra logs fisicos com diff, prompts e respostas do modelo.

## Como Funciona

```text
Gitea webhook
  -> API /webhook
  -> Redis Streams
  -> Worker
  -> Gitea diff
  -> blocos de review
  -> Ollama
  -> consolidacao final
  -> Gitea Pull Request Review
```

O worker nao envia o diff inteiro em uma unica chamada. Ele separa o diff por arquivo, monta blocos menores e chama o modelo sequencialmente. Depois, uma chamada final consolida os reviews parciais e produz o corpo do review e os comentarios inline.

## Avisos Importantes

- Nao altere a estrutura do prompt de entrada nem o formato de resposta base sem alterar tambem o parser e os testes. O processamento depende desse contrato para transformar a resposta do modelo em comentarios no Gitea.
- Os arquivos Markdown em `prompts/base/` podem ter regras de revisao ajustadas, mas nao devem instruir o modelo a responder em outro formato.
- O formato final esperado pelo sistema inclui secoes como `STATUS`, `ACHADOS_CONCRETOS`, `COMENTARIOS_INLINE`, `REVISAO_FINAL` e `EVENTO_GITEA`.
- Mantenha `REVIEW_CONCURRENCY=1`. Essa configuracao esta reservada para paralelismo de agentes/reviews, revisando mais de um PR por vez, e ainda esta em desenvolvimento. No futuro, so deve ser usada com capacidade adequada de modelo/infraestrutura e, em provedor pago, plano Pro ou equivalente. Nao altere por enquanto.

## Prompts

Os prompts ficam em `prompts/` e a configuracao principal fica em [config/review-prompts.yaml](config/review-prompts.yaml).

```text
prompts/
  base/
    review_partial.md
    review_final.md
  stacks/
    angular.md
    codeigniter.md
    go.md
    laravel.md
    php.md
    react.md
    solidjs.md
    vue.md
    sql.md
    docker.md
    cicd.md
```

O YAML define filtros globais e stacks autodetectadas. Cada stack aponta para um `.md` e uma lista de `file_patterns`; quando um bloco contem arquivos que casam com esses patterns, o prompt da stack e acrescentado ao prompt base.

O contrato de resposta do modelo fica preservado no codigo. Isso e intencional: o parser depende das secoes `STATUS`, `ACHADOS_CONCRETOS`, `COMENTARIOS_INLINE`, `REVISAO_FINAL` e `EVENTO_GITEA` para publicar corretamente no Gitea.

## Estrutura

```text
cmd/server/              entrada da API e worker
config/                  configuracao dos prompts
internal/agents/         agentes de processamento
internal/diff/           parser de diff
internal/gitea/          cliente da API do Gitea
internal/ollama/         cliente do Ollama
internal/queue/          contratos de fila
internal/queue/redis/    Redis Streams
internal/review/         blocos, prompts e parser de resposta final
internal/webhook/        handlers HTTP
internal/worker/         consumidor de jobs
prompts/                 prompts base e por stack
```

## Instalacao

Veja o passo a passo em [INSTALL.md](INSTALL.md).

## Como Rodar

Resumo rapido para ambiente com Docker:

```sh
cp .env.example .env
docker compose up --build
```

Antes de subir, edite o `.env` com `GITEA_URL`, `GITEA_TOKEN`, `GITEA_BOT_USERNAME`, `OLLAMA_URL` e `OLLAMA_MODEL`.

Depois de subir os servicos, a API fica disponivel em:

```text
http://localhost:8088
```

O worker fica em outro container e consome os jobs da fila Redis. Os logs de diffs, prompts e respostas ficam em `logs/diffs/`.

Para iniciar um review manualmente, envie a URL do Pull Request para a mesma API. O job entra na fila e segue exatamente o mesmo fluxo do webhook:

```sh
curl -X POST http://localhost:8088/review \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://gitea.example/Qualyagro/Oracle-APP/pulls/283"}'
```

A API responde `202 Accepted`; o worker busca o diff, executa o review e salva o resultado em `final-review.md`. Por padrao, reviews manuais nao publicam comentarios no Pull Request; para habilitar a publicacao, use `REVIEW_PUBLISH_MANUAL_REVIEWS=true` no `.env`.

Por seguranca, `REVIEW_ALLOW_AUTONOMOUS_REJECTION=false` por padrao. Assim, reviews que indicariam aprovacao ou rejeicao sao publicados como comentario (`COMMENT`), sem alterar o status do Pull Request, preservando o corpo, os comentarios e as marcacoes de linha. O corpo publicado informa o status (`aprovado`, `aprovado_com_observacao` ou `reprovado`), o modelo utilizado, tokens consumidos e o tempo decorrido. Para permitir decisoes automaticas, defina essa flag como `true`.

## Como Contribuir

Veja as orientacoes em [CONTRIBUTING.md](CONTRIBUTING.md).

Contribuicoes devem preservar o contrato de resposta do modelo, pois ele e usado para gerar comentarios inline e escolher o evento final do review no Gitea.

## Licenca

Distribuido sob a licenca MIT. Veja [LICENSE](LICENSE).

## Status

O projeto ja possui o fluxo principal de review automatizado:

- webhook;
- fila;
- worker;
- busca de diff;
- blocos de review;
- chamada ao Ollama;
- consolidacao final;
- publicacao do review no Gitea.

Novos agentes podem ser adicionados mantendo o worker desacoplado por meio da interface `Agent`.
