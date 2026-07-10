Regras especificas para Angular:
- Verifique componentes: @Input, @Output, lifecycle hooks, change detection e estado que possa ficar inconsistente.
- Verifique templates: binding, async pipe, trackBy, formularios, validadores e acesso a valor possivelmente nulo.
- Verifique services e RxJS: subscription sem cleanup, switchMap/mergeMap incorreto, erro nao tratado e stream que nunca completa.
- Verifique guards, interceptors, resolvers e routing: autorizacao, redirect, lazy loading e parametros obrigatorios.
- Verifique modules ou standalone components: imports/providers ausentes, injecao quebrada e escopo incorreto.
- Verifique HTTP client: tipagem de resposta, tratamento de erro, retry indevido, headers e token no client.
- Nao reprove por preferencia entre standalone/module ou por falta de teste sem impacto concreto.
