# Ledger de cobertura

O ledger registra cobertura por execução, unidade (`unit_id` e `scope_key` do
loop), versão de contrato e `check_id`.

## Ciclo

1. O `Template` resolve o `ReviewContract` e registra todos os checks como
   `PLANNED`.
2. O reviewer produz zero ou mais `CandidateFinding`.
3. O `candidate_validator` fecha cada check com seus contadores e estado.
4. O card `publish` consulta o ledger antes de iniciar a publicação.

Se a execução falhar depois do `Template` e antes do validador, os checks
continuam `PLANNED` e aparecem como cobertura incompleta.

## Estados

- `PLANNED`: previsto, ainda sem conclusão;
- `COMPLETED`: executado sem candidato;
- `CONFIRMED`: ao menos um candidato confirmado;
- `REJECTED`: candidatos gerados, todos rejeitados;
- `NEEDS_CONTEXT`: existe candidato aguardando mais contexto;
- `NOT_OBSERVABLE`: o contexto observado não permitiu validar;
- `NOT_APPLICABLE`: candidato fora da checklist contratada.

Além do estado, cada registro preserva:

- candidatos gerados e validados;
- confirmados e rejeitados;
- pedidos de contexto;
- não observáveis e não aplicáveis;
- tentativas de validação por modelo;
- duração da validação.

## API

`GET /api/executions/:id` inclui o resumo de cobertura no campo `coverage`.

O detalhamento também está disponível isoladamente:

```text
GET /api/executions/:id/coverage
```

O ledger não armazena prompts, respostas do provider, código-fonte ou
credenciais. Check IDs, contrato, escopo e contadores ficam no plano
operacional seguro.

## Compatibilidade

Pipelines antigas sem `ReviewContract` não geram entradas de cobertura e
continuam publicáveis. Quando existem checks planejados, a publicação é
bloqueada enquanto qualquer um permanecer em `PLANNED`.
