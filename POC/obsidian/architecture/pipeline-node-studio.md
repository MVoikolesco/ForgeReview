# Pipeline Node Studio

## Decisão

O Pipeline Studio substituirá as etapas monolíticas por um DAG de nós controlados.
O administrador compõe cards de entrada, preparação, condição, prompt, modelo,
validação de contrato, transformação/consolidação e publicação por meio de
portas tipadas. Não haverá execução de código, schemas ou integrações HTTP
arbitrárias definidos pelo usuário. Não existe requisito de compatibilidade para
pipelines, versões ou execuções atuais: schema, API e runtime antigos serão
removidos quando o novo contrato estiver implementado.

## Escopo aprovado

- Webhook Gitea, API POST e disparo manual permanecem como as três entradas
  obrigatórias, ativáveis e conectáveis individualmente.
- Os contratos de entrada e saída são escolhidos do catálogo versionado e
  controlado pelo sistema.
- Cada card de modelo escolhe seu próprio modelo, temperatura, top-p, tokens e
  tentativas; a seleção não pertence à aresta.
- Um card de condição possui saídas nomeadas, cada uma vinculada a uma regra
  segura e a uma cor no canvas.
- Prompt e modelo são nós distintos: o prompt produz uma solicitação tipada,
  que o modelo executa; a resposta segue para um validador de contrato.
- O validador pode acionar nova tentativa corretiva no mesmo modelo, limitada e
  auditada, sem confundir essa tentativa com uma rota técnica de fallback.

## Contrato de um nó

Todo nó persistido possui `key`, `name`, `type`, `config`, política de erro,
portas de entrada e portas de saída. Um tipo registrado declara o executor, o
schema de configuração, portas suportadas e contratos aceitos. O scheduler só
conhece essa estrutura, não conceitos concretos como Gitea, diff ou Gemini.

```text
NodeType -> executor + config schema + portas + política de erro
Node     -> instância configurada de NodeType
Port     -> direção + contrato versionado + cardinalidade + obrigatoriedade
Edge     -> porta de origem + porta de destino + condição + prioridade
Token    -> artifact imutável entregue a uma porta na execução
```

O contexto preserva metadados, variáveis declaradas e referências aos tokens.
Cada nó lê somente tokens de suas portas e emite novos tokens pelas próprias
saídas; não há mapa global de variáveis mutável e sem contrato.

## Catálogo inicial

`trigger`, `fetch`, `filter`, `group`, `loop`, `condition`, `template`,
`model`, `validate`, `transform`, `merge`, `consolidate`, `format`, `publish`,
`log`, `cache`, `workflow` e `variable`. `retry` e `error_control` são
políticas configuráveis de qualquer nó e só viram cards quando houver composição
avançada que não caiba na política comum.

`loop` cria tokens por item, escopo, concorrência e limite de iterações.
`workflow` encapsula uma definição publicada com portas declaradas, sem expandir
seus nós internos no canvas pai.

## Scheduler e estados

Uma execução e cada nó usam `pending`, `ready`, `running`, `completed`,
`skipped`, `failed`, `partial_failed`, `cancelled` e `waiting`. Um nó fica
pronto quando recebe todos os tokens obrigatórios da mesma chave de escopo e
satisfaz a condição de entrada. Isso permite ramos paralelos e joins sem regras
especiais no motor.

Cada tentativa persiste tokens de entrada/saída, erro sanitizado, duração, uso
do modelo, estado e escopo. A política de retenção controla metadata, conteúdo
temporário, debug completo ou mascaramento.

## Limites de segurança

- Adaptadores de eventos, executores, contratos, validadores semânticos e
  transformações continuam registrados no backend.
- Prompts recebem somente variáveis explicitamente permitidas pelo contrato.
- Publicação continua um nó de sistema terminal e idempotente.
- O fluxo dinâmico deve convergir para o contrato canônico de review antes de
  consolidação, verificação, formatação e publicação.

## Substituição necessária

1. Persistência: substituir stages/transitions/triggers por definições de nó,
   portas, conexões, contratos, tokens, tentativas, variáveis e subpipelines.
2. Catálogo e API: expor tipos de nó, schemas, contratos compatíveis, modelos
   disponíveis e capacidades de provider; nunca aceitar executor arbitrário.
3. Runtime: substituir o scheduler de etapas por worklist de tokens tipados,
   nós prontos, escopos de loop, joins e políticas de erro genéricas.
4. Studio: substituir o editor atual por cards especializados, portas nomeadas,
   saídas coloridas, picker de contratos/modelos e inspector por tipo de nó.
5. Observabilidade: exibir estado de nó, tentativa, token e escopo em tempo
   real, com reprocessamento quando permitido pela política.

## Ordem de entrega

1. Schema e contratos do domínio novo, removendo o modelo anterior.
2. Registro de cards, tokens, scheduler e persistência de execução genéricos.
3. Implementações de trigger, fetch, condition, template, model, validate,
   merge, format e publish.
4. API e Studio orientados integralmente a nós, portas e conexões.
5. Subpipelines, loop, cache e controles operacionais sobre o núcleo estável.

Relacionados: [[pipeline-2.0]], [[contracts]], [[data-model]].
