# ForgeReview Workflow Studio Roadmap

Atualizado em 2026-07-23.

## Visão

ForgeReview está sendo reconstruído como um Studio visual para workflows de
automação de desenvolvimento. A implementação anterior está preservada em
`POC/` como referência de comportamento, integrações, prompts e regras de
negócio. A nova aplicação não importa nem depende da POC.

## Estrutura atual

```text
POC/       Referência read-only da aplicação anterior
backend/   Gin, SQLite, Redis, motor de workflow e APIs
frontend/  Next.js, React Flow e Studio visual
docs/      Arquitetura, auditoria e este roadmap
```

## Entregue

### Fundação

- Nova aplicação separada em `backend/` e `frontend/`.
- Backend Go com Gin e SQLite como fonte de verdade.
- Redis configurado para despacho assíncrono de IDs de execução.
- Docker Compose com frontend na porta `3010`, backend na `8088`, SQLite e
  Redis persistidos em volumes.
- Imagem frontend de produção executa `next start`; `next dev` não é usado na
  imagem final.

### Domínio de workflow

- Definições versionadas de workflow com nós, portas tipadas, conexões e
  posições no canvas.
- Catálogo backend-controlado de cards por categoria.
- Validação backend de tipo de card, portas existentes e contratos compatíveis.
- Runner por disponibilidade de entradas obrigatórias, com tokens e relatório
  por nó/escopo.
- Política de erro por card: `fail` interrompe com erro sanitizado; `continue`
  registra a falha e não emite saída; `partial` registra execução parcial; e
  `route` exige uma aresta explícita da porta condicional `error` para uma
  entrada compatível. A porta só aparece no Studio quando a rota é selecionada.
- `error_control` aceita somente o token tipado e sanitizado de erro roteado;
  pode encerrar, concluir a rota, ou emitir `fallback_result`. Roteamentos em
  loops preservam o escopo filho e podem alimentar a agregação do loop.
- Estados de execução persistidos: fila, execução, conclusão e falha.
- Ciclo de vida de versão: rascunho, publicação atômica e arquivamento da
  versão publicada anterior.
- No ciclo visual, cada versão listada em Pipelines pode ser aberta no Studio.
  O Studio hidrata o grafo imutável pelo endpoint de versão e toda alteração é
  salva como um novo rascunho da mesma pipeline.
- Na inicialização, o backend garante a pipeline oficial publicada
  `official-gitea-pr-review` (versão inicial `1`) com o grafo completo de
  review. O seed é idempotente: se já houver uma versão publicada para essa
  chave, não cria versão nem altera workflows do usuário. Descubra-a por
  `GET /api/workflows` e carregue a versão retornada por
  `GET /api/workflow-versions/:id`.

### Cards com execução atual

- Locais: trigger, transform, variável, log, cache, filtro, agrupamento,
  template, condição, merge, validar, filtrar resposta, consolidar e formatar.
- `fetch`: lê metadados, diff e arquivos de PR pelo adaptador Gitea.
- `model`: executa chat por adaptador OpenAI-compatible ou Ollama.
  `max_tokens` é validado entre 1 e 128.000 (padrão 2.000) e enviado como
  `max_tokens` no contrato OpenAI-compatible ou `options.num_predict` no Ollama.
- `publish`: cria review nativa controlada no Gitea, com evento final e
  comentários inline, mantendo idempotência por execução, versão e card.
- `loop`: executa listas e grupos em escopos filhos sequenciais, aplica limite
  de iterações e agrega saídas terminais após os ramos filhos concluírem. No
  template oficial, `response_filter` é terminal por grupo e `loop.results`
  retorna ao escopo raiz para consolidar todos os achados antes de formatar e
  publicar uma única vez.
- `cache`: usa adaptador Redis explícito para ler, gravar JSON com TTL de 1 a
  86.400 segundos, ou excluir uma chave. Leituras sem valor emitem `value:
  null`; exclusões não emitem saída. Sem Redis disponível, o card falha em vez
  de usar a fila ou memória local implícita.
- Respostas de modelo são validadas como lista JSON de achados; respostas
  inválidas seguem pela saída `validate.invalid`.
- O `model` aceita `retry_limit` (padrão `0`, máximo `3`) e
  `retry_delay_ms` (padrão `0`, máximo `60000`). Quando sua saída está ligada
  diretamente a `validate.response`, uma resposta inválida pode ser corrigida
  no mesmo escopo sem adicionar uma aresta de retorno: o runner reaplica o
  prompt original com instrução estática de reparo e revalida até o limite. A
  rota `validate.invalid` permanece após esgotamento. Metadados de node runs
  registram contagens e estados de chamadas/validações, sem inserir prompts,
  respostas ou segredos nesses metadados.
