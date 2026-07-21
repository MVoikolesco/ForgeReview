# Architecture

The new application separates `backend/` (Gin, SQLite and workflow domain) from
`frontend/` (Next.js Studio). The workflow catalog is backend-controlled and
definitions connect typed card ports. See `docs/architecture.md`.

SQLite also stores controlled integration records: key, name, provider type,
safe transport configuration, environment-variable secret reference, and
status. The runner receives provider adapters explicitly. `fetch` uses the
Gitea PR reader and `model` selects OpenAI-compatible or Ollama chat; both only
run with an active integration and resolve credentials from its named
environment variable. See [[Decision Log]] and [[Feature Map]].

Docker Compose exposes the new frontend on port 3010 and backend on port 8088;
SQLite and Redis use named volumes.
