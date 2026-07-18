# Architecture

## System Context

ForgeReview exposes an authenticated admin API and Next.js console for AI and Gitea configuration.

## Components

- `internal/gitea` owns authenticated Gitea API calls for connection checks, organizations, and repositories.
- `internal/admin` owns validated admin endpoints and secret handling; Gitea tokens and AI connection keys use shared AES-256-GCM storage.
- `internal/gitea` resolves a worker client by job instance, repository selection, or the enabled default instance (then lowest enabled id); jobs without a configured instance fall back to `GITEA_URL`/`GITEA_TOKEN`.
- Manual API submission and approval use the same repository/default instance fallback when `GiteaInstanceID` is absent.
- `web-admin/src/components/gitea` provides the admin wizard and repository selection flow.
- Gitea editing reuses the stepper visual language; the API rejects a second instance and deletes an instance together with its repository mappings.
- `web-admin/src/components/stepper/stepper-modal.tsx` owns the shared stepper shell; wizard content and footers remain flow-specific.
- Shared card surfaces use `styles/_mixins.scss` for the same border, radius, surface, and shadow treatment.
- `internal/admin` enforces one default connection per provider and performs model/connection deletion in transactions, promoting active replacements when references allow it.
- `web-admin/src/components/connections` reuses the wizard in add-model mode; catalog reads use the existing connection and explicit default actions do not change the global provider.
- `web-admin/src/types.ts` owns the shared resource-schema contract used by `web-admin/src/config/resources.ts`; it remains separate from API data contracts in `web-admin/src/lib/contracts.ts`.
- `internal/admin` exposes authenticated manual-review enqueue and Gitea pull-request catalog endpoints; selected open PRs become `Manual` jobs carrying `GiteaInstanceID`.
- `web-admin/src/components/manual-review` uses `StepperModal` for instance, organization, repository, and open-PR selection. The console mounts accessible dispatch feedback while retaining Gitea connection management.
- `observability/progress?name=...` reads a selected historical run without polling it; the executions view polls only the live pipeline, remounts the flow when its run changes, and offers an explicit return action. Repeated executions of the same PR receive separate log directories with a `-run-<timestamp>` suffix.
- Manual reviews with automatic publication disabled persist `pending-review.json`, pause at `pre-publicacao`, and are completed through authenticated admin actions that publish, reject, or enqueue the same job again.
- `internal/reviewconfig` resolves the default profile through the enabled default provider, its default connection, and that connection's default model; repository profiles remain explicitly model-bound.
- `internal/ai` defines the shared chat metadata contract used by Ollama and OpenRouter; `internal/ollama` supports unauthenticated local endpoints and Bearer-authenticated Ollama Cloud endpoints, disables model thinking for structured review calls, and accepts both chat `message.content` and compatibility `response` output.
- `internal/review/promptconfig` valida e carrega quatro Markdown editáveis para o pipeline; `internal/review/pipeline` apenas intercala esses textos com dados delimitados, respostas intermediárias e o diff canônico.
- API e worker compartilham o `reviewlog.Store` implementado no Redis; progresso, processo, diff, prompts, respostas, pendências e decisões de cada execução usam chaves com TTL de 12 horas.
- O SQLite permanece como armazenamento de configuração. O runtime não precisa montar `/logs` nem abrir arquivos de execução, evitando falhas em volumes read-only.

## Data Flow

Admin POST/PATCH/test requests may carry a secret. Gitea tokens retain their compatible nonce-prefixed AES-GCM format; AI keys are AES-GCM encrypted with connection-and-field AAD using the 32-byte `GITEA_TOKEN_ENCRYPTION_KEY`. AI keys are write-only and GET responses expose only `api_key_configured`. Generic connection creation and updates derive the server-owned `requires_auth` classification from `ai_providers.auth_type`; Ollama is unauthenticated only for parsed `localhost`, `127.0.0.0/8`, or `::1` HTTP(S) endpoints, never from its display name. Catalog, connection checks, setup, and worker configuration apply the same rule. Thus Gemini and Groq, like other authenticated providers, are rejected before insertion when no API key can be encrypted. Because SQL cannot safely duplicate Go URL/IP parsing, migration 009 marks and disables every legacy connection without ciphertext; operators must reconfigure and re-enable even legacy loopback Ollama connections. Provider failures returned by catalog and connection-test endpoints contain only operation context and HTTP status, never remote bodies.

Connection model changes use `setup/add-model` and destructive resource deletes. Model parameters are removed before models, profiles are reassigned to an active replacement where possible, and deletes return a conflict instead of leaving an unusable review route.

Remote Ollama is represented by the existing `ollama` provider with `requires_auth=1` and an encrypted connection key. The admin catalog uses `/api/tags`, the worker uses `/api/chat`, and authenticated unload is a no-op. Loopback Ollama has `requires_auth=0`; its catalog requests never send a Bearer header, including when a client submits an API key.

O pipeline entrega o mesmo diff canônico, já truncado e sinalizado quando aplicável, a planner, reviewer, consolidator, verifier e formatter. Não existe seleção de prompt por stack. A validação determinística remove alegações de divergência de importação baseadas em contexto ausente; exige evidência de importação e uso alterados no próprio diff. Comentários finais sobre importação ou alias só são mantidos quando correspondem, no mesmo arquivo e linha, a um achado validado de divergência de importação com essa prova.

## Runtime and Deployment

The API and worker must receive the same `GITEA_TOKEN_ENCRYPTION_KEY` to use persisted credentials. AI authentication is database-only: missing or undecryptable keys fail closed; no AI `.env` fallback exists.
- `build-prod.sh` produces a self-contained `Prod/` context with the Linux server binary, exported Next.js `web/`, prompts, review config, entrypoint, Compose, and a preserved `.env`; only `Prod/data` and `Prod/logs` are bind-mounted persistence. The worker reads the prompt configuration from the image at `REVIEW_PROMPT_CONFIG_PATH`, which production sets to `/app/config/review-prompts.yaml`, avoiding host permission differences on read-only prompt mounts.

## Related Notes

- [[Project Overview]]
- [[Decision Log]]
- [[Feature Map]]

