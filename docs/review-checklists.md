# Checklists versionadas

Checklists fechadas agora fazem parte de uma versão imutável de
`ReviewContract`, persistida no backend. O workflow guarda somente a referência
no card `template`:

```json
{
  "review_contract_key": "official.pull-request",
  "review_contract_version": 1
}
```

O catálogo persistido de contratos está disponível em:

```text
GET /api/review-contracts
```

`GET /api/review-checklists` continua disponível como catálogo de
compatibilidade.

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

Uma checklist contém entre 1 e 64 checks. Cada versão de contrato reúne o JSON
Schema da resposta e o snapshot dessa checklist.

## Execução

O card `template` resolve a versão no banco, acrescenta a checklist ao prompt
base e produz um `ReviewTask` tipado. O card `model` apenas executa a tarefa e
preserva o contrato na resposta.

O `validate` usa o schema carregado pelo `Template`. O `candidate_validator`
recebe o mesmo contrato propagado pela execução; candidatos com `check_id` fora
da versão recebem `NOT_APPLICABLE` deterministicamente e não geram chamada ao
modelo validador.

`config.review_checklist` e `config.response_schema` locais continuam aceitos
somente para compatibilidade com workflows anteriores.

## Catálogo inicial

`official.pull-request@1` contém seis checks:

- autorização e isolamento;
- corretude comportamental;
- compatibilidade de contratos;
- performance em caminhos críticos;
- fronteiras arquiteturais;
- observabilidade de falhas.

O catálogo também oferece contratos especializados `review.security@1`,
`review.correctness@1`, `review.contracts@1`, `review.performance@1`,
`review.architecture@1` e `review.observability@1`. Assim, a pipeline pode usar
várias instâncias explícitas do card genérico `Template`, sem criar tipos de
card fixos como “Security”.

Novas versões usam o mesmo identificador com outro número ou uma nova chave.
Uma versão existente nunca é alterada retroativamente.
