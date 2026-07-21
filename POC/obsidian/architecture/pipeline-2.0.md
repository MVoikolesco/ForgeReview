# Melhoria Pipeline 2.0

## Objetivo

Evoluir o pipeline para um workflow persistido e versionado. O usuário poderá
montar o fluxo de review,
escolher e ordenar etapas permitidas e ajustar os prompts, enquanto o backend
continua controlando os contratos, a segurança, os limites de execução e a
publicação no Gitea.

Esta evolução da arquitetura descrita em
[[overview|Arquitetura resumida]] e reaproveita o pipeline de
`internal/review/pipeline.go`, a persistência de `review_steps` e o contrato
público de `internal/contracts/review.go`.

## Estado da implementação

A fundação banco-first foi implementada na migration
`012_database_pipeline.sql`:

- o seed cria contratos, tipos e o pipeline padrão publicado;
- profiles existentes recebem uma versão equivalente às flags e limites que
  possuíam em `review_policies`;
- `Service.Process` carrega e vincula uma versão publicada à review;
- `PipelineEngine` executa apenas as etapas carregadas do banco por meio do
  registry de executors;
- a definição completa é salva como snapshot em `pipeline_executions`;
- cada etapa e artifact é auditado em `stage_executions` e `stage_artifacts`;
- prompts das etapas e instruções dos contratos são carregados do banco;
- publicação automática e aprovação manual usam reserva idempotente em
  `review_publications`, marca identificadora no Gitea e reconciliação paginada
  após uma janela de segurança de 15 minutos;
- tentativas de contrato da revisão são auditadas individualmente em
  `stage_executions`;
- `review_steps` continua sendo a projeção compatível com o painel atual.

O runtime hardcoded anterior foi removido. A implementação atual interpreta um
grafo persistido por `pipeline_transitions`, com worklist determinística limitada.
Transições de negócio aceitam uma AST de regras tipada contra o JSON Schema do
contrato de saída. Cada etapa escolhe `all_matches` ou `first_match`; ciclos só
são aceitos quando suas arestas possuem `max_traversals`, além do limite global
`scheduler_max_runs`. `retry` permanece interno à etapa. Fallbacks aceitam apenas
erro técnico, timeout ou contrato inválido. O validador compartilhado rejeita
regras incompatíveis, loops sem limite, contratos incompatíveis, publicação não
terminal e as invariantes das etapas obrigatórias. Cada versão também
persiste os triggers `webhook`, `api` e `manual`, validados antes de criar a
review. Ver `internal/review/pipeline_definition.go`,
`internal/review/pipeline_engine.go`, `internal/review/workflow_rules.go` e
migrations 014--019.

A migration 017 adiciona os modos de roteamento/junção, regras e limites das
arestas, além dos catálogos de processors e adapters de Entrypoint. O runtime é
orientado a artifacts isolados e eventos de dados/fechamento. Executa
`each_arrival`, `any` e `wait_all`, incluindo fechamento de ramos sem match;
saídas usam `all_matches` ou `first_match`. Os processors controlados
`rule_filter` e `transform_merge` filtram lotes e aplicam transformações/merges
determinísticos sem código arbitrário. As migrations 018--019 fixam contratos
versionados por stage e auditam a proveniência, o payload projetado consumido e
seu hash em joins e transições filtradas.

Fallbacks técnicos recebem o artifact original que falhou, inclusive o artifact
mesclado por `wait_all`, e só são disparados por erro, timeout ou contrato
inválido. O estado mutável dos executors legados é clonado por evento para evitar
mistura de arquivos e achados entre ramos ou iterações.

Em Observabilidade > Execuções, o Pipeline Studio exibe em modo leitura o
pipeline efetivo do profile padrão, sem os demais cards operacionais. O canvas
mantém a moldura, os cards e as informações do workflow. Em Configurações >
Pipeline permanece um mapa compacto das versões publicadas, com ação para
definir a seleção atual. As APIs continuam permitindo criar e editar drafts,
configurar triggers, conectar etapas por contratos compatíveis, validar,
publicar, clonar e restaurar versões.

