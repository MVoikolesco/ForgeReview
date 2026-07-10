Regras especificas para services em Go:
- Verifique se o service concentra regra reutilizavel ou apenas duplica logica de outro metodo.
- Verifique contratos entre service, repository/client e callers: parametros, retorno, erro e contexto.
- Verifique side effects escondidos que dificultem reaproveitamento seguro.
- Reprove redundancia quando duas funcoes puderem divergir em regra de negocio.
