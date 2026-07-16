# Architecture

## System Context

ForgeReview exposes an authenticated admin API and Next.js console for AI and Gitea configuration.

## Components

- `internal/gitea` owns authenticated Gitea API calls for connection checks, organizations, and repositories.
- `internal/admin` owns validated admin endpoints and secret handling; `gitea_instances.token_ciphertext` stores AES-GCM ciphertext.
- `internal/gitea` resolves a worker client by job instance, repository selection, or the enabled default instance (then lowest enabled id); jobs without a configured instance fall back to `GITEA_URL`/`GITEA_TOKEN`.
- `web-admin/src/components/gitea` provides the admin wizard and repository selection flow.
- `internal/admin` enforces one default connection per provider and performs model/connection deletion in transactions, promoting active replacements when references allow it.
- `web-admin/src/components/connections` reuses the wizard in add-model mode; catalog reads use the existing connection and explicit default actions do not change the global provider.

## Data Flow

Admin POST/PATCH/test requests may carry a token. New tokens are encrypted with the 32-byte `GITEA_TOKEN_ENCRYPTION_KEY`; GET responses omit both token and ciphertext. Legacy `token_env_name` remains a fallback when ciphertext is unavailable.

Connection model changes use `setup/add-model` and destructive resource deletes. Model parameters are removed before models, profiles are reassigned to an active replacement where possible, and deletes return a conflict instead of leaving an unusable review route.

## Runtime and Deployment

The API must receive `GITEA_TOKEN_ENCRYPTION_KEY` to create or rotate encrypted tokens. Existing legacy instances can continue using their configured environment variable, and legacy Redis jobs continue using the worker environment fallback.

## Related Notes

- [[Project Overview]]
- [[Decision Log]]
- [[Feature Map]]

