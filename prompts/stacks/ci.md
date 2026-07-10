Regras especificas para CodeIgniter 4/PHP:
- Verifique Models e Query Builder: filtros ausentes, joins incorretos, update/delete amplo, escape/bind incorreto, paginacao/ordenacao perigosa.
- Verifique comunicacao entre Controllers, Models, Services, Libraries, Helpers e Entities: parametros, retorno, estado mutavel e side effects.
- Verifique validacao, sessao, permissao/autorizacao e tratamento de erro quando o diff expuser risco claro.
- Verifique migrations destrutivas ou incompativeis em app/Database/Migrations.
- Verifique redundancia de regra, query ou transformacao que possa gerar divergencia entre fluxos; trate como ponto de reprovacao quando o risco for concreto.
- Verifique abstracao e reaproveitamento: duplicacao ou service/library/helper mal extraido que aumente bug provavel ou manutencao arriscada.
- Nao reprove por falta de teste, comentario ou padrao arquitetural se nao houver risco tecnico concreto.
