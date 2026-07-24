# Contratos de resposta

O card `validate` aceita um JSON Schema Draft 2020-12 em
`config.response_schema`. O Studio valida antecipadamente o texto, mas a API
sempre recompila o schema antes de salvar ou publicar e o runtime valida cada
resposta do modelo.

Sem `response_schema`, o comportamento legado permanece: a resposta deve ser um
array de objetos com `path`, `line`, `comment` e `severity`. Assim, workflows
existentes não exigem migração.

## Catálogo oficial

`GET /api/response-contracts` retorna presets imutáveis:

- `review.findings.v1`: lista de achados de review;
- `review.candidate-findings.v1`: candidatos que exigem validação independente;
- `generic.object.v1`: objeto JSON genérico;
- `generic.array.v1`: lista JSON genérica;
- `review.summary.v1`: resumo com `summary` e `highlights`.

Selecionar um preset copia o schema para a versão do workflow. “Duplicar para
editar” remove o vínculo visual com o preset e mantém a cópia editável.

## Exemplo

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["summary"],
  "properties": {
    "summary": { "type": "string", "minLength": 1 }
  }
}
```

Uma resposta válida para esse contrato é:

```json
{"summary":"A alteração preserva o contrato público."}
```

## Limites e erros

- o schema deve ser um objeto com até 64 KiB e profundidade máxima 32;
- `$ref` local (`#...`) é aceito; referências externas são recusadas;
- o runtime diferencia `invalid_json`, `invalid_schema`, `schema_mismatch` e
  `additional_rule`;
- `validate_paths: true` continua exigindo uma resposta compatível com
  `Finding[]`, arquivos buscados e linhas adicionadas no patch;
- um schema genérico deve seguir para cards compatíveis com seu formato; o
  `response_filter` atual continua específico para findings.

O preset de candidatos é usado pela pipeline oficial antes do card
`candidate_validator`. Consulte `docs/findings-verificaveis.md`.
