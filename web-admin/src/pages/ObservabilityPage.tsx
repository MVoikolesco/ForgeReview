import {
  Background,
  Handle,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
  useNodesState,
} from "@xyflow/react";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { AdminRequest } from "../api/adminClient";

type Worker = {
  name: string;
  state: string;
  current_job: string;
  last_seen: string;
  processed: number;
  failed: number;
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
type ReviewLog = {
  name: string;
  updated_at: string;
  files: number;
  bytes: number;
};
type ProgressEvent = {
  stage: string;
  status: "running" | "done" | "failed" | string;
  percent: number;
  message?: string;
  group_id?: string;
  group_index?: number;
  total_groups?: number;
  files?: string[];
  findings?: number;
  failed_groups?: number;
  timestamp?: string;
};
type ReviewProgress = {
  name: string;
  updated_at: string;
  percent: number;
  stage: string;
  status: string;
  message: string;
  events: ProgressEvent[];
};
type ProgressResponse = {
  active: boolean;
  review?: ReviewProgress;
};
type StageNodeData = {
  title: string;
  subtitle: string;
  status: string;
  percent: number;
  message?: string;
  events: ProgressEvent[];
  active: boolean;
};

const empty: Metrics = {
  queue: { connected: false, stream_length: 0, pending: 0, workers: [] },
  reviews: { total: 0, bytes: 0 },
};
const emptyEvents: ProgressEvent[] = [];
const size = (value: number) =>
  value > 1024 * 1024
    ? `${(value / 1024 / 1024).toFixed(1)} MB`
    : `${Math.ceil(value / 1024)} KB`;

const workflowStages = [
  { id: "preparacao", title: "Preparacao", subtitle: "Diff e filtros" },
  { id: "planejamento", title: "Planejamento", subtitle: "Grupos do review" },
  { id: "revisao", title: "Review", subtitle: "Blocos de arquivos" },
  { id: "consolidacao", title: "Consolidacao", subtitle: "Achados finais" },
  { id: "verificacao", title: "Verificacao", subtitle: "Confianca" },
  { id: "formatacao", title: "Formatacao", subtitle: "Comentario" },
  { id: "publicacao", title: "Publicacao", subtitle: "Gitea" },
];

function WorkflowNode({ data, selected }: NodeProps<Node<StageNodeData>>) {
  const status = data.status || "pending";
  const expanded = selected || data.active || status === "failed";
  const recentEvents = data.events.slice(-4).reverse();

  return (
    <div className={`workflow-node ${status}${expanded ? " expanded" : ""}`}>
      <Handle type="target" position={Position.Left} />
      <div className="workflow-node-head">
        <span className="workflow-node-kicker">{statusLabel(status)}</span>
        <span>{data.percent || 0}%</span>
      </div>
      <strong>{data.title}</strong>
      <small>{data.message || data.subtitle}</small>
      <div className="workflow-node-progress">
        <span style={{ width: `${data.percent || 0}%` }} />
      </div>
      {expanded && (
        <div className="workflow-node-log">
          {recentEvents.length ? (
            recentEvents.map((event, index) => (
              <div key={`${event.timestamp}-${index}`} className={event.status}>
                <header>
                  <span>{event.message || statusLabel(event.status)}</span>
                  <time>
                    {event.timestamp
                      ? new Date(event.timestamp).toLocaleTimeString("pt-BR")
                      : "-"}
                  </time>
                </header>
                {event.group_id && (
                  <p>
                    Grupo {event.group_index || "-"}/{event.total_groups || "-"}: {event.group_id}
                  </p>
                )}
                {!!event.files?.length && (
                  <ul>
                    {event.files.slice(0, 5).map((file) => (
                      <li key={file}>{file}</li>
                    ))}
                    {event.files.length > 5 && (
                      <li>+{event.files.length - 5} arquivos</li>
                    )}
                  </ul>
                )}
                {(event.findings !== undefined || event.failed_groups !== undefined) && (
                  <footer>
                    {event.findings !== undefined && (
                      <span>{event.findings} achados</span>
                    )}
                    {event.failed_groups !== undefined && (
                      <span>{event.failed_groups} grupos falharam</span>
                    )}
                  </footer>
                )}
              </div>
            ))
          ) : (
            <div className="workflow-node-empty">Clique para acompanhar os logs desta etapa.</div>
          )}
        </div>
      )}
      <Handle type="source" position={Position.Right} />
    </div>
  );
}

function statusLabel(status: string) {
  if (status === "done") return "Concluido";
  if (status === "running") return "Rodando";
  if (status === "failed") return "Falhou";
  return "Aguardando";
}

function stageName(stage: string) {
  return workflowStages.find((item) => item.id === stage)?.title || stage;
}

function latestByStage(events: ProgressEvent[]) {
  return events.reduce<Record<string, ProgressEvent>>((acc, event) => {
    acc[event.stage] = event;
    return acc;
  }, {});
}

function eventsByStage(events: ProgressEvent[]) {
  return events.reduce<Record<string, ProgressEvent[]>>((acc, event) => {
    acc[event.stage] = [...(acc[event.stage] || []), event];
    return acc;
  }, {});
}

const nodeTypes = { workflow: WorkflowNode };

export function ObservabilityPage({ request }: { request: AdminRequest }) {
  const [metrics, setMetrics] = useState<Metrics>(empty),
    [reviews, setReviews] = useState<ReviewLog[]>([]),
    [progress, setProgress] = useState<ProgressResponse>({ active: false }),
    [error, setError] = useState(""),
    [updated, setUpdated] = useState<Date | null>(null);
  const [nodes, setNodes, onNodesChange] = useNodesState<Node<StageNodeData>>([]);

  const load = useCallback(async () => {
    try {
      const [metricResult, reviewResult, progressResult] = await Promise.all([
        request("observability/metrics"),
        request("observability/reviews"),
        request("observability/progress"),
      ]);
      setMetrics(metricResult);
      setReviews(reviewResult || []);
      setProgress(progressResult || { active: false });
      setUpdated(new Date());
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [request]);

  useEffect(() => {
    load();
    const timer = window.setInterval(load, 5000);
    return () => window.clearInterval(timer);
  }, [load]);

  const workers = metrics.queue.workers || [];
  const current = progress.review;
  const events = current?.events || emptyEvents;
  const latest = useMemo(() => latestByStage(events), [events]);
  const groupedEvents = useMemo(() => eventsByStage(events), [events]);
  const generatedNodes: Node<StageNodeData>[] = useMemo(
    () =>
      workflowStages.map((stage, index) => {
        const event = latest[stage.id];
        return {
          id: stage.id,
          type: "workflow",
          position: { x: (index % 4) * 340, y: Math.floor(index / 4) * 260 },
          data: {
            title: stage.title,
            subtitle: stage.subtitle,
            status: event?.status || "pending",
            percent:
              event?.status === "done"
                ? 100
                : event?.status === "running"
                  ? 62
                  : 0,
            message: event?.message,
            events: groupedEvents[stage.id] || [],
            active: current?.stage === stage.id,
          },
        };
      }),
    [current?.stage, groupedEvents, latest],
  );
  const edges: Edge[] = useMemo(
    () =>
      workflowStages.slice(0, -1).map((stage, index) => {
        const target = workflowStages[index + 1];
        return {
          id: `${stage.id}-${target.id}`,
          source: stage.id,
          target: target.id,
          animated: target.id === current?.stage,
          className: latest[target.id] ? "workflow-edge active" : "workflow-edge",
        };
      }),
    [current?.stage, latest],
  );

  useEffect(() => {
    setNodes((previous) =>
      generatedNodes.map((node) => {
        const currentNode = previous.find((item) => item.id === node.id);
        return {
          ...node,
          position: currentNode?.position || node.position,
          selected: currentNode?.selected,
        };
      }),
    );
  }, [generatedNodes, setNodes]);

  return (
    <section className="observability-page">
      {error && <div className="wizard-error">{error}</div>}
      <div className="workflow-hero">
        <article>
          <div>
            <span className="section-kicker">Workflow de review</span>
            <h2>{current ? current.name : "Nenhum review em execucao"}</h2>
            <p>
              {current?.message ||
                "Quando um job iniciar, o progresso aparece aqui em tempo real."}
            </p>
          </div>
          <strong>{current?.percent || 0}%</strong>
        </article>
        <article>
          <small>Etapa atual</small>
          <strong>{current ? stageName(current.stage) : "Aguardando"}</strong>
        </article>
        <article>
          <small>Jobs pendentes</small>
          <strong>{metrics.queue.pending || 0}</strong>
        </article>
        <article>
          <small>Workers ativos</small>
          <strong>
            {workers.filter((worker) => worker.state === "processing").length}
          </strong>
        </article>
      </div>
      <article className="ops-panel workflow-canvas-panel">
        <header>
          <div>
            <span className="section-kicker">Tempo real</span>
            <h3>Pipeline visual</h3>
          </div>
          <div className="workflow-canvas-actions">
            <span className="live-badge">
              <i /> a cada 5s
            </span>
            <button className="button button-secondary" onClick={load}>
              Atualizar
            </button>
          </div>
        </header>
        <div className="workflow-canvas">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            onNodesChange={onNodesChange}
            fitView
            nodesDraggable
            nodesConnectable={false}
            elementsSelectable
          >
            <Background gap={22} size={1} />
          </ReactFlow>
        </div>
      </article>
      <footer className="workflow-footer">
        <span>
          Redis: {metrics.queue.connected ? "operacional" : "indisponivel"}
        </span>
        <span>{metrics.queue.stream_length || 0} jobs na stream</span>
        <span>{metrics.reviews.total || 0} reviews com logs</span>
        <span>{size(metrics.reviews.bytes || 0)} armazenados</span>
        <span>Atualizado: {updated?.toLocaleTimeString("pt-BR") || "-"}</span>
        {reviews[0] && <span>Ultimo artefato: {reviews[0].name}</span>}
      </footer>
    </section>
  );
}
