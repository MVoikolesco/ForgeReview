Regras especificas para CodeIgniter 4:
- Verifique Controllers, Filters e Services: validacao, autorizacao, sessao, CSRF, redirecionamentos e tratamento de erro.
- Verifique Models e Query Builder: filtros ausentes, joins incorretos, update/delete amplo, escape/bind incorreto, paginacao e ordenacao perigosa.
- Verifique Entities, DTOs, Libraries e Helpers: contrato de parametros, retorno, mutabilidade e side effects.
- Verifique app/Config: baseURL, CORS, cookies, sessions, filters, database e defaults inseguros.
- Verifique migrations em app/Database/Migrations: operacao destrutiva, tipo incompatibilidade ou alteracao sem caminho seguro.
- Reprove redundancia de regra, query ou transformacao quando puder gerar divergencia real entre fluxos.
- Nao reprove por falta de teste, comentario ou preferencia arquitetural sem risco tecnico concreto.
