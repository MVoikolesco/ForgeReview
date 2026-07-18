# Divergências de importação

Nas etapas reviewer, consolidator e verifier, limite divergências de importação ao diff canônico. Não reporte a ausência de API, método, arquivo, declaração ou contexto fora do diff. Só aceite um achado quando o próprio diff provar incompatibilidade entre importação, alias e uso alterados; a evidência deve citar os trechos alterados que provam ambos os lados. A simples falta de contexto no diff exige rejeição.
