Regras especificas para React:
- Verifique hooks: dependencias de useEffect/useMemo/useCallback, stale closure, render loop e cleanup ausente.
- Verifique estado e props: valor opcional usado sem tratamento, estado derivado inconsistente e atualizacao baseada em estado antigo.
- Verifique componentes controlados, formularios, keys de listas e renderizacao condicional que possa quebrar fluxo.
- Verifique chamadas assincronas: loading, erro, cancelamento, race e atualizacao apos unmount quando houver evidencia.
- Verifique TypeScript: tipo muito permissivo, contrato de props quebrado, union nao tratado e import/export incompativel.
- Verifique seguranca: dangerouslySetInnerHTML, URL externa, token no client e manipulacao de HTML.
- Verifique duplicacao de regra entre componentes, hooks e services quando puder gerar divergencia real.
- Nao reprove por preferencia de composicao, nome de componente ou falta de memoizacao sem impacto concreto.
