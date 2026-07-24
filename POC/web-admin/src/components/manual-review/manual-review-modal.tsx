"use client";

import { StepperModal } from "@/components/stepper/stepper-modal";
import type { AdminRequest } from "@/lib/admin-client";
import type {
  GiteaInstance,
  GiteaOrganization,
  GiteaPullRequest,
  GiteaRepository,
} from "@/lib/contracts";
import { ArrowLeft, ArrowRight, Github, Loader2, Play } from "lucide-react";
import { useEffect, useState } from "react";

const steps = [
  ["Instância", "Escolha o Gitea"],
  ["Organização", "Escolha o espaço"],
  ["Pull request", "Selecione o PR"],
] as const;

export function ManualReviewModal({
  request,
  onClose,
  onAuthError,
  onSuccess,
  onFailure,
}: {
  request: AdminRequest;
  onClose: () => void;
  onAuthError: () => void;
  onSuccess: (message: string) => void;
  onFailure: (message: string) => void;
}) {
  const [step, setStep] = useState(0);
  const [instances, setInstances] = useState<GiteaInstance[]>([]);
  const [instance, setInstance] = useState<GiteaInstance | null>(null);
  const [orgs, setOrgs] = useState<GiteaOrganization[]>([]);
  const [organization, setOrganization] = useState("");
  const [repos, setRepos] = useState<GiteaRepository[]>([]);
  const [repo, setRepo] = useState<GiteaRepository | null>(null);
  const [prs, setPrs] = useState<GiteaPullRequest[]>([]);
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    void loadInstances();
  }, []);
  async function loadInstances() {
    try {
      const result = await request<GiteaInstance[]>("gitea/instances");
      setInstances(result);
      if (result.length === 1) await chooseInstance(result[0]);
    } catch (failure) {
      if (failure instanceof Error && failure.message === "AUTH") onAuthError();
      else
        setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  }
  async function chooseInstance(value: GiteaInstance) {
    setInstance(value);
    setBusy(true);
    setError("");
    try {
      const result = await request<GiteaOrganization[]>(
        `gitea/instances/${value.id}/organizations`,
        { method: "POST" },
      );
      setOrgs(result);
      setStep(result.length === 1 ? 2 : 1);
      if (result.length === 1) await chooseOrganization(result[0].name);
    } catch (failure) {
      const message = failure instanceof Error ? failure.message : String(failure);
      setError(message);
      onFailure(message);
    } finally {
      setBusy(false);
    }
  }
  async function chooseOrganization(value: string) {
    if (!instance) return;
    setOrganization(value);
    setBusy(true);
    setError("");
    try {
      setRepos(
        await request<GiteaRepository[]>(
          `gitea/instances/${instance.id}/repositories`,
          { method: "POST", body: JSON.stringify({ organization: value }) },
        ),
      );
      setStep(2);
    } catch (failure) {
      const message = failure instanceof Error ? failure.message : String(failure);
      setError(message);
    } finally {
      setBusy(false);
    }
  }
  async function chooseRepo(value: GiteaRepository) {
    if (!instance) return;
    setRepo(value);
    setBusy(true);
    setError("");
    try {
      setPrs(
        await request<GiteaPullRequest[]>(
          `gitea/instances/${instance.id}/pull-requests`,
          {
            method: "POST",
            body: JSON.stringify({
              owner: value.owner.login,
              repo: value.name,
            }),
          },
        ),
      );
      setStep(3);
    } catch (failure) {
      const message = failure instanceof Error ? failure.message : String(failure);
      setError(message);
    } finally {
      setBusy(false);
    }
  }
  async function dispatch(pr: GiteaPullRequest) {
    if (!instance || !repo) return;
    setBusy(true);
    setError("");
    try {
      await request("reviews/manual", {
        method: "POST",
        body: JSON.stringify({
          instance_id: instance.id,
          owner: repo.owner.login,
          repo: repo.name,
          pr_number: pr.number,
        }),
      });
      onSuccess(`Review do PR #${pr.number} enfileirado com sucesso.`);
      onClose();
    } catch (failure) {
      const message = failure instanceof Error ? failure.message : String(failure);
      setError(message);
      onFailure(`Não foi possível disparar o review: ${message}`);
    } finally {
      setBusy(false);
    }
  }
  const labels = [
    "Selecione a instância Gitea",
    "Selecione a organização",
    "Selecione o repositório",
    "Selecione o pull request",
  ];
  return (
    <StepperModal
      className="manual-review-modal"
      steps={steps}
      step={Math.min(step, 2)}
      eyebrow={`Etapa ${Math.min(step, 2) + 1} de 3`}
      title={labels[step]}
      description="Escolha o PR aberto que deve entrar na fila de revisão."
      brand={<Github size={18} />}
      heading="Disparar review manual"
      railDescription="Selecione o alvo sem usar o webhook público."
      footnote="A revisão será enviada à fila autenticada do administrador."
      error={error}
      onClose={onClose}
      footer={
        <footer>
          <button
            className="secondary-button"
            onClick={() => (step ? setStep(step - 1) : onClose())}
            disabled={busy}
          >
            <ArrowLeft size={16} />
            {step ? "Voltar" : "Cancelar"}
          </button>
          <span>Fluxo manual</span>
        </footer>
      }
    >
      {busy && !instances.length ? (
        <div className="manual-review-loading">
          <Loader2 className="spin" size={20} />
          Carregando instâncias…
        </div>
      ) : step === 0 ? (
        <div className="manual-review-list">
          {instances.map((item) => (
            <button
              key={item.id}
              onClick={() => void chooseInstance(item)}
              disabled={busy}
            >
              <strong>{item.name}</strong>
              <small>{item.base_url}</small>
              <ArrowRight size={16} />
            </button>
          ))}
          {!instances.length && (
            <p>
              Nenhuma instância Gitea configurada. Cadastre uma na área Gitea.
            </p>
          )}
        </div>
      ) : step === 1 ? (
        <div className="manual-review-list">
          {orgs.map((item) => (
            <button
              key={item.id}
              onClick={() => void chooseOrganization(item.name)}
              disabled={busy}
            >
              <strong>{item.full_name || item.name}</strong>
              <small>{item.name}</small>
              <ArrowRight size={16} />
            </button>
          ))}
          {!orgs.length && (
            <p>Nenhuma organização disponível para esta conta.</p>
          )}
        </div>
      ) : step === 2 ? (
        <div className="manual-review-list">
          {repos.map((item) => (
            <button
              key={item.id}
              onClick={() => void chooseRepo(item)}
              disabled={busy}
            >
              <strong>{item.full_name}</strong>
              <small>{item.private ? "Privado" : "Público"}</small>
              <ArrowRight size={16} />
            </button>
          ))}
          {!repos.length && <p>Nenhum repositório encontrado.</p>}
        </div>
      ) : (
        <div className="manual-review-list">
          {prs.map((item) => (
            <button
              key={item.number}
              onClick={() => void dispatch(item)}
              disabled={busy}
            >
              <strong>
                #{item.number} · {item.title}
              </strong>
              <small>
                {item.user?.login || "Autor não informado"} · PR aberto
              </small>
              <Play size={16} />
            </button>
          ))}
          {!prs.length && <p>Nenhum pull request aberto neste repositório.</p>}
        </div>
      )}
    </StepperModal>
  );
}
