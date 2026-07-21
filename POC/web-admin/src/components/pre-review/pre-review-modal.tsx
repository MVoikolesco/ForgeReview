"use client";

import type { AdminRequest } from "@/lib/admin-client";
import { Check, Loader2, RotateCcw, X } from "lucide-react";
import { useEffect, useState } from "react";

type PendingReview = {
  name: string;
  job: { owner: string; repo: string; pr_number: number };
  publication: {
    event: string;
    body: string;
    comments: { path: string; new_position: number; body: string }[];
  };
};

export function PreReviewModal({
  request,
  name,
  onClose,
  onAuthError,
  onComplete,
}: {
  request: AdminRequest;
  name: string;
  onClose: () => void;
  onAuthError: () => void;
  onComplete: (message: string) => void;
}) {
  const [pending, setPending] = useState<PendingReview | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    void load();
  }, [name]);

  async function load() {
    try {
      setPending(
        await request<PendingReview>(
          `reviews/pending?name=${encodeURIComponent(name)}`,
        ),
      );
    } catch (failure) {
      if (failure instanceof Error && failure.message === "AUTH")
        return onAuthError();
      setError(failure instanceof Error ? failure.message : String(failure));
    }
  }

  async function act(action: "approve" | "reject" | "rerun") {
    setBusy(true);
    setError("");
    try {
      await request(
        `reviews/pending/${action}?name=${encodeURIComponent(name)}`,
        { method: "POST" },
      );
      onComplete(
        action === "approve"
          ? "Review publicada no Gitea."
          : action === "reject"
            ? "Review negado sem publicação."
            : "Revisão solicitada novamente ao agente.",
      );
      onClose();
    } catch (failure) {
      if (failure instanceof Error && failure.message === "AUTH")
        return onAuthError();
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div
      className="pre-review-backdrop"
      role="presentation"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <section
        className="pre-review-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="pre-review-title"
      >
        <header>
          <div>
            <span className="eyebrow">Autorização manual</span>
            <h2 id="pre-review-title">Pré-publicação</h2>
            <p>
              {pending
                ? `PR #${pending.job.pr_number} · ${pending.job.owner}/${pending.job.repo}`
                : "Carregando a revisão gerada…"}
            </p>
          </div>
          <button
            className="icon-button"
            onClick={onClose}
            aria-label="Fechar revisão"
            disabled={busy}
          >
            <X size={18} />
          </button>
        </header>
        {error && <div className="banner error">{error}</div>}
        {!pending && !error ? (
          <div className="pre-review-loading">
            <Loader2 className="spin" size={22} /> Carregando conteúdo da
            publicação…
          </div>
        ) : (
          pending && (
            <div className="pre-review-content">
              <div className="pre-review-summary">
                <span>
                  Será publicado como{" "}
                  <strong>{pending.publication.event}</strong>
                </span>
                <span>
                  {pending.publication.comments.length} comentário(s) inline
                </span>
              </div>
              <article className="pre-review-body">
                <h3>Comentário principal</h3>
                <pre>{pending.publication.body}</pre>
              </article>
              {pending.publication.comments.length > 0 && (
                <article className="pre-review-comments">
                  <h3>Comentários inline</h3>
                  {pending.publication.comments.map((comment, index) => (
                    <div
                      key={`${comment.path}-${comment.new_position}-${index}`}
                    >
                      <strong>
                        {comment.path}:{comment.new_position}
                      </strong>
                      <p>{comment.body}</p>
                    </div>
                  ))}
                </article>
              )}
            </div>
          )
        )}
        <footer>
          <button
            className="secondary-button"
            onClick={() => void act("reject")}
            disabled={busy || !pending}
          >
            <X size={16} /> Negar e finalizar
          </button>
          <button
            className="secondary-button"
            onClick={() => void act("rerun")}
            disabled={busy || !pending}
          >
            <RotateCcw size={16} /> Solicitar nova revisão
          </button>
          <button
            className="primary-button"
            onClick={() => void act("approve")}
            disabled={busy || !pending}
          >
            <Check size={16} /> Autorizar publicação
          </button>
        </footer>
      </section>
    </div>
  );
}