Os triggers `webhook`, `api` e `manual` são representados no Studio por três
cards Entrypoint persistidos em `pipeline_version_triggers`. Cada Entrypoint
armazena posição própria e `target_stage_key`, podendo iniciar o workflow em uma
etapa diferente. A origem acompanha o job até o worker, e o scheduler começa no
destino configurado para `webhook`, `api` ou `manual`. A edição permite mover os
cards, trocar seus tipos, ativar/desativar cada origem e criar, alterar ou remover
suas conexões. Essas conexões são origens do scheduler, não
`pipeline_transitions`, pois não são outputs de uma etapa executada.
Os formulários de stages exibem prompt, modelo, tokens e retry somente quando
`use_llm` está ativo; etapas determinísticas mostram apenas os parâmetros que o
executor utiliza, como timeout.

No Workspace, `Editar pipeline` clona a versão publicada como um novo draft da
mesma definição, ou reabre o draft existente. O editor carrega o catálogo real,
mantém o draft aberto após salvar e publica pelo endpoint que valida o DAG. O
canvas fica estruturalmente bloqueado em visualização; em edição permite
adicionar e mover etapas, editar parâmetros, criar e remover conexões e alterar
  suas condições. Todas as etapas persistidas podem ser removidas no Studio; o
  rodapé informa dinamicamente as etapas mínimas ausentes e bloqueia salvar ou
  publicar até que Preparação, Verificação, Formatação e Publicação estejam no
  fluxo.
O card de pré-publicação continua sendo uma projeção e não é enviado no payload
persistido. Quando o profile exige aprovação manual, a pré-aprovação é projetada
entre Formatação e Publicação tanto na visualização quanto na edição, sem
substituir a transição real mantida no draft.

As colunas antigas de ativação e tokens em `review_policies` são usadas somente
para migrar profiles existentes no seed inicial. Depois da migração, a fonte de
verdade desses parâmetros é `pipeline_stages`; a edição administrativa desses
stages será exposta pelo CRUD dedicado futuro.

## Princípios

- A configuração do usuário define o workflow, não a estrutura dos contratos.
- Cada etapa possui um contrato de entrada e saída validado pelo backend.
- Toda execução usa um snapshot imutável da versão do pipeline.
- Falhas, retries, duração, tokens e outputs devem ser rastreáveis.
- Nenhuma etapa configurável pode publicar diretamente ou executar código
  arbitrário.
- Loops exigem limite explícito por aresta e limite global do scheduler; retry
  permanece interno ao executor.
- A publicação permanece uma operação de domínio controlada pelo backend.

## Arquitetura banco-first

O banco deve ser a fonte da verdade para a composição do workflow, mas não para
execução de código arbitrário. A aplicação mantém um catálogo de executors
conhecidos e o banco referencia esses executors por chave.

```text
Banco: tipos, etapas, prompts, contratos, ordem, transições e políticas
Código: executors, validações semânticas, segurança, fallbacks nativos e publicação
```

Isso permite criar novas variações de etapas sem criar arquivos físicos novos.
Por exemplo, três etapas de revisão podem usar o mesmo executor `llm_review`,
mas possuir nomes, prompts, modelos e posições diferentes:

```text
security-review    -> executor llm_review -> contrato review_findings
performance-review -> executor llm_review -> contrato review_findings
test-review        -> executor llm_review -> contrato review_findings
```

Um novo registro de etapa é suficiente quando o comportamento já é suportado por
um executor existente. Um comportamento novo, como consultar uma API externa,
executar uma ferramenta estática ou publicar em outro sistema, ainda exige um
novo executor controlado no código.

## Etapas obrigatórias

As etapas obrigatórias formam uma ordem parcial mínima:

```text
preparacao -> ... -> verificacao -> formatacao -> publicacao
```

Regras:

- `preparacao` é sempre a primeira etapa.
- `verificacao` deve ocorrer depois das etapas que produzem achados.
- `formatacao` deve ocorrer depois de `verificacao`.
- `publicacao` é sempre a última etapa e não é um prompt livre.
- Instâncias configuráveis de executors conhecidos podem ser inseridas entre
  `preparacao` e `verificacao`, ou entre `verificacao` e `formatacao`, desde que
  seus contratos sejam compatíveis.
- O pipeline não pode ser salvo se estiver sem uma etapa obrigatória ou se a
  ordem violar essas regras.

No modo de edição, o título de cada card é renomeado diretamente no canvas. Os
demais parâmetros ficam no inspector, para que o card preserve apenas tipo,
contratos e o resumo da sua obrigatoriedade. Critérios de filtros e rotas usam
rótulos em linguagem de produto no construtor de regras.

