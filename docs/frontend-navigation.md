# Frontend navigation

Authenticated content pages use `AppShell`. It supplies the ForgeReview brand,
semantic breadcrumb trail, active route indication, responsive menu, and a
per-page actions slot. The menu is role-aware: viewers see Overview and
Pipelines; editors additionally see Studio; administrators also see
Integrations and Administration. On narrow screens the menu button exposes the
same links and Escape closes it while returning focus to the button.

Dashboard, Pipelines, Integrations, and Administration use the standard shell.
Studio retains its 58px dense canvas toolbar and offers a compact Overview / Studio
return affordance plus role-appropriate destinations in its existing action menu.
`src/lib/navigation.ts` is the single source for route visibility and breadcrumb
derivation; its pure helpers have focused unit coverage.
