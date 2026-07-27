# Unidades semânticas

O card `semantic_units` transforma arquivos filtrados em unidades observáveis
menores que um grupo arbitrário de arquivos.

Nesta etapa, uma unidade corresponde a um hunk do unified diff e, quando
detectável, ao símbolo declarado no cabeçalho ou no próprio hunk.

## Contrato

Cada `SemanticUnit` contém:

- `unit_id` SHA-256 determinístico;
- repositório e base commit;
- arquivo e linguagem;
- tipo da unidade (`symbol` ou `hunk`);
- símbolo detectado;
- intervalo e lista exata de linhas adicionadas;
- diff limitado ao hunk;
- linhas de contexto;
- imports e dependências observáveis;
- testes e contratos relacionados entre os arquivos alterados;
- tipos de contexto realmente disponíveis.

O arquivo observado usado pelo validador permanece interno ao runtime. O prompt
recebe a representação tipada da unidade, sem duplicar o patch.

## Limites

O card aceita:

- `max_units`: 1 a 1000;
- `max_characters`: 1000 a 200000 por unidade;
- `context_lines`: 0 a 20 linhas resumidas.

Um hunk acima do limite ou uma execução que produza unidades demais falha de
forma fechada. Linhas adicionadas nunca são truncadas silenciosamente.

## Observabilidade

A extração é determinística e baseada no diff disponível. Ela não afirma ter:

- AST completo;
- arquivo completo quando o provider entregou apenas patch;
- chamadores e chamados fora do diff;
- dependências transitivas;
- testes que não estejam entre os arquivos alterados.

Esses dados serão enriquecidos posteriormente pela busca determinística de
contexto.

## Pipeline oficial

A pipeline oficial agora usa:

```text
filter
  → semantic_units
  → loop por unidade
  → Template + ReviewContract
```

O `unit_id` acompanha o registro de cobertura, permitindo consultar cada
`check_id` por unidade sem depender apenas do ordinal do loop.