O Studio persiste as conexões válidas entre portas de contrato. A execução
processa cada chegada de entrada individualmente e mantém a origem dos
artifacts para auditoria.

Uma etapa pode emitir múltiplas transições compatíveis para realizar fan-out;
todas as saídas elegíveis são executadas. A publicação continua única e
idempotente.

## Modelo de definição

Uma definição de pipeline deve ser versionada e conter, no mínimo:

```json
{
  "name": "Review segura",
  "version": 3,
  "stages": [
    {
      "id": "preparacao",
      "type": "preparation",
      "required": true
    },
    {
      "id": "analise-seguranca",
      "type": "llm_review",
      "prompt": "Analise os riscos de seguranca...",
      "input": "prepared_diff",
      "output_schema": "findings"
    },
    {
      "id": "verificacao",
      "type": "verification",
      "required": true,
      "prompt": "Valide os achados com evidencia concreta..."
    },
    {
      "id": "formatacao",
      "type": "formatting",
      "required": true
    },
    {
      "id": "publicacao",
      "type": "publication",
      "required": true
    }
  ]
}
```

Uma etapa deve suportar configuração de:

- identificador estável e nome exibido;
- tipo de executor;
- prompt versionado, quando aplicável;
- contrato de entrada e contrato de saída;
- modelo ou profile de IA;
- limite de tokens, timeout e número de tentativas;
- política de falha e fallback;
- posição na sequência;
- parâmetros e variáveis permitidos.

## Tipos e instâncias de etapa

Os tipos de etapa são um catálogo controlado pelo sistema. Eles apontam para um
executor registrado no código e definem as capacidades e o contrato base:

```text
stage_types
- id
- key
- name
- executor_key
- input_contract_key
- output_contract_key
- is_system_stage
- is_enabled
```

Uma etapa configurada dentro de um pipeline é uma instância de um tipo:

```text
pipeline_stages
- id
- pipeline_version_id
- stage_type_id
- stage_key
- name
- position
- prompt_template
- model_id
- input_contract_key
- output_contract_key
- required
- fallback_stage_id
- retry_limit
- timeout_seconds
- config_json
- is_enabled
```

O mesmo `stage_type` pode aparecer várias vezes no mesmo pipeline. O contrato
continua sendo controlado pelo sistema, enquanto cada instância pode ter um
prompt e parâmetros diferentes.

Os executors devem ser registrados no backend por meio de uma interface comum:

```go
type StageExecutor interface {
    Execute(ctx context.Context, input StageInput) (StageOutput, error)
}
```

Tipos previstos:

- `preparation`: normaliza o diff e prepara os arquivos revisáveis;
- `llm_analysis`: executa uma análise configurável por prompt;
- `verification`: valida, filtra e ajusta achados;
- `formatting`: produz o contrato final de review;
- `publication`: aplica policies e publica no Gitea;
- `transform`: transformação determinística interna, a ser adicionada quando
  houver necessidade real.

Não faz parte da primeira versão permitir scripts, chamadas HTTP arbitrárias ou
executors definidos pelo usuário. O usuário configura instâncias de executors
conhecidos.

## Contratos entre etapas

As etapas devem compartilhar um envelope de execução, evitando que cada etapa
troque texto sem estrutura:

```json
{
  "review_id": "review-123",
  "stage_id": "analise-seguranca",
  "artifacts": {
    "prepared_diff": "...",
    "findings": []
  },
  "metadata": {}
}
```

Artefatos iniciais:

- `prepared_diff`;
- `review_plan`;
- `findings`;
- `verified_findings`;
- `formatted_review`;
- `publication_request`.

As respostas de IA devem passar por estas validações:

1. Parse da resposta JSON.
2. Validação contra JSON Schema do estágio.
3. Validação semântica, incluindo arquivo, linha, severidade, evidência e
   referências válidas.
4. Retry com instrução de correção do contrato.
5. Fallback determinístico ou falha controlada.

O contrato externo de `Result`, `Comment` e `FinalReview` deve permanecer
estável para preservar a integração com o painel e o Gitea.

## Prompts configuráveis

Prompts devem ser templates versionados com uma lista explícita de variáveis:

