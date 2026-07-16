# Feature Map

## Implemented Features

- AI connections: one default connection per provider, explicit provider/model defaults, add-model catalog flow, and transactional model/connection removal with reference handling.
- Gitea admin integration: create and test an instance, list organizations and repositories, and save a multiple repository selection.
- Gitea tokens are never returned by generic CRUD or dedicated GET responses and are encrypted at rest for new registrations.
- Worker jobs carry an optional Gitea instance ID; the worker resolves repository selections and configured-instance credentials, with environment fallback for older jobs.

## Important Flows

1. Open **Gitea** in the console, fill URL, name, bot user, and token, then test the connection.
2. After a successful test, register the instance, list organizations, load repositories, select several, and save.
3. The UI presents loading, error, success, and empty states while preserving the existing dark/mobile console shell.
4. Open a connection to add models from its provider catalog without re-entering connection details; destructive removals require confirmation and preserve valid profile references.

## Known Limitations

Repository selection replaces the saved selection for the selected instance. Jobs created before instance routing was added have no instance ID and use the environment fallback when no configured instance can be selected.

## Related Notes

- [[Architecture]]
- [[Change Log]]

