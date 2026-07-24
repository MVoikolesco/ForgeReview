# Card catalog — stage 5

Stage 5 is implemented in V2 without depending on `POC/`.

## Declarative transforms

`transform.config.operations` accepts 1–32 ordered JSON operations:

```json
[
  {"op": "rename", "path": "pull.owner", "to": "repository.owner"},
  {"op": "set", "path": "policy.severity", "value": "high"},
  {"op": "remove", "path": "secret"},
  {"op": "coalesce", "paths": ["title", "name"], "to": "display_name"},
  {"op": "select", "path": "repository"}
]
```

Only `select`, `set`, `remove`, `rename`, and `coalesce` are accepted. Paths
are bounded dotted paths. There is no script execution, remote reference, or
dynamic code loading.

## Namespaced variables

`variable` supports `action: set|get`, a required name, and one controlled
namespace:

- `execution`: shared by the execution and its loop children;
- `loop`: isolated by `scope_key`;
- `card`: isolated by card and scope.

`set` uses its typed input, falling back to `config.value`. `get` can use
`config.default` when no value exists. The runtime store is concurrency-safe
and never persists credentials.

## Conditions and joins

Conditions keep legacy `true`/`false` behavior and add `match_1` through
`match_8` plus `default`. `config.branches` is evaluated in order and supports
`equals`, `not_equals`, `exists`, `contains`, `gt`, `gte`, `lt`, and `lte`.

Merge supports:

- `mode: all` — waits for every declared incoming edge;
- `mode: any` — runs on the first arrival and emits that value;
- `mode: quorum` with positive `quorum` — runs when the threshold arrives;
- optional `timeout_ms` from 1 to 60,000 — releases an incomplete `all` or
  `quorum` join with the values received so far.

Quorum is validated against the number of incoming edges. Non-selected
conditional routes are treated as skipped rather than failed nodes.

## Published subpipelines

A workflow definition can publish an immutable interface:

```json
{
  "interface": {
    "trigger_node_key": "start",
    "inputs": [
      {"key": "payload", "label": "payload", "contract": "any", "required": true}
    ],
    "outputs": [
      {
        "key": "result",
        "label": "result",
        "contract": "any",
        "required": true,
        "node_key": "transform",
        "port_key": "output"
      }
    ]
  }
}
```

Studio derives the interface from trigger `published_inputs` and each card's
published output controls. A `workflow` card selects a concrete published or
formerly published immutable version. Publishing validates the referenced
version, workflow key, interface, contracts, and reference graph.

Runtime execution:

- maps the parent input into the child interface;
- enforces required inputs and outputs;
- returns one output directly or multiple outputs as an object;
- preserves the parent execution while using the child version for publication
  idempotency;
- rejects direct/indirect cycles and depth above eight.

## Model fallback and cost

Model cards support:

- `temperature`: 0–2;
- `top_p`: greater than 0 through 1;
- `timeout_seconds`: 1–3,600, default 120;
- `keep_alive`: Ollama `0` or one duration up to 24h;
- `fallback_model_profile`: one different active profile;
- `input_cost_per_million_usd`;
- `output_cost_per_million_usd`;
- `max_cost_usd`.

The runtime estimates prompt tokens conservatively and reserves the worst-case
prompt plus `max_tokens` cost before calling a provider. A call exceeding the
configured budget is rejected before external spend. Actual provider usage is
reconciled into safe telemetry and the publication summary. Fallback runs once
after a provider failure, uses the same bounded request settings, and is
recorded without exposing prompts or responses.

Corrective validation retry remains separate: invalid structured output follows
the existing bounded model-to-validator retry contract.

## Invariants

- Published workflow versions and their interfaces are immutable.
- Frontend validation is immediate; backend validation remains authoritative.
- Archived versions that were previously published remain valid pins.
- Credentials, arbitrary code, and remote schemas are forbidden in versioned
  card configuration.
- Timeout, quorum, fallback, cost, and child-version facts are safe execution
  metadata; prompts, responses, and secrets are excluded.
