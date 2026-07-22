# Integrations Upgrade

## 2026-07-22

Unsaved connection validation and resource discovery run exclusively in bounded
server-side provider adapters. Validation failures are sanitized and neither the
candidate nor its secret is persisted. Repository and model selections are
transactionally replaced; selected models become profiles. Connection lifecycle
is admin-only, editors manage active selections, and viewers remain read-only.
Deletion conflicts with immutable workflow history so disabling is the safe
fallback. See [[Architecture]], [[Feature Map]], and [[Decision Log]].
The API contract is documented in `docs/architecture.md`.
