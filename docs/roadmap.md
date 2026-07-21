# ForgeReview Workflow Studio Roadmap

Atualizado em 2026-07-21.

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
  por nó.
- Estados de execução persistidos: fila, execução, conclusão e falha.
- Ciclo de vida de versão: rascunho, publicação atômica e arquivamento da
  versão publicada anterior.

### Cards com execução atual

- Locais: trigger, transform, variável, log, cache, filtro, agrupamento,
  template, condição, merge, validar, filtrar resposta, consolidar e formatar.
- `fetch`: lê metadados, diff e arquivos de PR pelo adaptador Gitea.
- `model`: executa chat por adaptador OpenAI-compatible ou Ollama.
- `publish`: publica comentário controlado no Gitea com idempotência por
  execução, versão e card.
- Respostas de modelo são validadas como lista JSON de achados; respostas
  inválidas seguem pela saída `validate.invalid`.

### Integrações e segurança

- Integrações persistidas para Gitea, Ollama e OpenAI-compatible/OpenRouter.
- Credenciais não entram na definição de pipeline, SQLite nem respostas HTTP.
- Cada conexão armazena apenas a referência a uma variável de ambiente.
- Integrações ativas são obrigatórias para cards externos.
- Publicação tem ledger idempotente e evita efeito externo duplicado.

### Studio frontend

- Canvas React Flow com cards, portas, conexões e estados de execução.
- Biblioteca de cards carregada do catálogo do backend.
- Inspector editável por tipo de card.
- Bloqueio visual de conexão entre contratos incompatíveis.
- Template visual da pipeline inicial de review.
- Execução de fluxo local, polling de trabalhos enfileirados e estados por card.
- Wizard de conexões em etapas para Gitea, Ollama local, Ollama Cloud e
  OpenRouter.
- Rotas: `/studio`, `/pipelines` e `/integrations`.
- Frontend estruturado em componentes reutilizáveis, base card shell, modal
  shell, tipos/API compartilhados e SCSS modular com tokens, temas e mixins.

## Contratos HTTP atuais

- `GET /health`
- `GET /api/cards`
- `GET /api/integrations`
- `POST /api/integrations`
- `GET /api/workflows`
- `POST /api/workflows`
- `GET /api/workflow-versions/:id`
- `POST /api/workflow-versions/:id/publish`
- `POST /api/workflow-versions/:id/executions`
- `GET /api/executions/:id`

## Próximas etapas

### 1. Completar a primeira pipeline de review

- Tornar `loop` funcional com escopo por grupo, limite de iterações e
  concorrência configurável.
- Ligar `group -> loop -> template -> model -> validate` para revisar cada
  grupo de arquivos, em vez de apenas projetar o card no canvas.
- Adicionar retry corretivo no card de modelo quando `validate.invalid` for
  recebido, com limite, backoff e tentativa auditada.
- Completar políticas de erro por card: parar, ignorar, parcial, fallback e rota
  de erro.
- Reimplementar a decisão final de review, comentários inline e publicação de
  review no Gitea a partir das regras maduras da POC.
- Criar pipeline seed oficial versionada para o fluxo de review.

### 2. Concluir gerenciamento visual

- Permitir abrir uma versão existente no Studio para visualização/edição de um
  novo rascunho.
- Adicionar clonagem, importação e exportação de definições.
- Implementar exclusão de cards/conexões, confirmação de ações e validação
  visual completa antes do save.
- Permitir editar todas as configurações declaradas pelo catálogo, não apenas
  os cards hoje suportados pelo inspector.
- Adicionar minimapa, busca de cards e atalhos de teclado consistentes.

### 3. Área de integrações e modelos

- Teste de conexão para Gitea, Ollama e OpenRouter.
- Atualização, desativação e remoção segura de integrações.
- Catálogo de modelos por provider e seleção em vez de entrada textual livre.
- Parâmetros por card de modelo: temperatura, top-p, tokens, timeout,
  keep-alive, fallback e limites de custo.
- Separar configurações reutilizáveis de modelo das conexões de provider.

### 4. Execução durável e observabilidade

- SSE ou WebSocket para atualizar execução no canvas sem polling.
- Cancelamento, reprocessamento de card/grupo e retomada segura.
- Escopos de loop e paralelismo limitado por pipeline/card.
- Agendamento, pausa, espera por evento e correlação de webhooks.
- Dead-letter queue, retries de transporte e métricas de worker/fila.
- Política de retenção, mascaramento e expiração para payloads sensíveis.

### 5. Administração e segurança

- Autenticação e autorização por domínio administrativo.
- Gestão de segredos com armazenamento cifrado ou secret manager, substituindo
  a referência exclusiva a variáveis de ambiente quando houver operação remota.
- Assinatura/verificação de webhooks Gitea e idempotência de eventos de entrada.
- Auditoria de alterações de pipeline, integração e publicação.

### 6. Expansão de cards

- Transformações declarativas e variáveis com namespaces controlados.
- Subpipelines (`workflow`) com interfaces de entrada/saída publicadas.
- Cache com TTL e invalidação.
- Controle de erro avançado, joins `any`/`all` e ramos condicionais múltiplos.
- Novos adaptadores: GitHub, GitLab, Gemini, Groq e outros providers
  OpenAI-compatible, sempre registrados no backend.

## Limitações conhecidas

- O editor permite projetar a pipeline oficial, mas loop e revisão por grupo
  ainda não são executados pelo runner.
- O browser não executa testes de conexão; o wizard apenas cadastra a conexão.
- A imagem Docker de produção foi corrigida, mas a build local por Docker não
  pôde ser executada neste ambiente por erro de I/O no binário Docker.
- A política de CORS atual permite somente `http://localhost:3010`; ambientes
  externos exigirão configuração explícita de origem.
- Não há autenticação no novo backend nesta fase.

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
- Segredos não podem ser salvos no JSON do workflow nem expostos pela API.
