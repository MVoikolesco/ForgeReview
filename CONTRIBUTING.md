# Como contribuir

Mudanças no ForgeReview devem preservar previsibilidade, rastreabilidade, contratos HTTP e segurança na publicação de reviews no Gitea.

## Ambiente

Configure o projeto seguindo o [INSTALL.md](INSTALL.md). Para validar backend e frontend:

```sh
make check
```

Para formatar todo o código Go:

```sh
make fmt
```

## Estilo

O `.editorconfig` na raiz define:

- tabs e largura 4 para Go, `go.mod`, `go.sum` e receitas do `Makefile`;
- 2 espaços para TypeScript, TSX, JSON, YAML, SCSS e shell;
- 4 espaços para SQL e Dockerfiles;
- UTF-8, LF, newline final e remoção de whitespace excedente.

`gofmt` continua sendo a fonte de verdade para Go. EditorConfig não substitui o formatador.

## GoDoc

Todo símbolo exportado deve ter comentário iniciado pelo nome do símbolo:

```go
// NewRouter builds the Gin engine and registers the HTTP routes.
func NewRouter(...) *gin.Engine
```

Descreva no texto o propósito, entradas relevantes, retorno e efeitos colaterais. Não use tags PHPDoc como `@param` ou `@return`; elas não fazem parte do padrão GoDoc.

Funções privadas devem receber comentários apenas quando coordenam regras, integrações ou transformações que não sejam evidentes pelo nome.

## Organização Go/Gin

- Um arquivo deve representar uma responsabilidade coesa.
- Handlers Gin validam entrada e escrevem respostas; regras de review ficam em `internal/review`.
- Persistência de review passa por `review.Repository`.
- Integrações externas ficam em `internal/integrations` ou `internal/providers`.
- Use `net/http` para status HTTP, em vez de números soltos.
- Separe imports da biblioteca padrão, módulos internos e dependências externas.
- Prefira payloads e structs nomeados quando DTOs anônimos tornarem o handler difícil de ler.
- Não altere formato JSON, rotas, status persistidos ou regras de publicação em uma refatoração apenas estrutural.

## Segurança

- Nunca registre tokens, API keys ou ciphertexts.
- Chaves de IA e tokens Gitea são write-only e cifrados no SQLite.
- Não persista diff bruto, prompt montado ou resposta intermediária sem decisão explícita de retenção.
- Não enfraqueça autenticação, validação do contrato de IA ou policies para obter um teste verde.

## Testes

Inclua testes ao alterar contratos, handlers, migrations, fila, policies, providers, parser de resultado ou publicação Gitea.

Antes de abrir um PR:

```sh
go test -race ./...
go vet ./...
npm --prefix web-admin run lint
npm --prefix web-admin run build
docker compose config --quiet
```

Atualize `README.md`, `INSTALL.md` e as notas em `obsidian/` quando a arquitetura, operação ou comportamento mudar.
