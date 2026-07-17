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
- Historical summaries include owner/repository/PR metadata parsed from the existing run directory name when available.
