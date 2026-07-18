# Features

Document project features, their behavior, dependencies, and related decisions.

## Related notes

- [[../architecture/overview|Architecture]]
- [[../decisions/log|Decision Log]]

## Manual review dispatch

- The console primary action and sidebar callout open the stepper in `web-admin/src/components/manual-review/manual-review-modal.tsx`.
- The flow loads configured Gitea instances, organizations, repositories, and open pull requests, then posts to the authenticated `reviews/manual` admin route.
- Successful enqueue feedback is announced with `role="status"`; failures use the modal alert state.

## Execution history

- `web-admin/src/components/executions/executions-flow.tsx` separates live polling from selected historical progress.
- Switching executions resets the React Flow canvas identity and clears the previous progress while historical data loads, so cards cannot retain the prior run.
- Repeated executions of the same PR are listed as independent runs instead of appending events to the previous run directory.
- Historical summaries include owner/repository/PR metadata parsed from the existing run directory name when available.
- Execution artifacts are stored in the shared Redis review-log namespace and expire after 12 hours; the flow does not require a writable logs volume.
- The default review profile resolves the active provider, connection, and model defaults for each new review; repository-specific profiles keep their explicit model.

## Manual publication approval

- The review policy flag is presented as `Publicar revisões automaticamente`.
- When disabled for a manual review, the worker stores a structured pending artifact and stops at the clickable `Pré-publicação` stage.
- The authenticated modal shows the exact Gitea event, main body, and inline comments, with actions to authorize publication, reject without publishing, or enqueue a new review.
