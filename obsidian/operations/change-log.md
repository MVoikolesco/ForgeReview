# Change log

## 2026-07-20

- Reorganizada a raiz: arquivos Docker foram para `docker/`, o script de release para `production/build.sh` e o artefato autocontido passou a ser gerado em `production/build-result/`; referências de Compose e documentação foram atualizadas.
- Reorganizado o modal de edição de perfil em seções de identidade e regras de execução; estados booleanos agora usam switches acessíveis em vez de checkboxes.
- O salvamento do perfil passou a enviar `PATCH` mínimo, somente com os campos alterados. A flag de logs detalhados requer a aplicação da migration `013_detailed_stage_logs.sql`; ambientes ainda em `012` não possuem a coluna no banco.
- Ajustados os tamanhos mínimos de tipografia dos componentes recentes de flow, logs e configurações para priorizar legibilidade, elevando textos pequenos para a faixa de 10px e 12px.
- O modal de detalhes de perfil deixou de ser somente leitura e agora permite editar nome, descrição, estado padrão/habilitado e a policy principal, incluindo a flag de logs detalhados.
- Adicionada a flag `enable_detailed_stage_logs` em `review_policies`. Quando ativa, o worker persiste `prompt`/`response` e métricas detalhadas das tentativas LLM, e o flow habilita um modal sob demanda por etapa com agrupamento por grupo/tentativa e painéis colapsáveis para prompt, resposta, artefatos e erro.
- O ambiente dev do Next passou a manter `.next` em volume separado e usar `WATCHPACK_POLLING=true`, evitando corrupção do React Client Manifest no bind mount e restaurando a detecção de alterações pelo hot reload.
- O flow ganhou um botão de diagnóstico em cada card que abre um modal com todos os eventos persistidos da etapa, incluindo status, duração, tentativas, grupos, arquivos e erros. O modal reutiliza o padrão visual de pré-review; os eventos continuam no SQLite, sem duplicação no Redis.
- O parser das etapas LLM agora tenta recuperar JSON truncado por `unexpected EOF` fechando aspas, colchetes e chaves pendentes antes de declarar `Contrato inválido`, reduzindo falhas repetidas no reviewer por resposta cortada do modelo.
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
