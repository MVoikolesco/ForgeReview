# Evolução além dos contratos configuráveis

## Resumo

Os contratos configuráveis resolvem apenas a validação estrutural da resposta. Para alcançar a arquitetura proposta, ainda seriam necessárias estas capacidades, em ordem recomendada:

## 1. Unidades semânticas

Substituir grupos apenas por quantidade de arquivos/caracteres por unidades com:

- arquivo;
- símbolo ou função;
- linhas alteradas;
- diff;
- código ao redor;
- imports;
- dependências;
- testes relacionados;
- contratos relacionados.

Essa etapa reduz falsos positivos causados por contexto incompleto.

## 2. Checklists fechadas

Criar checklists versionadas por revisor, por exemplo:

- segurança;
- corretude;
- contratos;
- performance;
- arquitetura;
- observabilidade.

Cada item deve possuir `check_id`, descrição, categoria e nível mínimo de contexto necessário.

## 3. Reviewer especializado

O modelo não deve receber apenas “revise este diff”. Cada chamada deve receber:

- unidade de análise;
- categoria;
- checklist;
- contrato de resposta;
- contexto disponível;
- limitações de observabilidade;
- findings já conhecidos.

A pipeline poderá executar vários reviewers em paralelo ou sequencialmente.

## 4. Candidate Findings

Separar as entidades:

```text
CandidateFinding
  → validação
  → finding confirmado ou rejeitado
```

O reviewer especializado apenas detecta candidatos. Ele não publica diretamente no Gitea.

O candidato deve conter:

- `check_id`;
- alegação;
- cenário;
- impacto;
- evidências;
- nível de confiança;
- contexto necessário;
- status.

## 5. Estados de decisão

Adicionar estados explícitos:

```text
CONFIRMED
REJECTED
NOT_APPLICABLE
NEEDS_CONTEXT
NOT_OBSERVABLE
```

A regra principal deve ser:

```text
ausência no diff ≠ ausência no sistema
```

Isso evita publicar problemas baseados apenas no que não apareceu no trecho analisado.

## 6. Validador independente

Além do validador JSON Schema, criar uma segunda etapa que receba um único candidato e responda somente:

- confirma;
- rejeita;
- precisa de contexto.

Essa etapa não deve procurar novos problemas nem reabrir todo o review.

## 7. Identidade e deduplicação

Criar fingerprint estável usando algo semelhante a:

```text
repository
+ base_commit
+ file
+ symbol
+ check_id
+ issue_type
+ affected_entity
```

A deduplicação não deve depender do texto produzido pelo modelo.

## 8. Ledger de cobertura

Persistir, por unidade e execução:

- checks previstos;
- checks executados;
- status de cada check;
- candidatos gerados;
- candidatos validados;
- checks não observáveis;
- duração e tentativas.

O review deve terminar por cobertura, não apenas quando o último nó foi executado.

## 9. Context Requests

Transformar `NEEDS_CONTEXT` em entidade persistida com:

- pergunta objetiva;
- motivo;
- símbolos/arquivos solicitados;
- tipos de evidência aceitos;
- tentativas realizadas;
- status;
- resolução.

Sem isso, o sistema identifica a incerteza, mas não consegue agir sobre ela.

## 10. Resolução automática de contexto

Implementar em camadas:

1. busca determinística no código;
2. busca de chamadores e chamados;
3. rotas e middlewares;
4. testes relacionados;
5. documentação;
6. histórico do review;
7. pergunta ao autor.

RAG e busca vetorial podem ficar para uma fase posterior. A busca estrutural no repositório deve vir antes.

## 11. Separação entre estado interno e publicação

O relatório público deve conter apenas:

- findings confirmados;
- perguntas de contexto selecionadas;
- resumo.

O estado interno deve preservar:

- candidatos rejeitados;
- `NOT_OBSERVABLE`;
- tentativas;
- evidências;
- fingerprints;
- métricas.

## 12. Passagem final de novidade

Ao final, executar uma análise restrita para encontrar no máximo alguns candidatos que:

- possuem evidência em linhas alteradas;
- não foram representados;
- não foram cobertos por check concluído.

Esses candidatos devem voltar ao mesmo fluxo de deduplicação e validação.

## 13. Métricas

Adicionar métricas para acompanhar a evolução:

- cobertura de checks;
- taxa de rejeição;
- precisão estimada;
- taxa de duplicidade;
- resolução automática de contexto;
- escalonamento humano;
- taxa de `NOT_OBSERVABLE`;
- quantidade de novidades na passagem final.

## Ordem prática recomendada

```text
1. Contratos JSON Schema
2. Catálogo e validação no frontend
3. CandidateFinding
4. Validador independente
5. Fingerprint e deduplicação
6. Checklists versionadas
7. Ledger de cobertura
8. Unidades semânticas
9. Context Requests
10. Busca determinística de contexto
11. Integração de respostas humanas
12. RAG e memória de reviews
13. Passagem final de novidade
14. Métricas e otimização
```

O maior ganho depois dos contratos provavelmente virá de `CandidateFinding + validação independente`. Isso muda o sistema de “modelo gerou e publicou” para “modelo propôs, sistema verificou e só então publicou”.
