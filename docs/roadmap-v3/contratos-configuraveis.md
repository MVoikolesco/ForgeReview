# Contratos configuráveis, validação no frontend e catálogo padrão

## Resumo

A primeira evolução será transformar o contrato de resposta em uma configuração explícita do card `validate`, usando JSON Schema. O frontend fará validação antecipada para melhorar a experiência, mas o backend continuará sendo a autoridade final.

O sistema terá um catálogo oficial, somente leitura, com contratos padrão descritos e versionados. O usuário poderá selecionar um preset ou duplicá-lo para criar um contrato personalizado.

## Contrato e validação

- Adicionar `config.response_schema` ao card `validate`.
- Usar JSON Schema como padrão oficial.
- Validar em duas camadas no frontend:
  - o conteúdo informado é JSON válido;
  - o JSON representa um JSON Schema suportado.
- Validar novamente no backend antes de salvar/publicar.
- Validar a resposta em runtime nesta ordem:

```text
resposta do modelo
  → parse JSON
  → validação JSON Schema
  → regras específicas do card
  → saída valid ou invalid
```

- Preservar o contrato legado de `Finding[]` quando `response_schema` não estiver presente.
- Diferenciar erros de:
  - JSON inválido;
  - schema inválido;
  - resposta incompatível com o schema;
  - regra adicional, como caminho de arquivo inexistente.

O card `model` continuará produzindo a resposta. O card `validate` será responsável pelo contrato.

## Frontend

- Adicionar ao `CardInspector` uma seção “Contrato de resposta”.
- Permitir:
  - selecionar um contrato padrão;
  - visualizar nome e descrição;
  - abrir o schema em editor JSON;
  - formatar o JSON;
  - validar enquanto o usuário edita;
  - restaurar o preset original;
  - criar uma cópia editável do preset.
- Impedir salvar, publicar ou executar enquanto o schema personalizado estiver inválido.
- A validação visual será orientativa; a API continuará sendo a autoridade final.
- Usar fixtures compartilhadas para garantir que frontend e backend aceitem os mesmos schemas e respostas.

## Catálogo oficial

Criar um catálogo servido pelo backend, somente leitura, por exemplo:

```text
GET /api/response-contracts
```

Cada item terá:

```json
{
  "key": "review.findings.v1",
  "name": "Achados de review",
  "description": "Lista de problemas encontrados em arquivos alterados.",
  "version": 1,
  "schema": {},
  "editable": false
}
```

Presets iniciais:

- `review.findings.v1`: lista de achados com arquivo, linha, comentário e severidade.
- `generic.object.v1`: objeto JSON genérico para respostas estruturadas.
- `generic.array.v1`: lista JSON genérica.
- `review.summary.v1`: resumo estruturado de uma análise.

Os presets oficiais não serão alterados pelo usuário. A ação “duplicar para editar” copiará o schema para `config.response_schema` do workflow.

## Persistência e compatibilidade

- O schema será armazenado dentro da configuração versionada do node `validate`.
- Não haverá tabela nova nesta primeira etapa.
- Schemas não poderão conter referências remotas ou `$ref` externos.
- A pipeline oficial continuará usando o preset equivalente ao contrato atual de `Finding[]`.
- Workflows existentes sem `response_schema` continuarão funcionando sem migração obrigatória.
- A validação do backend deverá limitar tamanho e profundidade do schema para evitar configurações abusivas.

## Documentação Markdown

Registrar a evolução nos documentos existentes:

- `docs/architecture.md`
  - contrato do card `validate`;
  - fluxo de validação;
  - endpoint do catálogo;
  - compatibilidade com o contrato legado.
- `docs/roadmap.md`
  - marcar contratos configuráveis como etapa planejada/entregue conforme a implementação.
- `obsidian/Decision Log.md`
  - decisão por JSON Schema;
  - frontend como validação antecipada;
  - backend como autoridade final;
  - catálogo oficial somente leitura.
- `obsidian/Feature Map.md`
  - catálogo de contratos;
  - presets;
  - editor e validação no Inspector.
- Criar `docs/response-contracts.md` como referência dos presets, suas descrições, exemplos de resposta válida e casos de uso.

## Testes

- Frontend:
  - JSON malformado;
  - schema JSON malformado;
  - schema semanticamente inválido;
  - resposta válida;
  - resposta incompatível;
  - seleção e duplicação de preset;
  - bloqueio de salvar/publicar/executar.
- Backend:
  - validação de schema no save;
  - rejeição de schema inseguro ou grande demais;
  - validação de resposta conforme schema;
  - diferenciação dos tipos de erro;
  - compatibilidade sem `response_schema`;
  - retry do modelo usando o mesmo contrato;
  - catálogo retornando presets estáveis.
- Integração:
  - preset `review.findings.v1` produzindo exatamente o comportamento atual;
  - resposta válida seguindo para `response_filter`;
  - resposta inválida seguindo para `validate.invalid`.

## Assumptions

- O padrão será JSON Schema.
- O catálogo inicial será oficial e somente leitura.
- A personalização ocorrerá por duplicação do preset.
- O contrato ficará no card `validate`, não no `model`.
- A documentação será atualizada junto com a implementação, sem usar `POC/` como dependência.
