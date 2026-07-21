# Generic Workflow Foundation

## Implemented (2026-07-21)

`internal/workflow` supplies a provider-independent runtime for the dynamic
workflow replacement. Definitions consist of registered node types, typed input
and output ports, port-to-port edges, scoped tokens, joins, and per-node error
policies. The scheduler is a bounded FIFO worklist and joins only tokens with
the same scope ID.

The initial backend-only executor registry is deliberately safe: `source`
(trusted token injection), `passthrough`, boolean/object-field `condition`,
two-input `merge`, `sink`, and deterministic `fail` for tests. It does not
invoke Gitea, LLM providers, user code, or arbitrary HTTP endpoints. `fail`
can fail, continue, or route a sanitized error token through the reserved
`error` output; the other node types expose that output where applicable.

Migration `internal/database/migrations/020_generic_workflow_runtime.sql`
creates independent definition, version, node, port, edge, execution, scope,
node-execution, and token tables. It does not reference legacy pipeline stages
or transitions. The Go registry remains the executable allow-list; the database
node-type catalog is descriptive metadata, not a mechanism for registering code.

Focused unit coverage in `internal/workflow/workflow_test.go` validates typed
graph rejection, required scoped joins, condition-port routing, and routed or
continued executor errors.

Related: [[pipeline-node-studio|Pipeline Node Studio]] and
[[overview|Arquitetura atual]].
