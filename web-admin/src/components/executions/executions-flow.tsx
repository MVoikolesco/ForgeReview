"use client";

import { PreReviewModal } from "@/components/pre-review/pre-review-modal";
import { StageLogsModal } from "@/components/stage-logs/stage-logs-modal";
import { PipelineManager } from "@/components/settings/pipeline-manager";
import type { AdminRequest } from "@/lib/admin-client";
import {
  Background,
  Controls,
  Handle,
  Position,
  ReactFlow,
  useNodesState,
  type Edge,
  type Node,
  type NodeProps,
} from "@xyflow/react";
import { AlertCircle, Clock3, HardDrive, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

type Worker = {
  name: string;
  state: string;
  current_job: string;
};

type Metrics = {
  queue: {
    connected: boolean;
    stream_length: number;
    pending: number;
    workers: Worker[];
  };
  reviews: { total: number; bytes: number };
  queue_error?: string;
};

type ProgressEvent = {
  stage: string;
  status: string;
  percent: number;
  message?: string;
  group_id?: string;
  group_index?: number;
  total_groups?: number;
  files?: string[];
  findings?: number;
  failed_groups?: number;
  attempt?: number;
  max_attempts?: number;
  timestamp?: string;
  duration_ms?: number;
  error?: string;
};

type ReviewProgress = {
  name: string;
  updated_at: string;
  percent: number;
  stage: string;
  status: string;
  message: string;
  events: ProgressEvent[];
  owner?: string;
  repo?: string;
  pr_number?: number;
};

type ProgressResponse = {
  active: boolean;
  review?: ReviewProgress;
};

type ReviewLog = {
  name: string;
  updated_at: string;
  files: number;
  bytes: number;
  owner?: string;
  repo?: string;
  pr_number?: number;
};

type ReviewProfile = {
  id: number;
  is_default: number;
  is_enabled: number;
};

type ReviewPolicy = {
  id: number;
  profile_id: number;
  publish_manual_reviews: number | boolean;
  enable_detailed_stage_logs: number | boolean;
};

type StageNodeData = {
  title: string;
  subtitle: string;
  status: string;
  percent: number;
  message?: string;
  events: ProgressEvent[];
  active: boolean;
  targetPosition: Position;
  sourcePosition: Position;
  onOpenLogs: () => void;
  canOpenLogs: boolean;
};

const emptyMetrics: Metrics = {
  queue: { connected: false, stream_length: 0, pending: 0, workers: [] },
  reviews: { total: 0, bytes: 0 },
};

const workflowStages = [
  {
    id: "preparacao",
    title: "Preparação",
    subtitle: "Diff, contexto e filtros",
  },
  {
    id: "planejamento",
    title: "Planejamento",
    subtitle: "Organização dos grupos",
  },
  {
    id: "revisao",
    title: "Revisão",
    subtitle: "Análise dos blocos de arquivos",
  },
  {
    id: "consolidacao",
    title: "Consolidação",
    subtitle: "Síntese dos achados",
  },
  {
    id: "verificacao",
    title: "Verificação",
    subtitle: "Confiança e consistência",
  },
  {
    id: "formatacao",
    title: "Formatação",
    subtitle: "Composição do comentário",
  },
  {
    id: "pre-publicacao",
    title: "Pré-publicação",
    subtitle: "Revisão e autorização",
  },
  { id: "publicacao", title: "Publicação", subtitle: "Envio para o Gitea" },
] as const;

const stagePositions = [
  { x: 0, y: 0 },
  { x: 419, y: 16 },
  { x: 812, y: 0 },
  { x: 1138, y: 169 },
  { x: 923, y: 397 },
  { x: 440, y: 382 },
  { x: 9, y: 389 },
  { x: 420, y: 590 },
  { x: 812, y: 590 },
];

function statusLabel(status: string) {
  if (status === "done") return "Concluída";
  if (status === "running") return "Em execução";
  if (status === "retrying") return "Retentando";
  if (status === "failed") return "Falhou";
  if (status === "waiting") return "Aguardando autorização";
  return "Aguardando";
}

function stageName(stage: string) {
  return workflowStages.find((item) => item.id === stage)?.title || stage;
}

function formatTime(timestamp?: string) {
  if (!timestamp) return "--:--:--";
  return new Date(timestamp).toLocaleTimeString("pt-BR");
}

function formatSize(value: number) {
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`;
  return `${Math.ceil(value / 1024)} KB`;
}

function resolveNodeProgress(
  stageId: string,
  event: ProgressEvent | undefined,
  latestByStage: Record<string, ProgressEvent>,
) {
  if (
    stageId === "pre-publicacao" &&
    event?.status === "waiting" &&
    latestByStage.publicacao
  ) {
    return { status: "done", percent: 100 };
  }
  return {
    status: event?.status || "pending",
    percent:
      event?.status === "done"
        ? 100
        : Math.max(0, Math.min(100, event?.percent || 0)),
  };
}

function WorkflowNode({ data, selected }: NodeProps<Node<StageNodeData>>) {
  const status = data.status || "pending";
  const expanded =
    selected || data.active || status === "failed" || status === "retrying";
  const recentEvents = data.events.slice(-4).reverse();

  return (
    <article
      className={`execution-node ${status} ${data.active ? "active" : ""} ${expanded ? "expanded" : ""}`}
    >
      <Handle type="target" position={data.targetPosition} />
      <header>
        <span className="execution-node-status">
          <i />
          {statusLabel(status)}
        </span>
        <span className="execution-node-percent">
          <span>{data.percent}%</span>
          <button
            className="execution-node-logs-button"
            type="button"
            aria-label={`Ver logs da etapa ${data.title}`}
            title={`Ver logs da etapa ${data.title}`}
            disabled={!data.canOpenLogs}
            onClick={(event) => {
              event.stopPropagation();
              if (!data.canOpenLogs) return;
              data.onOpenLogs();
            }}
          >
            <AlertCircle size={14} />
          </button>
        </span>
      </header>
      <strong>{data.title}</strong>
      <small>{data.message || data.subtitle}</small>
      <div className="execution-node-progress">
        <span style={{ width: `${data.percent}%` }} />
      </div>
      {expanded && (
        <div className="execution-node-logs">
          {recentEvents.length ? (
            recentEvents.map((event, index) => (
              <div
                className={event.status}
                key={`${event.timestamp || event.message}-${index}`}
              >
                <header>
                  <span>{event.message || statusLabel(event.status)}</span>
                  <time>{formatTime(event.timestamp)}</time>
                </header>
                {event.group_id && (
                  <p>
                    Grupo {event.group_index || "-"}/{event.total_groups || "-"}{" "}
                    · {event.group_id}
                  </p>
                )}
                {!!event.files?.length && (
                  <ul>
                    {event.files.slice(0, 4).map((file) => (
                      <li key={file}>{file}</li>
                    ))}
                    {event.files.length > 4 && (
                      <li>+{event.files.length - 4} arquivos</li>
                    )}
                  </ul>
                )}
                {(event.findings !== undefined ||
                  event.failed_groups !== undefined) && (
                  <footer>
                    {event.findings !== undefined && (
                      <span>{event.findings} achados</span>
                    )}
                    {event.failed_groups !== undefined && (
                      <span>{event.failed_groups} grupos com falha</span>
                    )}
                  </footer>
                )}
                {event.attempt !== undefined && (
                  <footer>
                    <span>
                      Tentativa {event.attempt}/{event.max_attempts || "-"}
                    </span>
                  </footer>
                )}
              </div>
            ))
          ) : (
            <span className="execution-node-empty">
              Os logs desta etapa aparecerão aqui.
            </span>
          )}
        </div>
      )}
      <Handle type="source" position={data.sourcePosition} />
    </article>
  );
}

const nodeTypes = { workflow: WorkflowNode };

type Props = {
  request: AdminRequest;
  onAuthError: () => void;
  onPipelineName: (name: string) => void;
  editRequest: number;
  saveRequest: number;
  discardRequest: number;
  onEditingChange: (editing: boolean) => void;
};

export function ExecutionsFlow({ request, onAuthError, onPipelineName, editRequest, saveRequest, discardRequest, onEditingChange }: Props) {
  const [metrics, setMetrics] = useState<Metrics>(emptyMetrics);
  const [progress, setProgress] = useState<ProgressResponse>({ active: false });
  const [reviews, setReviews] = useState<ReviewLog[]>([]);
  const [error, setError] = useState("");
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [selectedReview, setSelectedReview] = useState("");
  const [defaultProfileID, setDefaultProfileID] = useState<number | undefined>();
  const [selectedStageLogs, setSelectedStageLogs] = useState<{
    title: string;
    stage: string;
    logs: ProgressEvent[];
  } | null>(null);
  const [preReviewName, setPreReviewName] = useState("");
  const [publicationPolicy, setPublicationPolicy] = useState<{
    id: number;
    manual: boolean;
    detailedLogs: boolean;
  } | null>(null);
  const [updatingPublicationPolicy, setUpdatingPublicationPolicy] =
    useState(false);
  const [nodes, setNodes, onNodesChange] = useNodesState<Node<StageNodeData>>(
    [],
  );
  const historicalRequest = useRef(0);

  const load = useCallback(
    async (showRefreshing = false) => {
      if (showRefreshing) setRefreshing(true);
      try {
        const [
          metricsResult,
          progressResult,
          reviewsResult,
          profiles,
          policies,
        ] = await Promise.all([
          request<Metrics>("observability/metrics"),
          request<ProgressResponse>("observability/progress"),
          request<ReviewLog[]>("observability/reviews"),
          request<ReviewProfile[]>("review/profiles"),
          request<ReviewPolicy[]>("review/policies"),
        ]);
        setMetrics(metricsResult);
        setProgress(progressResult);
        setReviews(reviewsResult || []);
        const defaultProfile = profiles.find(
          (profile) => profile.is_default === 1 && profile.is_enabled === 1,
        );
        setDefaultProfileID(defaultProfile?.id);
        const defaultPolicy = defaultProfile
          ? policies.find((policy) => policy.profile_id === defaultProfile.id)
          : undefined;
        setPublicationPolicy(
          defaultPolicy
            ? {
                id: defaultPolicy.id,
                manual:
                  defaultPolicy.publish_manual_reviews === true ||
                  Number(defaultPolicy.publish_manual_reviews) === 1,
                detailedLogs:
                  defaultPolicy.enable_detailed_stage_logs === true ||
                  Number(defaultPolicy.enable_detailed_stage_logs) === 1,
              }
            : null,
        );
        setUpdatedAt(new Date());
        setError(metricsResult.queue_error || "");
      } catch (failure) {
        if (failure instanceof Error && failure.message === "AUTH")
          return onAuthError();
        setError(failure instanceof Error ? failure.message : String(failure));
      } finally {
        if (showRefreshing) setRefreshing(false);
      }
    },
    [onAuthError, request],
  );

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => {
      if (!selectedReview) void load();
    }, 5000);
    return () => window.clearInterval(timer);
  }, [load, selectedReview]);

  async function selectHistorical(name: string) {
    const requestId = ++historicalRequest.current;
    setError("");
    setProgress({ active: false });
    setNodes([]);
    if (!name) {
      setSelectedReview("");
      void load(true);
      return;
    }
    try {
      setSelectedReview(name);
      const historical = await request<ProgressResponse>(
        `observability/progress?name=${encodeURIComponent(name)}`,
      );
      if (requestId === historicalRequest.current) setProgress(historical);
    } catch (failure) {
      if (requestId === historicalRequest.current) {
        setError(failure instanceof Error ? failure.message : String(failure));
      }
    }
  }

  async function togglePublicationPolicy() {
    if (!publicationPolicy || updatingPublicationPolicy) return;
    const manual = !publicationPolicy.manual;
    setUpdatingPublicationPolicy(true);
    setError("");
    try {
      await request(`review/policies/${publicationPolicy.id}`, {
        method: "PATCH",
        body: JSON.stringify({ publish_manual_reviews: manual ? 1 : 0 }),
      });
      setPublicationPolicy({ ...publicationPolicy, manual });
    } catch (failure) {
      if (failure instanceof Error && failure.message === "AUTH")
        return onAuthError();
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setUpdatingPublicationPolicy(false);
    }
  }

  const current = progress.review;
  const events = useMemo(() => current?.events || [], [current?.events]);
  const latestByStage = useMemo(() => {
    const latest = events.reduce<Record<string, ProgressEvent>>(
      (result, event) => {
        result[event.stage] = event;
        return result;
      },
      {},
    );
    const errorEvent = latest.erro;
    if (errorEvent) {
      const failedStage = [...workflowStages]
        .reverse()
        .find((stage) =>
          ["running", "retrying"].includes(latest[stage.id]?.status || ""),
        );
      if (failedStage) {
        latest[failedStage.id] = { ...errorEvent, stage: failedStage.id };
      }
    }
    return latest;
  }, [events]);
  const eventsByStage = useMemo(
    () =>
      events.reduce<Record<string, ProgressEvent[]>>((result, event) => {
        (result[event.stage] ||= []).push(event);
        return result;
      }, {}),
    [events],
  );

  const generatedNodes = useMemo<Node<StageNodeData>[]>(
    () =>
      workflowStages.map((stage, index) => {
        const event = latestByStage[stage.id];
        const isTopRow = index < 4;
        const { status, percent } = resolveNodeProgress(
          stage.id,
          event,
          latestByStage,
        );
        return {
          id: stage.id,
          type: "workflow",
          position: stagePositions[index],
          data: {
            title: stage.title,
            subtitle: stage.subtitle,
            status,
            percent,
            message: event?.message,
            events: eventsByStage[stage.id] || [],
            active: progress.active && current?.stage === stage.id,
            onOpenLogs: () =>
              setSelectedStageLogs({
                title: stage.title,
                stage: stage.id,
                logs: eventsByStage[stage.id] || [],
              }),
            canOpenLogs: publicationPolicy?.detailedLogs === true,
            targetPosition:
              index === 4 || index === 7
                ? Position.Top
                : isTopRow
                  ? Position.Left
                  : Position.Right,
            sourcePosition:
              index === 3 || index === 6
                ? Position.Bottom
                : isTopRow
                  ? Position.Right
                  : Position.Left,
          },
        };
      }),
    [current?.stage, eventsByStage, latestByStage, progress.active, publicationPolicy?.detailedLogs],
  );

  const edges = useMemo<Edge[]>(() => {
    const connections: Edge[] = workflowStages
      .slice(0, -1)
      .map((stage, index) => {
        const target = workflowStages[index + 1];
        const reached = Boolean(latestByStage[target.id]);
        const active = progress.active && current?.stage === target.id;
        return {
          id: `${stage.id}-${target.id}`,
          source: stage.id,
          target: target.id,
          type: "default",
          animated: active,
          className: `execution-edge ${reached ? "reached" : ""} ${active ? "active" : ""}`,
        };
      });
    const retryStage = workflowStages.find(
      (stage) => latestByStage[stage.id]?.status === "retrying",
    );
    if (retryStage) {
      connections.push({
        id: `${retryStage.id}-retry`,
        source: retryStage.id,
        target: retryStage.id,
        type: "default",
        animated: true,
        className: "execution-edge retry-loop active",
        label: `Tentativa ${latestByStage[retryStage.id].attempt || "-"}/${latestByStage[retryStage.id].max_attempts || "-"}`,
      });
    }
    return connections;
  }, [current?.stage, latestByStage, progress.active]);

  useEffect(() => {
    setNodes((previous) =>
      generatedNodes.map((node) => {
        const existing = previous.find((item) => item.id === node.id);
        return {
          ...node,
          position: existing?.position || node.position,
          selected: existing?.selected,
        };
      }),
    );
  }, [generatedNodes, setNodes]);

  const activeWorkers =
    metrics.queue.workers?.filter((worker) => worker.state === "processing")
      .length || 0;

  return <section className="executions-view pipeline-studio-only"><PipelineManager request={request} profiles={[]} catalog={[]} onAuthError={onAuthError} onSaved={() => void load(true)} readOnly profileID={defaultProfileID} onPipelineName={onPipelineName} editRequest={editRequest} saveRequest={saveRequest} discardRequest={discardRequest} onEditingChange={onEditingChange} /></section>;

  return (
    <section className="executions-view">
      {error && <div className="banner error">{error}</div>}
      <div className="execution-overview">
        <article className="execution-current">
          <div>
            <span
              className={`live-indicator ${progress.active && !selectedReview ? "active" : ""}`}
            >
              <i />
              {selectedReview
                ? "Execução histórica"
                : progress.active
                  ? "Execução ao vivo"
                  : "Última execução"}
            </span>
            <h2>{current?.name || "Nenhuma revisão registrada"}</h2>
            <p>
              {current?.owner
                ? `PR #${current?.pr_number} · ${current?.owner}/${current?.repo} · `
                : null}
              {current?.message ||
                "O pipeline será preenchido assim que uma revisão for iniciada."}
            </p>
          </div>
          <div className="execution-total-progress">
            <strong>{current?.percent || 0}%</strong>
            <span>progresso geral</span>
          </div>
        </article>
        <article className="execution-history">
          <header>
            <strong>Execuções anteriores</strong>
            <Clock3 size={16} />
          </header>
          <label className="sr-only" htmlFor="execution-history-select">
            Selecionar execução
          </label>
          <select
            id="execution-history-select"
            value={selectedReview}
            onChange={(event) => void selectHistorical(event.target.value)}
          >
            <option value="">Pipeline em tempo real</option>
            {reviews.map((review) => (
              <option key={review.name} value={review.name}>
                {review.owner && review.repo
                  ? `PR #${review.pr_number} · ${review.owner}/${review.repo}`
                  : review.name}
              </option>
            ))}
          </select>
          {selectedReview && (
            <button
              className="secondary-button"
              onClick={() => void selectHistorical("")}
            >
              Voltar para pipeline em tempo real
            </button>
          )}
          <small>
            {reviews.length} revisões registradas · {activeWorkers} workers
            ativos
          </small>
        </article>
      </div>

      <article className="execution-flow-panel pipeline-runtime-panel">
        <header>
          <div>
            <span className="eyebrow">Pipeline configurado</span>
            <h3>Pipeline efetivo do profile padrão</h3>
            <p>
              A configuração exibida é a que será usada pelas próximas reviews.
            </p>
          </div>
          <div className="execution-flow-actions">
            <button
              className={`publication-toggle ${publicationPolicy?.manual ? "enabled" : "disabled"}`}
              type="button"
              aria-pressed={publicationPolicy?.manual || false}
              onClick={() => void togglePublicationPolicy()}
              disabled={!publicationPolicy || updatingPublicationPolicy}
              title={
                publicationPolicy?.manual
                  ? "Desabilitar para revisar antes de publicar"
                  : "Habilitar publicação automática"
              }
            >
              Publicação automática:{" "}
              {publicationPolicy?.manual ? "Ativa" : "Desativada"}
            </button>
          </div>
        </header>
        <PipelineManager request={request} profiles={[]} catalog={[]} onAuthError={onAuthError} onSaved={() => void load(true)} readOnly profileID={defaultProfileID} />
      </article>

      {preReviewName && (
        <PreReviewModal
          request={request}
          name={preReviewName}
          onAuthError={onAuthError}
          onClose={() => setPreReviewName("")}
          onComplete={() => void load(true)}
        />
      )}

      {selectedStageLogs && (
        <StageLogsModal
          request={request}
          reviewName={current?.name || selectedReview}
          stage={selectedStageLogs!.stage}
          title={selectedStageLogs!.title}
          subtitle={current?.name ? `Review ${current?.name}` : "Execução atual"}
          summaryLogs={selectedStageLogs!.logs}
          onAuthError={onAuthError}
          onClose={() => setSelectedStageLogs(null)}
        />
      )}

      <footer className="execution-footnotes">
        <span className={metrics.queue.connected ? "online" : "offline"}>
          <i />
          Redis {metrics.queue.connected ? "operacional" : "indisponível"}
        </span>
        <span>
          <HardDrive size={13} />
          {metrics.queue.stream_length || 0} jobs na stream
        </span>
        <span>
          {metrics.reviews.total || 0} revisões ·{" "}
          {formatSize(metrics.reviews.bytes || 0)}
        </span>
        {reviews[0] && <span>Último artefato: {reviews[0].name}</span>}
        <span>
          Atualizado às {updatedAt?.toLocaleTimeString("pt-BR") || "--:--:--"}
        </span>
      </footer>
    </section>
  );
}
