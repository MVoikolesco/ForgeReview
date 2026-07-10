Regras especificas para SQL/Banco:
- Verifique filtros incorretos ou ausentes em SELECT, UPDATE e DELETE.
- Verifique risco de atualizar/deletar mais registros que o esperado.
- Verifique N+1 queries.
- Verifique query que possa causar performance ruim por falta de filtro, join incorreto ou ordenacao pesada.
- Verifique migrations que removem colunas/dados sem estrategia clara.
- Verifique risco de dados divergentes ou redundancia perigosa.
