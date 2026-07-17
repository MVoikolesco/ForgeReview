# Como Contribuir

Obrigado por considerar contribuir com o Gitea AI Reviewer.

Este projeto mexe com fluxo automatizado de review, entao mudancas devem preservar previsibilidade, rastreabilidade e seguranca na publicacao dos comentarios no Gitea.

## Antes de Comecar

1. Abra uma issue ou descreva claramente o problema que o PR resolve.
2. Prefira mudancas pequenas e focadas.
3. Evite misturar refatoracao grande com alteracao funcional.
4. Mantenha compatibilidade com o fluxo atual de webhook, fila, worker, Ollama e publicacao no Gitea.

## Ambiente

Configure o projeto seguindo [INSTALL.md](INSTALL.md).

Para validar localmente:

```sh
go test ./...
```

Ou via Docker:

```sh
docker run --rm -v "${PWD}:/app" -w /app golang:1.22-alpine go test ./...
```

## Padroes de Codigo

- Use `gofmt`/`go fmt ./...`.
- Mantenha funcoes pequenas quando possivel.
- Prefira interfaces simples e explicitas.
- Nao esconda erros importantes.
- Nao escreva tokens, diffs completos ou dados sensiveis no stdout quando isso puder vazar informacao.
- Chaves de IA são write-only, cifradas no SQLite e não podem usar `.env` como fallback.
- Preserve logs fisicos que ajudam a auditar prompts, diffs e respostas.

## Prompts

Prompts devem ser faceis de editar por quem nao quer mexer em Go.

Ao alterar prompts:

- Edite os quatro arquivos em `prompts/`: revisão técnica, segurança/performance, divergências de importação e formato final.
- Mantenha os caminhos desses arquivos e somente os filtros de arquivos em `config/review-prompts.yaml`.
- Nao coloque regra especifica de projeto no YAML.
- Não há seleção ou composição de prompts por stack.
- Mantenha prompts curtos, objetivos e acionaveis.
- Nao altere a estrutura de entrada ou resposta base apenas pelo Markdown.
- Nao instrua o modelo a responder em JSON, Markdown livre ou outro formato sem atualizar o parser.

## Contrato de Resposta

O formato de resposta do modelo e parte critica do sistema.

Nao altere a estrutura abaixo sem atualizar parser, testes e fluxo de publicacao:

- `STATUS`
- `ACHADOS_CONCRETOS`
- `CONTRATOS_DECLARADOS`
- `REFERENCIAS_A_VERIFICAR`
- `REGRAS_DE_VALIDACAO`
- `COMENTARIOS_INLINE`
- `REVISAO_FINAL`
- `EVENTO_GITEA`

Esse contrato e usado para transformar a resposta do modelo em comentarios inline e evento final no Gitea.

Mudancas nesse contrato devem ser tratadas como alteracao de codigo, nao apenas como ajuste de prompt.

## Testes

Inclua ou atualize testes quando alterar:

- parsing de diff;
- montagem de blocos;
- resolucao de prompts;
- parser da resposta final;
- cliente Gitea;
- cliente Ollama;
- fluxo do worker;
- filtros de arquivos.

Antes de abrir PR:

```sh
go test ./...
docker build -t gitea-agents-configurable-test .
```

## Checklist de Pull Request

- A mudanca tem escopo claro.
- `go test ./...` passou.
- O build Docker passou quando aplicavel.
- O README ou INSTALL foi atualizado quando o uso mudou.
- Prompts novos estao cadastrados em `config/review-prompts.yaml`.
- O contrato de resposta foi preservado ou atualizado com testes.
- Nao ha segredo, token ou URL privada nos exemplos.

## Licenca

Ao contribuir, voce concorda que sua contribuicao sera distribuida sob a licenca do projeto: [MIT](LICENSE).
