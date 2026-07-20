"use client";

import type { AdminRequest } from "@/lib/admin-client";
import { ChevronDown, Loader2, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

type SummaryLog = {
  status: string;
  percent: number;
  message?: string;
  timestamp?: string;
  duration_ms?: number;
  error?: string;
  group_id?: string;
  group_index?: number;
  total_groups?: number;
  attempt?: number;
  max_attempts?: number;
  files?: string[];
  findings?: number;
  failed_groups?: number;
};

type DetailedEntry = {
  id: number;
  attempt: number;
  status: string;
  artifact_type?: string;
  duration_ms?: number;
  error?: string;
  group_id?: string;
  group_index?: number;
  total_groups?: number;
  prompt?: string;
  response?: string;
  provider?: string;
  model?: string;
  input_chars?: number;
  response_chars?: number;
  requested_output_tokens?: number;
  actual_prompt_tokens?: number;
  actual_completion_tokens?: number;
  files?: string[];
  started_at?: string;
  finished_at?: string;
  metadata?: Record<string, unknown>;
  artifacts?: { type: string; payload?: unknown }[];
};

type DetailedResponse = {
  name: string;
  stage: string;
  entries: DetailedEntry[];
};

function statusLabel(status: string) {
  if (status === "done" || status === "completed") return "Concluída";
  if (status === "running") return "Em execução";
  if (status === "retrying") return "Retentando";
  if (status === "failed") return "Falhou";
  if (status === "waiting") return "Aguardando autorização";
  return status || "Aguardando";
}

function formatTime(timestamp?: string) {
  if (!timestamp) return "--:--:--";
  return new Date(timestamp).toLocaleTimeString("pt-BR");
}

function formatDuration(duration?: number) {
  if (!duration || duration < 1) return "";
  return `${(duration / 1000).toFixed(2)}s`;
}

function titleForAttempt(entry: DetailedEntry) {
  if (entry.group_id) {
    return `Grupo ${entry.group_id} · tentativa ${entry.attempt || 0}`;
  }
  if (entry.attempt > 0) return `Tentativa ${entry.attempt}`;
  return "Execução da etapa";
}

export function StageLogsModal({
  request,
  reviewName,
  stage,
  title,
  subtitle,
  summaryLogs,
  onAuthError,
  onClose,
}: {
  request: AdminRequest;
  reviewName: string;
  stage: string;
  title: string;
  subtitle: string;
  summaryLogs: SummaryLog[];
  onAuthError: () => void;
  onClose: () => void;
}) {
  const [details, setDetails] = useState<DetailedResponse | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setError("");
      try {
        const response = await request<DetailedResponse>(
          `observability/stage-logs?name=${encodeURIComponent(reviewName)}&stage=${encodeURIComponent(stage)}`,
        );
        if (!cancelled) setDetails(response);
      } catch (failure) {
        if (failure instanceof Error && failure.message === "AUTH") {
          onAuthError();
          return;
        }
        if (!cancelled) {
          setError(failure instanceof Error ? failure.message : String(failure));
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [onAuthError, request, reviewName, stage]);

  const groupedEntries = useMemo(() => {
    const groups = new Map<string, DetailedEntry[]>();
    for (const entry of details?.entries || []) {
      const key = entry.group_id || "__stage__";
      groups.set(key, [...(groups.get(key) || []), entry]);
    }
    return [...groups.entries()];
  }, [details?.entries]);

  return (
    <div
      className="stage-logs-backdrop"
      role="presentation"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <section
        className="stage-logs-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="stage-logs-title"
      >
        <header>
          <div>
            <span className="eyebrow">Diagnóstico da etapa</span>
            <h2 id="stage-logs-title">{title}</h2>
            <p>{subtitle}</p>
          </div>
          <button className="icon-button" onClick={onClose} aria-label="Fechar logs">
            <X size={18} />
          </button>
        </header>

        <div className="stage-logs-content">
          <section className="stage-logs-section">
            <header>
              <h3>Resumo da etapa</h3>
              <small>{summaryLogs.length} evento(s)</small>
            </header>
            {summaryLogs.length === 0 ? (
              <div className="stage-logs-empty">Nenhum evento resumido registrado.</div>
            ) : (
              <div className="stage-log-summary-list">
                {summaryLogs.map((log, index) => (
                  <article className={`stage-log-summary ${log.status}`} key={`${log.timestamp || "summary"}-${index}`}>
                    <div>
                      <strong>{log.message || statusLabel(log.status)}</strong>
                      <small>{statusLabel(log.status)} · {log.percent}%</small>
                    </div>
                    <time>{formatTime(log.timestamp)}</time>
                  </article>
                ))}
              </div>
            )}
          </section>

          <section className="stage-logs-section">
            <header>
              <h3>Tentativas e grupos</h3>
              <small>{details?.entries.length || 0} registro(s) detalhados</small>
            </header>
            {loading ? (
              <div className="stage-logs-loading">
                <Loader2 className="spin" size={20} /> Carregando detalhes da etapa...
              </div>
            ) : error ? (
              <div className="banner error">{error}</div>
            ) : groupedEntries.length === 0 ? (
              <div className="stage-logs-empty">Nenhum detalhe adicional foi persistido para esta etapa.</div>
            ) : (
              groupedEntries.map(([groupKey, entries]) => (
                <section className="stage-log-group" key={groupKey}>
                  <header>
                    <h4>{groupKey === "__stage__" ? "Etapa" : `Grupo ${groupKey}`}</h4>
                    <small>{entries.length} tentativa(s)</small>
                  </header>
                  {entries.map((entry) => (
                    <details className={`stage-log-detail ${entry.status}`} key={entry.id} open={entry.status === "failed"}>
                      <summary>
                        <div>
                          <strong>{titleForAttempt(entry)}</strong>
                          <small>
                            {statusLabel(entry.status)}
                            {formatDuration(entry.duration_ms) ? ` · ${formatDuration(entry.duration_ms)}` : ""}
                            {entry.provider ? ` · ${entry.provider}` : ""}
                            {entry.model ? ` / ${entry.model}` : ""}
                          </small>
                        </div>
                        <div className="stage-log-summary-meta">
                          <time>{formatTime(entry.started_at)}</time>
                          <ChevronDown size={16} />
                        </div>
                      </summary>
                      <div className="stage-log-body">
                        {(entry.group_id || entry.files?.length || entry.input_chars || entry.response_chars || entry.actual_prompt_tokens || entry.actual_completion_tokens) && (
                          <div className="stage-log-facts">
                            {entry.group_id && <span>Grupo {entry.group_index || "-"}/{entry.total_groups || "-"} · {entry.group_id}</span>}
                            {!!entry.files?.length && <span>{entry.files.length} arquivo(s)</span>}
                            {entry.input_chars ? <span>{entry.input_chars} chars prompt</span> : null}
                            {entry.response_chars ? <span>{entry.response_chars} chars resposta</span> : null}
                            {entry.actual_prompt_tokens ? <span>{entry.actual_prompt_tokens} tokens prompt</span> : null}
                            {entry.actual_completion_tokens ? <span>{entry.actual_completion_tokens} tokens resposta</span> : null}
                          </div>
                        )}

                        {entry.error && <pre className="stage-log-error">{entry.error}</pre>}

                        {entry.prompt && (
                          <article className="stage-log-panel">
                            <h5>Prompt enviado</h5>
                            <pre>{entry.prompt}</pre>
                          </article>
                        )}

                        {entry.response && (
                          <article className="stage-log-panel">
                            <h5>Resposta recebida</h5>
                            <pre>{entry.response}</pre>
                          </article>
                        )}

                        {!!entry.artifacts?.length && (
                          <article className="stage-log-panel">
                            <h5>Artefatos persistidos</h5>
                            {entry.artifacts.map((artifact, index) => (
                              <details className="stage-log-artifact" key={`${artifact.type}-${index}`}>
                                <summary>{artifact.type}</summary>
                                <pre>{JSON.stringify(artifact.payload, null, 2)}</pre>
                              </details>
                            ))}
                          </article>
                        )}
                      </div>
                    </details>
                  ))}
                </section>
              ))
            )}
          </section>
        </div>

        <footer>
          <span>Review {reviewName}</span>
          <button className="secondary-button" onClick={onClose}>Fechar</button>
        </footer>
      </section>
    </div>
  );
}