```text
{{prepared_diff}}
{{findings}}
{{repository}}
{{pull_request}}
{{custom_instructions}}
```

O backend deve validar as variáveis antes de salvar o pipeline e limitar o
contexto que cada tipo de etapa pode acessar. Não deve existir acesso livre ao
estado interno da aplicação.

Devem ser aplicados limites para:

- tamanho do prompt e da resposta;
- tokens por etapa e por execução;
- quantidade de chamadas;
- timeout total e por etapa;
- modelos disponíveis;
- quantidade de itens e tamanho de artifacts;
- custo estimado da execução.

## Executor do workflow

O `runPipeline` atual deve evoluir para um executor que interpreta uma definição
validada:

```go
executor.Execute(ctx, pipelineDefinition, executionContext)
```

Responsabilidades do executor:

- validar a definição antes do início;
- executar etapas na ordem configurada;
- resolver inputs e outputs entre etapas;
- aplicar timeout, retry e fallback;
- persistir checkpoints após cada etapa;
- respeitar cancelamento da review;
- interromper o fluxo em erro terminal;
- emitir eventos de progresso compatíveis com o painel atual;
- produzir o `Result` final somente após a formatação validada.

Em uma etapa futura, a validação da definição poderá usar um grafo acíclico
direcionado. O executor deverá detectar ciclos e só permitir uma etapa quando
todos os seus predecessores estiverem concluídos.

## Transições e fallbacks

As relações do fluxo também devem ser persistidas, permitindo que o pipeline
seja linear inicialmente e evolua para um DAG controlado:

```text
pipeline_transitions
- id
- pipeline_version_id
- from_stage_id
- to_stage_id
- transition_type
- condition_key
- priority
```

Os tipos iniciais de transição podem ser:

- `success`: etapa concluída com output válido;
- `retry`: chama novamente a mesma etapa;
- `fallback`: encaminha para uma etapa alternativa;
- `skip`: continua o fluxo com resultado parcial;
- `failure`: encerra a execução.

As condições devem começar como chaves conhecidas pelo backend, por exemplo
`has_findings`, `no_findings`, `partial_result`, `contract_invalid` e
`confidence_below_threshold`. Não deve existir uma linguagem de expressão livre
na primeira versão.

O fallback pode ser configurado por instância, mas sua execução permanece
controlada pelo executor:

```text
security-review --fallback_stage_id--> deterministic-filter
```

Uma etapa não deve substituir silenciosamente os artifacts de outra. Revisões
repetidas devem acumular seus achados e preservar `source_stage`, para que uma
etapa posterior de consolidação possa combinar e deduplicar os resultados.

## Persistência proposta

As tabelas atuais continuam sendo usadas para compatibilidade e observabilidade.
O modelo banco-first deve evoluir com:

- `pipeline_definitions`: identidade e vínculo com profile/repositório;
- `pipeline_versions`: versão publicada, status e configuração imutável;
- `stage_types`: catálogo dos tipos e executors disponíveis;
- `stage_contracts`: contratos versionados controlados pela aplicação;
- `pipeline_stages`: instâncias ordenadas, com prompt e parâmetros;
- `pipeline_transitions`: ordem, fallbacks e condições permitidas;
- `pipeline_executions`: snapshot da versão e estado geral da execução;
- `stage_executions`: tentativas, status, duração, modelo e erros;
- `stage_artifacts`: outputs validados ou referências para payloads maiores.

Os contratos obrigatórios são definidos pelo sistema, não pelo usuário:

```text
preparation -> prepared_diff
llm_review  -> review_findings
verification -> verified_findings
formatting  -> formatted_review
publication -> publication_request
```

O usuário pode configurar o prompt e os parâmetros da etapa, mas não pode
alterar o contrato final de publicação nem remover as validações semânticas.

Cada execução deve armazenar o snapshot da definição efetivamente usada. Alterar
um pipeline não pode modificar uma review em andamento ou dificultar a
reprodução de uma review antiga.

`review_steps` pode continuar sendo a projeção operacional consumida pelo painel
existente, enquanto `stage_executions` guarda os detalhes necessários para
retomada e auditoria.

Cada execução de etapa deve registrar:

- status e tentativa;
- input e output ou referência ao artifact;
- erro normalizado;
- duração;
- tokens de entrada e saída;
- modelo, provider e versão do prompt;
- versão do contrato;
- fallback utilizado, quando houver.

