Regras especificas para Services/Libraries CodeIgniter 4:
- Verifique se o service/library reaproveita Models e Helpers existentes sem duplicar regra.
- Verifique contratos com Controllers e Models: parametros, retorno, excecoes e estado mutavel.
- Verifique transacoes em fluxos que alteram multiplas tabelas.
- Reprove abstracao quando ela esconder regra duplicada ou impedir reaproveitamento seguro.
