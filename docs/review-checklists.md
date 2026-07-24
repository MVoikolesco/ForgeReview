# Checklists versionadas

Reviewers podem receber uma checklist fechada em `config.review_checklist`.
A configuração é um snapshot persistido dentro da versão do workflow:

```json
{
  "key": "official.pull-request.v1",
  "name": "Review de pull request",
  "version": 1,
  "items": [
    {
      "check_id": "security.authorization",
      "description": "Verificar bypass de autenticação ou autorização.",
      "category": "security",
      "minimum_context": "file"
    }
  ]
}
```

O catálogo somente leitura está disponível em:

```text
GET /api/review-checklists
```

## Contrato

Cada item possui:

- `check_id` estável e único;
- descrição objetiva;
- categoria;
- nível mínimo de contexto.

Categorias aceitas:

- `security`;
- `correctness`;
- `contracts`;
- `performance`;
- `architecture`;
- `observability`.

Contextos aceitos:

- `diff`;
- `file`;
- `symbol`;
- `dependencies`;
- `tests`;
- `contracts`;
- `repository`.

Uma checklist contém entre 1 e 64 checks. O backend valida novamente cada
snapshot antes de salvar ou publicar o workflow.

## Execução

No card `model`, o runtime acrescenta a checklist ao prompt e exige que cada
candidato utilize exatamente um `check_id` listado. O modelo não pode criar
checks adicionais.

O `candidate_validator` recebe o mesmo snapshot. Candidatos com `check_id` fora
da versão configurada recebem `NOT_APPLICABLE` deterministicamente e não geram
chamada ao modelo validador.

Sem `review_checklist`, o comportamento livre anterior permanece disponível
para compatibilidade.

## Catálogo inicial

`official.pull-request.v1` contém seis checks:

- autorização e isolamento;
- corretude comportamental;
- compatibilidade de contratos;
- performance em caminhos críticos;
- fronteiras arquiteturais;
- observabilidade de falhas.

Novas versões devem usar outra chave ou número de versão. Versões já embutidas
em workflows nunca são alteradas retroativamente.

