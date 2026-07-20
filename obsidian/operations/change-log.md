# Change log

## 2026-07-20

- O Pipeline Studio foi incorporado diretamente à aba Pipeline e passou a
  ocupar toda a área útil do wrapper, sem margem própria ou overflow. O
  cabeçalho mantém ações de descartar, salvar e publicar.
- Movido o Pipeline Studio para Observabilidade > Execuções em modo leitura do
  pipeline efetivo do profile padrão; a tela exibe exclusivamente o canvas. A
  aba Configurações > Pipeline voltou a exibir o mapa compacto e a seleção da
  versão atual. Validação: `npm --prefix web-admin run lint`.
- O Estúdio de Execuções não possui mais cabeçalho interno: o header real do
  Workspace mostra `Pipeline / <nome efetivo>`. Ajustado o encaixe do canvas no
  conteúdo do painel para eliminar margem, rodapé residual e overflow lateral.
- O menu do Estúdio passou a flutuar sobre o canvas. O inspector direito abre
  somente ao selecionar uma etapa, exibindo seus contratos/configurações, ou ao
  acionar o indicador de status no rodapé, que explica as rotas de erro.
  Validação: `npm --prefix web-admin run lint`.
- Os dois overlays do Estúdio agora iniciam fechados e possuem controles de
  fechar; um botão no canvas abre o menu esquerdo. Os controles de zoom foram
  reposicionados para o topo central, evitando sobreposição com os overlays.
- Adicionado o fluxo de edição pelo header do Workspace: `Editar pipeline`
  solicita o nome e cria um draft clonado do pipeline efetivo para o profile
  padrão, sem selecioná-lo automaticamente. O header passa a oferecer salvar e
  descartar durante a edição. Validação: `npm --prefix web-admin run lint`.
- O inspector de uma etapa agora separa os modos: em visualização mostra status
  e progresso da execução com configuração somente leitura; em edição expõe os
  parâmetros completos para alteração. Validação: `npm --prefix web-admin run lint`.
- Aumentada a escala tipográfica e espacial do Pipeline Studio para se alinhar
  ao flow operacional anterior: cards, logs, sidebar, inspector e controles
  agora priorizam leitura sem zoom. Validação: `npm --prefix web-admin run lint`.
- Restaurados indicadores operacionais no Studio: etapas concluídas projetam
  100%, cards abrem o modal de logs e a pré-publicação aparece entre formatação
  e publicação quando o profile exige autorização manual. A flag
  `publish_manual_reviews` passou a ser editável nas regras do profile para
  habilitar ou bypassar esse fluxo. Ramificações compatíveis agora fazem fan-out
  para todas as saídas elegíveis; validação: `go test ./...` e `npm --prefix
  web-admin run lint`.
- Ajustadas as portas dos cards do Studio para o subheader de contratos: cada
  etapa mostra uma entrada e uma saída tipadas, sem handles externos. As linhas
  herdam a cor do contrato de saída e os rótulos técnicos intermediários foram
  ocultados. Validação: `go test ./...` e `npm --prefix web-admin run lint`.
- Cada etapa agora configura o lado esquerdo ou direito de entrada e saída no
  inspector de edição. O canvas default organiza o fluxo em duas faixas, com
  retorno à esquerda na segunda, e as conexões usam os handles e as cores dos
  contratos correspondentes. Validação: `go test ./...` e `npm --prefix
  web-admin run lint`.
  Validação: `npm --prefix web-admin run lint` e `npm --prefix web-admin run build`.
- Corrigido o Compose de desenvolvimento: API e worker agora sobrescrevem o
  comando de produção `/app/server` por Air no estágio `development`, que não
  contém o binário de produção. Ver [[operations/runbook|runbook]].
- Implementado o backend de DAG Studio: versões de pipeline agora persistem
  transições e triggers editáveis, clonáveis e validados; o executor usa um
  scheduler limitado e audita artifacts com a transição de origem. Adicionado
  executor terminal `error_log`, roteamento pré-persistência por origem e
  migrations `014_pipeline_draft_profile.sql`/`015_pipeline_dag.sql`.
  Validação: `go test ./...`. Ver [[architecture/pipeline-2.0|pipeline 2.0]].
