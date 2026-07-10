Regras especificas para Solid.js:
- Verifique signals, memos e effects: dependencia reativa perdida, createEffect com side effect incorreto e update que cria loop.
- Verifique recursos assincronos: createResource, Suspense, erro, loading, refetch e race entre chamadas.
- Verifique componentes: props opcionais, children, splitProps, event handlers e passagem de callbacks.
- Verifique stores e mutacao: update profundo incorreto, estado compartilhado e referencia que deixa de ser reativa.
- Verifique roteamento/SolidStart quando aparecer no diff: server functions, loaders, actions, cache e boundary client/server.
- Verifique JSX: condicional, lista, key/index e acesso a valor possivelmente nulo.
- Nao aplique regras especificas de React hooks como se fossem equivalentes a Solid; Solid tem modelo reativo proprio.
