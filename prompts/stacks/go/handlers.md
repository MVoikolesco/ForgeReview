Regras especificas para handlers/API em Go:
- Verifique parsing de request, validacao, status code e tratamento de erro.
- Verifique propagacao correta de context.Context.
- Verifique se o handler chama services com parametros coerentes e trata retornos/erros.
- Evite apontar estilo de roteamento sem risco real.
