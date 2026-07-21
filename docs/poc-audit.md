# POC Audit

The POC remains in `POC/` as behavioral reference only. The first concepts to
extract into the new application's isolated adapters are Gitea pull-request
collection and publication, provider-specific model transport, contract parsing,
file grouping, prompt construction and review formatting.

The POC's fixed review stages, shared mutable runtime state and pipeline-specific
HTTP contracts must not be reused as the Workflow Studio engine.
