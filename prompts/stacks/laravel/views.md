Regras especificas para Views Blade:
- Verifique variaveis usadas sem garantia de existencia.
- Verifique loops ou includes que possam disparar N+1 por acessar relations nao carregadas.
- Verifique escaping e uso inseguro de HTML bruto.
- Nao comente estilo visual subjetivo.
