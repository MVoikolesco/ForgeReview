# ForgeReview V2

## Destaques

A V2 transforma a pipeline de review em um fluxo estruturado e verificável:

```text
reviewer
  → CandidateFinding[]
  → validação de contrato
  → deduplicação
  → validação independente
  → findings confirmados
  → publicação
```

## Contratos configuráveis

- JSON Schema Draft 2020-12 no card `validate`.
- Catálogo oficial de contratos.
- Editor, formatação, restauração e duplicação de schemas no Studio.
- Validação antecipada no frontend e autoritativa no backend.
- Limites de tamanho/profundidade e bloqueio de referências externas.
- Compatibilidade com workflows legados baseados em `Finding[]`.

## Candidate Findings e validação independente

- Nova entidade `CandidateFinding`, com alegação, cenário, impacto, evidências,
  confiança, contexto, símbolo, tipo e entidade afetada.
- Novo card `candidate_validator`.
- Uma chamada isolada por candidato.
- Estados `CONFIRMED`, `REJECTED`, `NEEDS_CONTEXT`, `NOT_OBSERVABLE` e
  `NOT_APPLICABLE`.
- Somente candidatos confirmados são convertidos em findings publicáveis.
- Falhas de contrato ou provider interrompem a etapa sem confirmação automática.

## Fingerprints e deduplicação

- Fingerprint SHA-256 gerado exclusivamente pelo sistema.
- Identidade baseada em repositório, commit-base, arquivo, símbolo, check, tipo
  do problema e entidade afetada.
- Texto, severidade e linha não interferem na identidade.
- Duplicados são removidos antes da validação independente.
- Findings são deduplicados novamente durante a consolidação entre grupos.
- O Studio mostra a quantidade de duplicados removidos.

## Checklists versionadas

- Catálogo oficial e somente leitura de checklists.
- Snapshot armazenado dentro de cada versão do workflow.
- Seis checks oficiais para segurança, corretude, contratos, performance,
  arquitetura e observabilidade.
- Reviewer limitado aos `check_id` da checklist selecionada.
- Candidatos externos à checklist recebem `NOT_APPLICABLE`.
- Checks não aplicáveis não consomem chamada do modelo validador.
- Seleção visual da checklist no reviewer e no validador.

## Pipeline oficial e compatibilidade

- Reviewer e validador podem utilizar perfis de modelo separados.
- O dashboard exige ambos configurados para declarar a pipeline pronta.
- Versões oficiais intactas recebem upgrade append-only.
- Pipelines modificadas pelo usuário não são substituídas.
- Workflows sem schema ou checklist continuam operacionais pelo caminho legado.

## Qualidade

- Testes de schemas, checklists, fingerprints, deduplicação e decisões.
- Testes do upgrade append-only da pipeline oficial.
- 36 testes frontend aprovados.
- TypeScript, build de produção, build Go e análise estática aprovados.

