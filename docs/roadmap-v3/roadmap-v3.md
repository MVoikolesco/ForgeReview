# Roadmap v3 — ForgeReview verificável

## Registro

Criar `docs/roadmap-v3.md`, sem substituir o roadmap atual, consolidando os planejamentos desta conversa.

## Estrutura

1. **Objetivo**
   - Evoluir de reviews baseados em prompt para reviews estruturados, verificáveis e rastreáveis.

2. **Fase 1 — Contratos configuráveis**
   - JSON Schema no card `validate`.
   - Validação antecipada no frontend.
   - Validação definitiva no backend.
   - Catálogo oficial de contratos.
   - Presets e duplicação para customização.
   - Compatibilidade com `Finding[]` legado.

3. **Fase 2 — Findings verificáveis**
   - `CandidateFinding`.
   - Validador independente.
   - Estados `CONFIRMED`, `REJECTED`, `NEEDS_CONTEXT`, `NOT_OBSERVABLE` e `NOT_APPLICABLE`.
   - Fingerprints e deduplicação estável.

4. **Fase 3 — Cobertura**
   - Checklists versionadas.
   - Reviewers especializados.
   - Unidades semânticas.
   - Ledger de cobertura e critério de término.

5. **Fase 4 — Contexto**
   - `ContextRequest`.
   - Busca determinística no repositório.
   - Context Packs.
   - Respostas humanas verificáveis.
   - Integração posterior com RAG e memória de reviews.

6. **Fase 5 — Qualidade e operação**
   - Separação entre estado interno e publicação.
   - Passagem final de novidade.
   - Métricas de precisão, cobertura, rejeição, duplicidade e resolução de contexto.
   - Testes de contrato, integração e regressão.

## Documentos relacionados

Atualizar também, durante a implementação:

- `docs/architecture.md`;
- `docs/roadmap.md`;
- `obsidian/Decision Log.md`;
- `obsidian/Feature Map.md`;
- criar `docs/response-contracts.md` para o catálogo de contratos.

## Estado atual

O arquivo `docs/roadmap-v3.md` ainda não existe. A criação e edição ficam pendentes da saída do modo de planejamento para execução.
