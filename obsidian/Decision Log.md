# Decision Log

## 2026-07-21: Rebuild from POC

The current ForgeReview implementation moved to `POC/`. The new application
does not import POC code and begins with a generic card-based workflow domain.

## 2026-07-21: Controlled adapter integrations

Integration credentials are not stored in SQLite. An integration stores an
environment-variable reference, while its JSON config is restricted to provider
endpoint and model settings. The runner requires an active integration and an
explicit Gitea, OpenAI-compatible, or Ollama adapter before external cards can
run. This keeps external traffic bounded to registered provider contracts and
leaves publication and arbitrary HTTP execution out of scope.