- `merge` é um join real: sua entrada `collect_all` aguarda todas as arestas
  declaradas e emite a lista ordenada dos valores recebidos.
- `workflow`/subpipeline aparece no catálogo como indisponível e não pode ser
  salvo até existir um contrato de execução backend.

### Integrações e segurança

- Identidade local em SQLite: usuários têm hash bcrypt e os papéis `viewer`,
  `editor` e `admin`. Compose exige
  `FORGEREVIEW_BOOTSTRAP_ADMIN_EMAIL` e
  `FORGEREVIEW_BOOTSTRAP_ADMIN_PASSWORD`. Somente a primeira inicialização, com
  tabela vazia, cria o admin com exatamente o e-mail configurado; não há
  credencial padrão.
- Sessões usam cookie `HttpOnly`/`SameSite=Lax`, nonce revogável no SQLite e
  payload assinado por `FORGEREVIEW_SESSION_SIGNING_KEY`; nenhum token é salvo
  no browser. Viewer é somente leitura, editor gerencia rascunhos e admin
  gerencia integrações, segredos e usuários.

- Integrações persistidas para Gitea, Ollama e OpenAI-compatible/OpenRouter.
- Perfis de modelo reutilizáveis separam a seleção de modelo da conexão e de
  sua credencial. Cada perfil referencia uma conexão LLM registrada e ativa.
- Tokens/API keys não entram na definição de pipeline nem em respostas HTTP.
- Cada Token/API key é cifrado com AES-256-GCM antes de persistir em SQLite;
  apenas `secret_configured` é exposto pela API.
- A chave mestra obrigatória vem exclusivamente de
  `FORGEREVIEW_ENCRYPTION_KEY`, em base64 canônico de 32 bytes. O backend a usa
  somente para cifrar na criação e decifrar imediatamente antes da execução.
- Bancos Studio existentes migram removendo `secret_reference`; conexões legadas
  ficam sem segredo e exigem novo cadastro do Token/API key.
- Integrações ativas são obrigatórias para cards externos.
- Publicação tem ledger idempotente e evita efeito externo duplicado.

### Studio frontend

- Canvas React Flow com cards, portas, conexões e estados de execução.
- Biblioteca de cards carregada do catálogo do backend.
- Inspector editável por tipo de card.
- Bloqueio visual de conexão entre contratos incompatíveis.
- Template visual da pipeline oficial de review, com `group -> loop -> template
  -> model -> validate -> response_filter` por grupo e a fronteira explícita de
  `loop.results` para a publicação única no escopo raiz.
- Execução de fluxo local, polling de trabalhos enfileirados e estados por card.
- Wizard de conexões em etapas para Gitea, Ollama local, Ollama Cloud e
  OpenRouter.
- Rotas: `/studio`, `/pipelines` e `/integrations`.
- Dashboard operacional em `/`: saúde do backend, cartões seguros de conexão,
  definição publicada da pipeline oficial e execuções recentes, sem exibir
  payloads, erros de nós ou segredos.
- O template e a pipeline seed usam `model_profile` para a conexão de modelo;
  a publicação Gitea inicia com `medium_severity_event: "COMMENT"` e
  `allow_autonomous_rejection: false`.
- Frontend estruturado em componentes reutilizáveis, base card shell, modal
  shell, tipos/API compartilhados e SCSS modular com tokens, temas e mixins.
- Um `ToggleSwitch` compartilhado oferece semântica de input, foco, teclado,
  disabled e reduced-motion para todas as configurações booleanas. Seleções de
  repositórios/modelos usam linhas modernas com identidade do provider, contagem
  e estados hover/selecionado. A política de severidade/rejeição pertence ao
  Inspector de `publish`, nunca ao `fetch`.
- Login responsivo no estilo escuro do dashboard/Studio, com orientação de
  primeira inicialização sem mostrar senhas e feedback distinto para credenciais
  inválidas e backend de autenticação indisponível.
- Editor e admin podem selecionar cards e conexões no React Flow e removê-los
  por botão ou `Delete`/`Backspace`. A remoção sempre abre uma confirmação
  acessível do Studio (sem confirmação do navegador); remover um card remove
  também suas conexões. Viewer pode inspecionar/selecionar, mas não alterar ou
  remover o grafo.
- Antes de salvar, publicar ou executar, o Studio apresenta uma lista acionável
  de validação local para identidade, grafo vazio, portas/contratos, entradas
  obrigatórias, configurações obrigatórias dos cards conhecidos e rotas de erro.
  A API continua sendo a autoridade final para validação e persistência.

