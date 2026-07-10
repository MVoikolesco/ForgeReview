Regras especificas para Laravel/PHP:
- Verifique validacao insuficiente em controllers, requests e services.
- Verifique risco de mass assignment em Models.
- Verifique uso perigoso de update/delete sem filtro adequado.
- Verifique N+1 queries em Eloquent.
- Verifique migrations destrutivas ou incompativeis.
- Verifique queries sem paginacao/filtro quando houver risco claro de performance.
- Verifique transacoes ausentes em operacoes que alteram multiplas tabelas.
- Verifique comunicacao entre controllers, services, jobs, models e repositories.
- Redundancia de regra/query/transformacao deve reprovar quando puder gerar divergencia.
