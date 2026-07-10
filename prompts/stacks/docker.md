Regras especificas para Docker/Compose:
- Verifique portas conflitantes.
- Verifique volumes com permissao perigosa.
- Verifique variaveis obrigatorias ausentes.
- Verifique secrets expostos.
- Verifique paths quebrados.
- Verifique imagens sem tag apenas quando isso afetar estabilidade.
- Verifique comandos de inicializacao que possam falhar no ambiente.
- Nao critique COPY por "sobrescrever" destino ou por nao verificar existencia; em Dockerfile, fonte ausente quebra o build.
- Nao critique volume novo quando ele apenas expõe arquivo/diretorio necessario ao container e nao amplia permissao perigosa.