- Substituída a aba Pipeline pelo Pipeline Studio em canvas: sidebar de etapas
  e triggers, cards configuráveis com portas de contrato, conexões editáveis,
  card terminal de erro e controles de draft/publicação. Validação: `npm
  --prefix web-admin run lint` e `npm --prefix web-admin run build`.
- Reorganizada a raiz: arquivos Docker foram para `docker/`, o script de release para `production/build.sh` e o artefato autocontido passou a ser gerado em `production/build-result/`; referências de Compose e documentação foram atualizadas.
- Fixado o comando de runtime de API/worker como `/app/server` nos Compose, evitando que containers ou imagens antigas tentem executar o caminho removido `/usr/local/bin/review-bot`; o runbook agora exige recriação forçada do stack. O warning de `vm.overcommit_memory` do Redis foi documentado como ajuste do kernel do host.
- Reorganizado o modal de edição de perfil em seções de identidade e regras de execução; estados booleanos agora usam switches acessíveis em vez de checkboxes.
- O salvamento do perfil passou a enviar `PATCH` mínimo, somente com os campos alterados. A flag de logs detalhados requer a aplicação da migration `013_detailed_stage_logs.sql`; ambientes ainda em `012` não possuem a coluna no banco.
- Ajustados os tamanhos mínimos de tipografia dos componentes recentes de flow, logs e configurações para priorizar legibilidade, elevando textos pequenos para a faixa de 10px e 12px.
- O modal de detalhes de perfil deixou de ser somente leitura e agora permite editar nome, descrição, estado padrão/habilitado e a policy principal, incluindo a flag de logs detalhados.
- Adicionada a flag `enable_detailed_stage_logs` em `review_policies`. Quando ativa, o worker persiste `prompt`/`response` e métricas detalhadas das tentativas LLM, e o flow habilita um modal sob demanda por etapa com agrupamento por grupo/tentativa e painéis colapsáveis para prompt, resposta, artefatos e erro.
- O ambiente dev do Next passou a manter `.next` em volume separado e usar `WATCHPACK_POLLING=true`, evitando corrupção do React Client Manifest no bind mount e restaurando a detecção de alterações pelo hot reload.
- O flow ganhou um botão de diagnóstico em cada card que abre um modal com todos os eventos persistidos da etapa, incluindo status, duração, tentativas, grupos, arquivos e erros. O modal reutiliza o padrão visual de pré-review; os eventos continuam no SQLite, sem duplicação no Redis.
- O parser das etapas LLM agora tenta recuperar JSON truncado por `unexpected EOF` fechando aspas, colchetes e chaves pendentes antes de declarar `Contrato inválido`, reduzindo falhas repetidas no reviewer por resposta cortada do modelo.
- Corrigido o contrato de achados do pipeline para aceitar `end_line`, campo previsto pelo prompt técnico. Respostas válidas de consolidação deixam de cair no fallback determinístico por causa de `DisallowUnknownFields`; cobertura adicionada ao teste do pipeline.
- O card `pre-publicacao` do flow agora passa para 100% no frontend assim que existe progresso em `publicacao`, evitando ficar visualmente preso em 90% após a autorização manual.
- O backend passou a normalizar `final_review.summary` e `final_review.observations` depois da decisão final, impedindo publicações `aprovado` com texto pedindo correção; o prompt do formatter também foi alinhado para usar `APPROVE` em vez de `APPROVED`.
- A observabilidade do flow agora sintetiza um evento terminal de `publicacao` quando a review já está `concluido`, evitando que aprovações manuais publicadas permaneçam visualmente presas em `pre-publicacao` a 90%.
- A etapa de revisão agora retenta falhas de consulta ao provider conforme `RetryLimit` e preserva a causa por grupo quando todos os grupos falham. Ver [[architecture/pipeline-2.0|pipeline 2.0]] e [[operations/runbook|runbook]].
- O progresso da etapa registra a causa da falha do provider, e a resolução de conexão/modelo preserva o erro de configuração para diagnóstico sem registrar credenciais.
- A resolução do provider não ignora mais falhas ao descriptografar credenciais; o worker diferencia credencial inválida de rejeição HTTP 401.
- A publicação Gitea voltou a incluir status, duração, modelo, tokens e observação de aprovação no corpo, mantendo o marcador `forgereview:<id>` para reconciliação.
- Adicionado ambiente Docker de desenvolvimento com Air para API e worker Go, volume temporário dedicado e override que força o Next.js do `web-admin` em desenvolvimento sem incluí-lo no watch do backend.
- Criado o documento `docs/development.md` com inicialização, hot reload, diagnóstico, persistência e limpeza segura dos volumes. Ver também [[runbook|runbook]].

