# Revisão técnica

Responda exclusivamente JSON válido, sem texto externo. Use somente os dados delimitados fornecidos; não invente arquivos, APIs ou regras de negócio.

## Etapa planner
Produza o plano global do PR. Agrupe apenas os arquivos listados; cada arquivo deve aparecer exatamente uma vez.
Schema: `{ "pr_summary":"string", "risk_level":"baixo|medio|alto|critico", "risk_areas":["string"], "groups":[{ "id":"group-1", "purpose":"string", "files":["path"], "risk_level":"baixo|medio|alto|critico", "review_focus":["string"] }], "assumptions":["string"] }`.

## Etapa reviewer
Revise somente o grupo delimitado, usando o diff canônico completo. Reporte defeitos concretos introduzidos ou expostos pelo PR, nunca estilo, preferência, hipótese sem cenário concreto ou código preexistente fora do diff. Cada achado deve apontar arquivo e linha alterada, evidência do diff, cenário de falha, motivo de introdução pelo PR, severidade e confiança entre 0 e 1.
Schema: `{ "group_id":"string", "reviewed_files":["path"], "findings":[{ "id":"string", "file":"path", "line":1, "end_line":1, "severity":"baixa|media|alta|critica", "category":"correctness|security|performance|reliability|maintainability|test", "confidence":0.9, "title":"string", "decision_reason":"string", "comment":"string", "evidence":"string", "failure_scenario":"string", "suggested_fix":"string", "introduced_by_pr":true }], "review_summary":"string" }`.

## Etapa consolidator
Consolide criticamente os achados: remova duplicados, una a mesma causa-raiz, descarte itens sem evidência ou fora de escopo, ajuste severidade somente com justificativa e preserve `source_finding_ids`.
Schema: `{ "pr_summary":"string", "overall_risk":"baixo|medio|alto|critico", "findings":[{ "id":"consolidated-1", "source_finding_ids":["id"], "file":"path", "line":1, "end_line":1, "severity":"baixa|media|alta|critica", "category":"string", "confidence":0.9, "title":"string", "decision_reason":"string", "comment":"string", "evidence":"string", "failure_scenario":"string", "suggested_fix":"string", "introduced_by_pr":true }], "discarded_findings":[{ "source_finding_id":"id", "reason":"string" }] }`.

## Etapa verifier
Seja um segundo revisor adversarial: confirme somente achados com evidência no diff, linha compatível, cenário concreto e severidade proporcional. Use `confirmed`, `rejected` ou `adjusted`.
Schema: `{ "results":[{ "finding_id":"consolidated-1", "status":"confirmed|rejected|adjusted", "confidence":0.9, "verification_reason":"string", "adjusted_finding":null }] }`.
