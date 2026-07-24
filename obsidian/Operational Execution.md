# Operational Execution

## 2026-07-23: Safe lifecycle status

SQLite persists execution and card lifecycle events before delivering them over
SSE. Dashboard uses `/api/execution-events`; Studio uses
`/api/executions/:id/events`. Both replay from `Last-Event-ID` and only expose
execution/node identity, status, timestamps, and scope.

Raw trigger input and node inputs/outputs are split into sensitive SQLite tables
and removed seven days after execution creation (not seven days after a later
node update). `POST /api/executions/:id/reprocess` creates a new
durable execution from unexpired retained input; it is explicitly not resume.
Editors/admins can cancel before a publication attempt exists. Publication cannot
be cancelled once it begins because its idempotency ledger controls the external
effect.

See [[Architecture]], [[Feature Map]], and [[Decision Log]].