## 2026-07-18

- Ativada a área de Configurações do console com tabs dedicadas para visão geral, perfis, pipeline e tipos de etapa. A UI usa cards e modais responsivos, replica a seleção efetiva do runtime e consome o novo endpoint agregado e somente leitura `GET /api/admin/review/settings`.
- Migrado o pipeline para o modelo banco-first da 2.0. A migration 012 adiciona contratos, tipos, definições, versões, etapas, transições, snapshots, execuções, artifacts e reservas de publicação. O seed materializa o fluxo atual e versões equivalentes para profiles existentes; `PipelineEngine` substitui a ordem hardcoded, resolve executors registrados e preserva `Result`, `review_steps`, aprovação manual e publicação Gitea. Publicações recebem marca idempotente e estados incertos são reconciliados no Gitea após janela de segurança.
- Restaurado o pipeline de review multi-etapas sobre a arquitetura Gin: preparação e filtros, planejamento de grupos, revisão com retentativa de contrato, consolidação, verificação, formatação, fallbacks determinísticos e metadata de progresso persistida. O serviço continua usando a fila, políticas, publicação e pré-publicação atuais.
- Restaurado o pipeline de review multiestágio no worker Gin sem alterar rotas, fila Redis, publicação manual ou o cliente Gitea. Os adapters Ollama, OpenAI-compatible e Gemini agora expõem chat bruto limitado por estágio; metadados de chamadas e eventos detalhados persistem em `review_steps` e são retornados pela observabilidade.
- O Compose monta `web-admin/` no workspace do alvo `development`, com `node_modules` em volume nomeado e polling habilitado, para que alterações locais acionem o hot reload do Next.js sem impactar o runtime de produção.
- O registro HTTP foi modularizado: `internal/http/router.go` conserva o único ponto de composição em `RegisterRoutes`; os domínios `health`, `webhook`, `reviews` e `admin` foram movidos para pacotes próprios, cada qual com `router.go` e `RegisterRoutes`. Os contratos de paths, métodos e autenticação foram preservados e cobertos em `internal/http/router_test.go`. Ver [[architecture/system-map|mapa do sistema]].

## 2026-07-17

- Implementado backend novo com Gin, separado do legado histórico.
- Auditoria comparou `7779b3a` com `e1227cb` e identificou regressões de contratos, worker, policies, Gitea e pipeline.
- Corrigidos auth da API versionada, assets Next, webhook form, migration 009, resolução Gitea, validação de IA, cancelamento, heartbeat e continuidade do worker.
- Restauradas integrações administrativas de teste, organizações, repositórios, PRs e catálogo real Ollama/OpenRouter.
- Setup de conexão passou a proteger a API key, persistir parâmetros e concluir conexão/modelo/profile/policy em uma transação.
- Adicionada `pending_reviews` e fluxo approve/reject/rerun compatível com o painel.
- Compose passou a subir `web` com alvo `development`/`production` conforme `APP_ENVIRONMENT`.
- Documentação ampliada em [[architecture/system-map|mapa]], [[architecture/data-model|dados]], [[architecture/migration-audit|auditoria]] e [[operations/runbook|runbook]].

Limitações atuais e itens não reimplementados estão em [[architecture/migration-audit|Auditoria da migração]].