## Publicação

Publicação não deve ser uma etapa LLM livre. O fluxo deve ser:

```text
formatacao -> policy de publicacao -> aprovacao manual opcional -> Gitea
```

O executor de publicação recebe somente um `FormattedReview` validado. O
backend decide o evento permitido (`APPROVE`, `COMMENT` ou `REQUEST_CHANGES`),
considerando as policies existentes, aprovação manual e regras de segurança.

A operação deve ser idempotente para evitar comentários duplicados durante
retries ou retomadas.

## Segurança e governança

O sistema deve proteger contra:

- prompt injection no diff ou em instruções configuráveis;
- acesso de uma etapa a artifacts que não foram autorizados;
- respostas gigantes ou loops de retry;
- uso de modelos não permitidos;
- publicação provocada por conteúdo gerado pelo usuário;
- exposição de tokens, secrets ou contexto sensível;
- execução de código arbitrário.

Todo pipeline publicado deve passar por validação estrutural e por uma policy de
limites. A configuração administrativa deve registrar quem criou, alterou ou
publicou cada versão.

## Evolução do produto

### Fase 1: extração do pipeline atual

Transformar preparação, planejamento, revisão, consolidação, verificação,
formatação e publicação em executors com contratos explícitos, sem alterar a
experiência atual.

### Fase 2: Pipeline Studio configurável

Adicionar definições versionadas, CRUD administrativo, prompts por etapa,
validação das etapas obrigatórias e seleção de pipeline por profile ou
repositório.

### Fase 3: etapas LLM customizadas

Permitir etapas `llm_analysis` com prompt, modelo, contrato de saída, posição e
limites definidos pelo usuário, usando apenas schemas suportados pelo backend.

### Fase 4: resiliência e operação

Adicionar checkpoint, retry individual, retomada, cancelamento, timeout,
métricas de custo e reprocessamento a partir de uma etapa.

O DAG e o editor visual foram antecipados para a Fase 2. Extensões futuras
incluem variáveis condicionais, filtros por tipo de arquivo e regras adicionais
por contrato.

## Complexidade estimada

### MVP linear configurável

Complexidade média-alta, aproximadamente `6/10`. Envolve modelo versionado,
CRUD, executor genérico, schemas, persistência de artifacts, retry, retomada e
adaptação do painel.

### DAG visual

Complexidade alta, aproximadamente `8/10`. Adiciona paralelismo, dependências,
merge, condicionais, detecção de ciclos, reexecução parcial e editor gráfico.

### Plataforma aberta

Permitir código, scripts ou integrações externas arbitrárias elevaria a
complexidade para aproximadamente `9/10`, principalmente por isolamento,
sandbox, segurança, governança e controle de custos. Não é recomendado para a
primeira versão.

## Critérios de aceite da primeira versão

- É possível criar, editar, validar, publicar e versionar um pipeline.
- Uma review usa uma versão imutável do pipeline.
- Preparação, verificação, formatação e publicação são obrigatórias.
- O usuário consegue adicionar e ordenar etapas LLM suportadas.
- O mesmo tipo de etapa pode ser instanciado várias vezes com prompts,
  modelos e parâmetros diferentes.
- Etapas de review preservam a origem dos achados e podem ser consolidadas.
- Cada etapa valida seu output antes de liberar o próximo estágio.
- Fallbacks e transições condicionais usam somente regras conhecidas pelo
  backend.
- Retry e fallback não permitem publicar um resultado inválido.
- A publicação respeita as policies atuais e continua idempotente.
- O painel continua exibindo progresso, falhas e duração das etapas.
- Uma execução pode ser auditada e, quando seguro, retomada a partir do último
  checkpoint.
- O contrato externo de resultado permanece compatível.

## Conclusão

O ForgeReview não precisa ser reconstruído. A base atual de providers,
policies, steps, resultado e publicação pode ser reaproveitada. A mudança
central é substituir o fluxo hardcoded por um executor orientado a uma
definição versionada.

A recomendação é começar com um workflow linear configurável, quatro etapas
obrigatórias e etapas LLM opcionais entre elas. A liberdade do usuário deve
ficar na composição e nos prompts, enquanto contratos, transições, segurança,
custos e publicação permanecem sob controle do backend.
