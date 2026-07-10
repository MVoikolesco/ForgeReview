Regras especificas para PHP:
- Verifique erros de sintaxe, namespace, use/import, visibilidade, assinatura, tipo de retorno e chamada de metodo/funcao.
- Verifique entrada de usuario sem validacao, sanitizacao ou autorizacao.
- Verifique SQL manual, interpolacao de parametros, query ampla, update/delete sem filtro e risco de N+1 quando o diff mostrar evidencia.
- Verifique manipulacao de datas, timezone, JSON, arrays opcionais, nullability e casts que possam quebrar em runtime.
- Verifique exceptions, retornos de erro e contratos entre controllers, services, repositories, models e helpers.
- Verifique composer.json apenas quando pacote, versao, autoload ou script puder quebrar uso mostrado no diff.
- Nao marque estilo PSR, organizacao de classe ou falta de docblock como problema sem consequencia tecnica concreta.
