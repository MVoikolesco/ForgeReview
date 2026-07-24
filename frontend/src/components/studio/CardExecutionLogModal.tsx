"use client";

import type {
  CardData,
  ExecutionCardLog,
  ExecutionEvent,
} from "../../lib/types";
import { ModalShell } from "../common/ModalShell";
import styles from "./CardExecutionLogModal.module.scss";

type Props = {
  executionID: number;
  node: CardData;
  events: ExecutionEvent[];
  entries: ExecutionCardLog[];
  loading: boolean;
  onClose: () => void;
};

const statusLabel: Record<string, string> = {
  running: "Em execução",
  completed: "Concluído",
  failed: "Falhou",
  partial: "Concluído parcialmente",
  cancelled: "Cancelado",
};

const statusText = (status: string) => statusLabel[status] ?? status;

const scopeText = (scope?: string) =>
  !scope || scope === "root" ? "Execução principal" : `Escopo ${scope}`;

const formatDuration = (duration?: number) =>
  duration === undefined
    ? undefined
    : duration < 1000
      ? `${duration} ms`
      : `${(duration / 1000).toFixed(2)} s`;

const factText = (key: string, value: unknown): string | undefined => {
  const labels: Record<string, string> = {
    attempt_count: "Tentativas usadas",
    retry_limit: "Limite de tentativas",
    retry_delay_ms: "Intervalo entre tentativas",
    completed_iterations: "Iterações concluídas",
    failed_iterations: "Iterações com falha",
    max_iterations: "Limite de iterações",
    concurrency: "Concorrência",
    model: "Modelo",
    validation_error: "Motivo da validação",
    prompt_tokens: "Tokens de entrada",
    completion_tokens: "Tokens de saída",
    total_tokens: "Tokens totais",
    error_code: "Código do erro",
    error_policy: "Política para erro",
    error_action: "Ação adotada",
  };
  if (key === "validation_attempts" || key === "provider_calls") {
    if (!Array.isArray(value)) return undefined;
    const calls = value
      .filter(
        (item): item is Record<string, unknown> =>
          !!item && typeof item === "object",
      )
      .map(
        (item) =>
          `Tentativa ${String(item.attempt ?? "?")}: ${statusText(String(item.status ?? "registrada"))}`,
      );
    return calls.length ? calls.join(" · ") : undefined;
  }
  if (!labels[key]) return undefined;
  const shown =
    key === "retry_delay_ms" ? `${String(value)} ms` : String(value);
  return `${labels[key]}: ${shown}`;
};

export function CardExecutionLogModal({
  executionID,
  node,
  events,
  entries,
  loading,
  onClose,
}: Props) {
  const latest = entries.at(-1);
  const failure = entries.find((entry) => entry.status === "failed");
  const conclusion = failure
    ? {
        title: "Este card falhou",
        detail: failure.error || "A execução foi interrompida neste card.",
        status: "failed",
      }
    : latest?.status === "completed"
      ? {
          title: "Este card foi concluído",
          detail: `${entries.length} registro(s) de execução foram concluídos.`,
          status: "completed",
        }
      : latest
        ? {
            title: statusText(latest.status),
            detail:
              "Confira os registros abaixo para acompanhar o processamento.",
            status: latest.status,
          }
        : {
            title: "Ainda não há registro",
            detail: "Este card não foi alcançado nesta execução.",
            status: "empty",
          };

  return (
    <ModalShell
      title={`Execução: ${node.name}`}
      eyebrow="DIAGNÓSTICO DO CARD"
      description={`Execução #${executionID} · ${node.type}`}
      onClose={onClose}
    >
      <section className={styles.content} aria-live="polite">
        {loading ? (
          <p className={styles.muted}>Carregando diagnóstico do card…</p>
        ) : (
          <>
            <div
              className={`${styles.conclusion} ${styles[conclusion.status] ?? ""}`}
            >
              <strong>{conclusion.title}</strong>
              <p>{conclusion.detail}</p>
            </div>

            <section>
              <h3>Linha do tempo</h3>
              {events.length ? (
                <ol className={styles.timeline}>
                  {events.map((event) => (
                    <li key={event.id}>
                      <time dateTime={event.created_at}>
                        {new Date(event.created_at).toLocaleTimeString("pt-BR")}
                      </time>
                      <span
                        className={`${styles.badge} ${styles[event.status] ?? ""}`}
                      >
                        {event.status === "running"
                          ? "Iniciado"
                          : statusText(event.status)}
                      </span>
                      {event.node?.scope_key && (
                        <small>{scopeText(event.node.scope_key)}</small>
                      )}
                    </li>
                  ))}
                </ol>
              ) : (
                <p className={styles.muted}>
                  Nenhum evento foi registrado para este card nesta execução.
                </p>
              )}
            </section>

            {entries.length > 0 && (
              <section>
                <h3>Resultados por execução</h3>
                <div className={styles.entries}>
                  {entries.map((entry) => {
                    const facts = Object.entries(entry.facts ?? {})
                      .map(([key, value]) => factText(key, value))
                      .filter((item): item is string => Boolean(item));
                    return (
                      <details
                        key={entry.id}
                        open={entry.status === "failed"}
                        className={styles[entry.status] ?? ""}
                      >
                        <summary>
                          <span>
                            <strong>{scopeText(entry.scope_key)}</strong>
                            <small>
                              {statusText(entry.status)}
                              {formatDuration(entry.duration_ms)
                                ? ` · ${formatDuration(entry.duration_ms)}`
                                : ""}
                            </small>
                          </span>
                          <time>
                            {entry.started_at
                              ? new Date(entry.started_at).toLocaleTimeString(
                                  "pt-BR",
                                )
                              : "--:--"}
                          </time>
                        </summary>
                        <div className={styles.entryBody}>
                          {entry.error && (
                            <p className={styles.error}>{entry.error}</p>
                          )}
                          {facts.length > 0 && (
                            <div className={styles.facts}>
                              {facts.map((fact) => (
                                <span key={fact}>{fact}</span>
                              ))}
                            </div>
                          )}
                          {entry.inputs !== undefined && (
                            <details className={styles.payload}>
                              <summary>Entrada recebida</summary>
                              <pre>{JSON.stringify(entry.inputs, null, 2)}</pre>
                            </details>
                          )}
                          {entry.outputs !== undefined && (
                            <details className={styles.payload}>
                              <summary>Resposta / saída produzida</summary>
                              <pre>{JSON.stringify(entry.outputs, null, 2)}</pre>
                            </details>
                          )}
                          {!entry.error && facts.length === 0 && entry.inputs === undefined && entry.outputs === undefined && (
                            <p className={styles.muted}>
                              Concluído sem métricas adicionais para este tipo
                              de card.
                            </p>
                          )}
                        </div>
                      </details>
                    );
                  })}
                </div>
              </section>
            )}
            <p className={styles.note}>
              Entradas e respostas são registradas para diagnóstico por sete dias.
              Campos de credenciais são mascarados automaticamente; contadores de tokens não são exibidos.
            </p>
          </>
        )}
      </section>
    </ModalShell>
  );
}
