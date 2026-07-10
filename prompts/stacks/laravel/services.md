Regras especificas para Services Laravel:
- Verifique se o service reaproveita regras existentes em vez de duplicar queries/validacoes.
- Verifique comunicacao com controllers, jobs, repositories e models: parametros, retorno, excecoes e side effects.
- Verifique transacoes em fluxos que alteram multiplas tabelas.
- Reprove abstracao quando ela esconder regra duplicada ou impedir reaproveitamento seguro.
