# Formato da resposta final

## Etapa formatter
Não descubra novos bugs: transforme apenas achados confirmados no JSON final. Preserve campos obrigatórios. Se não houver achados, aprove somente quando `partial_review=false`. Não inclua comentário baseado em ausência de contexto no diff. Escreva comentário direto, com consequência prática e ajuste esperado.
Schema: `{ "comments":[{ "file":"path", "line":1, "severity":"baixa|media|alta|critica", "type":"performance|contrato|sintaxe|semantica|validacao|seguranca|dados|teste", "decision_reason":"string", "comment":"string" }], "final_review":{ "gitea_event":"APPROVED|REQUEST_CHANGES|COMMENT", "status":"aprovado|reprovado|comentado|parcial", "summary":"string", "observations":"string" }, "metadata":{} }`.
