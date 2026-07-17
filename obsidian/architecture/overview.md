# Architecture

## System Context

ForgeReview exposes an authenticated admin API and Next.js console for AI and Gitea configuration.

## Components

- `internal/gitea` owns authenticated Gitea API calls for connection checks, organizations, and repositories.
- `internal/admin` owns validated admin endpoints and secret handling; `gitea_instances.token_ciphertext` stores AES-GCM ciphertext.
- `internal/gitea` resolves a worker client by job instance, repository selection, or the enabled default instance (then lowest enabled id); jobs without a configured instance fall back to `GITEA_URL`/`GITEA_TOKEN`.
- `web-admin/src/components/gitea` provides the admin wizard and repository selection flow.
- Gitea editing reuses the stepper visual language; the API rejects a second instance and deletes an instance together with its repository mappings.
- `web-admin/src/components/stepper/stepper-modal.tsx` owns the shared stepper shell; wizard content and footers remain flow-specific.
- Shared card surfaces use `styles/_mixins.scss` for the same border, radius, surface, and shadow treatment.
- `internal/admin` enforces one default connection per provider and performs model/connection deletion in transactions, promoting active replacements when references allow it.
- `web-admin/src/components/connections` reuses the wizard in add-model mode; catalog reads use the existing connection and explicit default actions do not change the global provider.
- `internal/admin` exposes authenticated manual-review enqueue and Gitea pull-request catalog endpoints; selected open PRs become `Manual` jobs carrying `GiteaInstanceID`.
- `web-admin/src/components/manual-review` uses `StepperModal` for instance, organization, repository, and open-PR selection. The console mounts accessible dispatch feedback while retaining Gitea connection management.
- `observability/progress?name=...` reads a selected historical run without polling it; the executions view polls only the live pipeline and offers an explicit return action.
- `internal/ai` defines the shared chat metadata contract used by Ollama and OpenRouter; `internal/ollama` supports unauthenticated local endpoints and Bearer-authenticated Ollama Cloud endpoints.

## Data Flow

Admin POST/PATCH/test requests may carry a token. New tokens are encrypted with the 32-byte `GITEA_TOKEN_ENCRYPTION_KEY`; GET responses omit both token and ciphertext. Legacy `token_env_name` remains a fallback when ciphertext is unavailable.

Connection model changes use `setup/add-model` and destructive resource deletes. Model parameters are removed before models, profiles are reassigned to an active replacement where possible, and deletes return a conflict instead of leaving an unusable review route.

Ollama Cloud is represented by the existing `ollama` provider with an API-key environment-variable reference on the connection. The admin catalog uses `/api/tags`, the worker uses `/api/chat`, and cloud unload is a no-op; secrets are never persisted.

## Runtime and Deployment

The API must receive `GITEA_TOKEN_ENCRYPTION_KEY` to create or rotate encrypted tokens. Existing legacy instances can continue using their configured environment variable, and legacy Redis jobs continue using the worker environment fallback.

## Related Notes

- [[Project Overview]]
- [[Decision Log]]
- [[Feature Map]]

