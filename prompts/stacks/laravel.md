Regras especificas para Laravel:
- Verifique Form Requests, Controllers e Services: validacao, autorizacao, policies, gates e tratamento de erro.
- Verifique Eloquent: mass assignment, casts, accessors/mutators, relationships, eager loading e N+1.
- Verifique queries/update/delete: filtros ausentes, transacoes ausentes em operacoes multi-tabela e concorrencia.
- Verifique Jobs, Events, Listeners e Commands: retries, idempotencia, fila, timeout e side effects.
- Verifique migrations e seeders: alteracao destrutiva, tipo incompativel, default perigoso ou dependencia de ordem.
- Verifique routes e middleware: metodo HTTP, nome de rota, parametro obrigatorio e permissao.
- Reprove redundancia de regra/query/transformacao quando puder gerar divergencia real entre camadas.
- Nao reprove por padrao arquitetural, falta de teste ou preferencia de repository/service sem falha concreta.
