Regras especificas para SolidJS/TypeScript:
- Aponte apenas risco concreto em signals, memos, effects, recursos assincronos, cleanup ou estado compartilhado.
- Verifique props opcionais usadas sem tratamento e contratos quebrados entre componentes, hooks/utilitarios e services.
- Verifique chamada assincrona sem tratamento quando puder quebrar fluxo, estado ou exibicao de erro.
- Nao trate preferencia entre Solid, React ou padrao visual como problema.
