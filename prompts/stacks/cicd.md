Regras especificas para CI/CD:
- Verifique comandos que podem quebrar build/test/deploy.
- Verifique secrets expostos.
- Verifique variaveis obrigatorias ausentes.
- Verifique paths incorretos.
- Verifique cache incorreto quando houver risco claro.
- Verifique etapas que rodam em ordem errada.
