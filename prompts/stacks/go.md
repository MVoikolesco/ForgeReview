Regras especificas para Go:
- Verifique erro de compilacao causado por assinatura alterada, metodo removido, interface nao implementada, import nao utilizado ou pacote inexistente.
- Verifique tratamento de erro: erro ignorado, contexto perdido, wrapping enganoso ou retorno incompativel com o contrato.
- Verifique nil pointer, slice/map nil, concorrencia insegura, race, goroutine sem cancelamento e channel bloqueante.
- Verifique uso de context.Context: propagacao, cancelamento, timeout e operacao que ignora contexto.
- Verifique HTTP handlers, middlewares e clients: status code, body, headers, timeout e fechamento de response body.
- Verifique structs/interfaces exportadas quando a mudanca quebrar chamadas existentes mostradas no diff.
- Verifique go.mod apenas quando pacote incorreto, versao vulneravel conhecida ou import/uso incompativel aparecer no diff.
- Nao sugira operador ternario; Go nao possui ternario.
