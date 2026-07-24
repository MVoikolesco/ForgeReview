# Findings verificáveis

O fluxo oficial separa proposta, decisão e publicação:

```text
reviewer
  → CandidateFinding[]
  → validação JSON Schema e linhas observadas
  → candidate_validator (uma decisão isolada por candidato)
  → Finding[] confirmado
  → filtro, consolidação e publicação
```

## CandidateFinding

Cada candidato contém `check_id`, `claim`, `scenario`, `impact`, `evidence`,
`confidence`, `required_context`, `symbol`, `issue_type`, `affected_entity`,
`path`, `line`, `comment` e `severity`.
O reviewer não define `status`.

O sistema inicia o candidato em `PENDING`. Os estados internos são:

- `CONFIRMED`;
- `REJECTED`;
- `NOT_APPLICABLE`;
- `NEEDS_CONTEXT`;
- `NOT_OBSERVABLE`.

Somente `CONFIRMED` é convertido para o contrato público `Finding`. As decisões
completas permanecem nos outputs internos do card e nos logs sensíveis da
execução, sujeitos à retenção operacional existente; não são enviadas ao Gitea.

## Validador independente

O card `candidate_validator` recebe a lista validada e os arquivos observados.
Ele realiza uma chamada separada para cada candidato, com prompt fechado que:

- proíbe procurar novos problemas;
- avalia apenas a alegação recebida;
- reforça que ausência no diff não significa ausência no sistema;
- aceita somente `CONFIRMED`, `REJECTED` ou `NEEDS_CONTEXT`;
- exige uma justificativa objetiva.

`NOT_OBSERVABLE` é atribuído pelo sistema quando o arquivo afetado não existe no
contexto recebido. `NOT_APPLICABLE` fica reservado para checklists e unidades
semânticas, uma evolução posterior. Resposta inválida ou falha do provider
interrompe o card; o sistema nunca confirma por fallback.

O perfil do validador é configurado separadamente no Studio. Ele pode usar outro
modelo ou provider, embora a independência mínima garantida seja uma chamada e
um prompt separados do reviewer.

## Fingerprint e deduplicação

O sistema gera o fingerprint; o modelo não pode fornecê-lo. A identidade usa:

```text
repository
+ base_commit
+ path
+ symbol
+ check_id
+ issue_type
+ affected_entity
```

O valor é um SHA-256 com prefixo `sha256:`. Alegação, comentário, severidade e
linha não participam da identidade, evitando que mudanças de redação produzam
duplicatas.

O `fetch` extrai repositório e base commit do PR e os propaga como metadados
controlados dos arquivos. Quando o provider não informa o commit-base, o número
do PR é usado como fallback explícito.

A deduplicação acontece:

1. antes da chamada independente, evitando custo para candidatos repetidos no
   mesmo grupo;
2. na consolidação, eliminando repetições entre resultados de grupos distintos.

O primeiro candidato de cada fingerprint é preservado. Os seguintes ficam no
estado interno `REJECTED`, com justificativa de duplicidade. Workflows legados
sem fingerprint continuam usando a deduplicação textual anterior.

## Compatibilidade e upgrade

Workflows existentes continuam podendo produzir e publicar o contrato legado
`Finding[]`. A pipeline oficial intocada é atualizada de forma append-only:
a versão anterior é arquivada e uma nova versão publicada inclui o validador.
Pipelines oficiais modificadas pelo usuário não são substituídas.
