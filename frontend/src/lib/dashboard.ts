import type {
  Integration,
  ModelProfile,
  WorkflowDefinition,
  WorkflowSummary,
} from "./types";

export const officialReviewKey = "official-gitea-pr-review";

export function publishedOfficialVersion(workflows: WorkflowSummary[]) {
  return workflows
    .find((workflow) => workflow.key === officialReviewKey)
    ?.versions.find((version) => version.status === "published");
}

export function reviewPipelineState(
  definition: WorkflowDefinition | undefined,
  integrations: Integration[],
  profiles: ModelProfile[],
) {
  if (!definition) {
    return { readiness: "Pipeline oficial não publicada", safe: false, steps: [] as string[] };
  }
  const node = (type: string) => definition.nodes.find((item) => item.type === type);
  const fetch = node("fetch");
  const publish = node("publish");
  const model = node("model");
  const activeConnection = (key: unknown) =>
    typeof key === "string" &&
    integrations.some(
      (item) => item.key === key && item.status === "active" && item.secret_configured,
    );
  const hasInput = (nodeKey: string | undefined, port: string) =>
    Boolean(nodeKey && definition.edges.some((edge) => edge.to_node === nodeKey && edge.to_port === port));
  const profile = profiles.find(
    (item) => item.key === model?.config.model_profile && item.status === "active",
  );
  const ready =
    activeConnection(fetch?.config.integration) &&
    activeConnection(publish?.config.integration) &&
    Boolean(profile && activeConnection(profile.integration_key)) &&
    hasInput(fetch?.key, "event") &&
    hasInput(publish?.key, "pull_request");
  const safe =
    publish?.config.allow_autonomous_rejection === false &&
    publish?.config.medium_severity_event === "COMMENT";
  return {
    readiness: ready ? "Pronta para revisão" : "Configuração pendente",
    safe,
    steps: definition.nodes.map((item) => item.name),
  };
}
