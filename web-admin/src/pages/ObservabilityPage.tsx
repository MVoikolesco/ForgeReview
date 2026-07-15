import { useCallback, useEffect, useState } from "react";
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
const empty: Metrics = {
  queue: { connected: false, stream_length: 0, pending: 0, workers: [] },
  reviews: { total: 0, bytes: 0 },
};
const size = (value: number) =>
  value > 1024 * 1024
    ? `${(value / 1024 / 1024).toFixed(1)} MB`
    : `${Math.ceil(value / 1024)} KB`;

export function ObservabilityPage({ request }: { request: AdminRequest }) {
  const [metrics, setMetrics] = useState<Metrics>(empty),
    [lines, setLines] = useState<string[]>([]),
    [reviews, setReviews] = useState<ReviewLog[]>([]),
    [error, setError] = useState(""),
    [updated, setUpdated] = useState<Date | null>(null);
  const load = useCallback(async () => {
    try {
      const [metricResult, logResult, reviewResult] = await Promise.all([
        request("observability/metrics"),
        request("observability/logs?lines=250"),
        request("observability/reviews"),
      ]);
      setMetrics(metricResult);
      setLines(logResult.lines || []);
      setReviews(reviewResult || []);
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
  return (
    <section className="observability-page">
      {error && <div className="wizard-error">{error}</div>}
      <div className="ops-summary">
        <article>
          <span
            className={`ops-light ${metrics.queue.connected ? "online" : ""}`}
          />
          <div>
            <small>Redis e fila</small>
            <strong>
              {metrics.queue.connected ? "Operacional" : "Indisponível"}
            </strong>
          </div>
        </article>
        <article>
          <small>Jobs na stream</small>
          <strong>{metrics.queue.stream_length || 0}</strong>
        </article>
        <article>
          <small>Jobs pendentes</small>
          <strong>{metrics.queue.pending || 0}</strong>
        </article>
        <article>
          <small>Reviews com logs</small>
          <strong>{metrics.reviews.total || 0}</strong>
        </article>
      </div>
      <div className="ops-grid">
        <article className="ops-panel worker-panel">
          <header>
            <div>
              <span className="section-kicker">Processamento</span>
              <h3>Workers</h3>
            </div>
            <button className="button button-secondary" onClick={load}>
              Atualizar
            </button>
          </header>
          {!workers.length ? (
            <div className="ops-empty">
              Nenhum heartbeat de worker encontrado.
            </div>
          ) : (
            workers.map((worker) => (
              <div className="worker-row" key={worker.name}>
                <span className={`worker-state ${worker.state}`} />
                <div>
                  <strong>{worker.name}</strong>
                  <small>
                    {worker.current_job ||
                      `${worker.state === "processing" ? "Processando" : "Aguardando jobs"}`}
                  </small>
                </div>
                <dl>
                  <div>
                    <dt>Concluídos</dt>
                    <dd>{worker.processed || 0}</dd>
                  </div>
                  <div>
                    <dt>Falhas</dt>
                    <dd>{worker.failed || 0}</dd>
                  </div>
                </dl>
              </div>
            ))
          )}
        </article>
        <article className="ops-panel review-log-panel">
          <header>
            <div>
              <span className="section-kicker">Artefatos</span>
              <h3>Reviews recentes</h3>
            </div>
            <small>{size(metrics.reviews.bytes || 0)}</small>
          </header>
          <div className="review-log-list">
            {reviews.length ? (
              reviews.slice(0, 8).map((review) => (
                <div key={review.name}>
                  <span className="review-log-icon">PR</span>
                  <div>
                    <strong>{review.name}</strong>
                    <small>
                      {review.files} arquivos · {size(review.bytes)}
                    </small>
                  </div>
                  <time>
                    {review.updated_at
                      ? new Date(review.updated_at).toLocaleString("pt-BR")
                      : "—"}
                  </time>
                </div>
              ))
            ) : (
              <div className="ops-empty">Nenhum review registrado.</div>
            )}
          </div>
        </article>
      </div>
      <article className="ops-panel console-panel">
        <header>
          <div>
            <span className="section-kicker">Tempo real</span>
            <h3>Log do worker</h3>
          </div>
          <span className="live-badge">
            <i /> atualização a cada 5s
          </span>
        </header>
        <pre>
          {lines.length
            ? lines.join("\n")
            : "O arquivo de log será criado quando o worker iniciar."}
        </pre>
        <footer>
          Última atualização: {updated?.toLocaleTimeString("pt-BR") || "—"}
        </footer>
      </article>
    </section>
  );
}