## Contratos HTTP atuais

- `GET /health`
- `GET /api/cards`
- `GET /api/integrations`
- `POST /api/integrations`
- `GET /api/model-profiles`
- `POST /api/model-profiles`
- `GET /api/workflows`
- `POST /api/workflows`
- `GET /api/workflow-versions/:id`
- `POST /api/workflow-versions/:id/publish`
- `POST /api/workflow-versions/:id/executions`
- `GET /api/executions/:id`
- `GET /api/executions?limit=10` (resumos seguros; `limit` entre 1 e 100)

## Próximas etapas

### 1. Concluir gerenciamento visual

- Permitir abrir uma versão existente no Studio para visualização/edição de um
  novo rascunho.
- Adicionar clonagem, importação e exportação de definições.
- Permitir editar todas as configurações declaradas pelo catálogo, não apenas
  os cards hoje suportados pelo inspector.
- Adicionar minimapa, busca de cards e atalhos de teclado consistentes.

### 2. Área de integrações e modelos

- Teste de conexão para Gitea, Ollama e OpenRouter.
- Atualização, desativação e remoção segura de integrações.
- Catálogo de modelos por provider e seleção em vez de entrada textual livre.
- Parâmetros por card de modelo: temperatura, top-p, timeout,
  keep-alive, fallback e limites de custo.

### 3. Execução durável e observabilidade

- SSE ou WebSocket para atualizar execução no canvas sem polling.
- Cancelamento, reprocessamento de card/grupo e retomada segura.
- Paralelismo de loop acima de `concurrency: 1`, com limites por pipeline/card.
- Agendamento, pausa, espera por evento e correlação de webhooks.
- Dead-letter queue, retries de transporte e métricas de worker/fila.
- Política de retenção, mascaramento e expiração para payloads sensíveis.

### 4. Administração e segurança

- Interface completa de listagem, alteração e desativação de usuários.
- Gestão de segredos com armazenamento cifrado ou secret manager, substituindo
   o armazenamento local AES-GCM quando houver operação remota, incluindo
   rotação de chave e re-cifragem.
- Assinatura/verificação de webhooks Gitea e idempotência de eventos de entrada.
- Auditoria de alterações de pipeline, integração e publicação.

### 5. Expansão de cards

- Transformações declarativas e variáveis com namespaces controlados.
- Implementar subpipelines (`workflow`) com interfaces de entrada/saída publicadas;
  até lá o card permanece explicitamente indisponível.
- Joins `any` e ramos condicionais múltiplos (`merge` já cobre join `all`).
- Novos adaptadores: GitHub, GitLab, Gemini, Groq e outros providers
  OpenAI-compatible, sempre registrados no backend.

## Limitações conhecidas

- Loops são sequenciais neste incremento (`concurrency` deve ser `1`) e não
  suportam aninhamento. O template oficial continua exigindo integrações Gitea
  e de modelo ativas, além das coordenadas do PR, para executar os cards
  externos.
- O browser não executa testes de conexão; o wizard apenas cadastra a conexão.
- A imagem Docker de produção foi corrigida, mas a build local por Docker não
  pôde ser executada neste ambiente por erro de I/O no binário Docker.
- A política de CORS atual permite somente `http://localhost:3010`; ambientes
  externos exigirão configuração explícita de origem.
- A sessão local requer `FORGEREVIEW_SESSION_SIGNING_KEY` e Compose exige os
  valores explícitos `FORGEREVIEW_BOOTSTRAP_ADMIN_EMAIL` e
  `FORGEREVIEW_BOOTSTRAP_ADMIN_PASSWORD`. Eles só criam a primeira conta quando
  `users` está vazia; mudanças posteriores não redefinem contas. A recuperação
  de todo acesso administrativo exige backup e reset deliberado dos registros
  locais de identidade/sessão, seguido de um novo bootstrap — não existe reset
  automático nem credencial escondida.
- Aprovação manual e reconciliação de uma publicação de review com resultado
  externo incerto ainda não são implementadas.

## Validação recorrente

```bash
go test ./...
npm run lint
npm test
npm run build
docker compose up --build
```

Execute os comandos Go em `backend/` e os comandos npm em `frontend/`.

## Decisões de continuidade

- A POC é referência, não dependência.
- Pipeline publicada é imutável.
- SQLite é a fonte de verdade; Redis nunca é a fonte definitiva de estado.
- Cards externos dependem de adaptadores controlados e integrações cadastradas.
- Segredos não podem ser salvos no JSON do workflow nem expostos pela API; a
  persistência de integrações usa apenas ciphertext autenticado.
