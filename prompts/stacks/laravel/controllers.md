Regras especificas para Controllers Laravel:
- Verifique validacao/autorizacao, chamadas ao service correto e tratamento de excecoes.
- Verifique se o controller nao duplicou regra que deveria estar em FormRequest, Service ou Policy.
- Verifique se parametros enviados para services/models batem com o contrato esperado.
- Evite sugerir reorganizacao sem bug provavel.
