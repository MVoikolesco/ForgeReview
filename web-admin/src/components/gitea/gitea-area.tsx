"use client";

import { StepperModal } from "@/components/stepper/stepper-modal";
import type { AdminRequest } from "@/lib/admin-client";
import type {
  GiteaInstance,
  GiteaOrganization,
  GiteaRepository,
} from "@/lib/contracts";
import {
  ArrowLeft,
  ArrowRight,
  CheckCircle2,
  Github,
  Loader2,
  Pencil,
  Plus,
  Save,
  Server,
  Trash2
} from "lucide-react";
import { useEffect, useState } from "react";

type Form = Pick<GiteaInstance, "name" | "base_url" | "bot_username"> & {
  token: string;
};
type SavedRepository = {
  id: number;
  full_name: string;
  owner: string;
  gitea_instance_id?: number;
};
const emptyForm: Form = { name: "", base_url: "", bot_username: "", token: "" };
const steps = [
  ["Conexão", "Valide o acesso"],
  ["Organização", "Escolha o espaço"],
  ["Repositórios", "Defina o escopo"],
] as const;

export function GiteaArea({
  request,
  onAuthError,
}: {
  request: AdminRequest;
  onAuthError: () => void;
}) {
  const [instance, setInstance] = useState<GiteaInstance | null>(null);
  const [repositories, setRepositories] = useState<GiteaRepository[]>([]);
  const [organization, setOrganization] = useState("");
  const [editing, setEditing] = useState(false);
  const [step, setStep] = useState(0);
  const [form, setForm] = useState<Form>(emptyForm);
  const [orgs, setOrgs] = useState<GiteaOrganization[]>([]);
  const [repos, setRepos] = useState<GiteaRepository[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  async function load() {
    try {
      const [instances, savedRepos] = await Promise.all([
        request<GiteaInstance[]>("gitea/instances"),
        request<SavedRepository[]>("repositories"),
      ]);
      const current = instances[0] || null;
      setInstance(current);
      const currentRepos = current
        ? savedRepos.filter((repo) => repo.gitea_instance_id === current.id)
        : [];
      setRepositories(
        currentRepos.map((repo) => ({
          id: repo.id,
          name: repo.full_name.split("/").pop() || repo.full_name,
          full_name: repo.full_name,
          owner: { login: repo.owner },
          private: false,
        })),
      );
      setSelected(currentRepos.map((repo) => repo.full_name));
      setOrganization(currentRepos[0]?.owner || "");
    } catch (failure) {
      if (failure instanceof Error && failure.message === "AUTH") onAuthError();
      else
        setError(failure instanceof Error ? failure.message : String(failure));
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function startNew() {
    setForm(emptyForm);
    setEditing(true);
    setStep(0);
    setOrgs([]);
    setRepos([]);
    setSelected([]);
    setOrganization("");
    setMessage("");
    setError("");
  }
  function startEdit() {
    if (!instance) return;
    setForm({
      name: instance.name,
      base_url: instance.base_url,
      bot_username: instance.bot_username,
      token: "",
    });
    setEditing(true);
    setStep(0);
    setMessage("");
    setError("");
  }
  function update(field: keyof Form, value: string) {
    setForm((current) => ({ ...current, [field]: value }));
    setError("");
    setMessage("");
  }
  async function validateAndSave() {
    setBusy(true);
    setError("");
    try {
      let saved: GiteaInstance;
      if (instance) {
        await request(`gitea/instances/${instance.id}/test`, {
          method: "POST",
          body: JSON.stringify(form),
        });
        saved = await request<GiteaInstance>(`gitea/instances/${instance.id}`, {
          method: "PATCH",
          body: JSON.stringify(form),
        });
      } else {
        await request("gitea/instances/test", {
          method: "POST",
          body: JSON.stringify(form),
        });
        saved = await request<GiteaInstance>("gitea/instances", {
          method: "POST",
          body: JSON.stringify(form),
        });
      }
      setInstance(saved);
      setForm((current) => ({ ...current, token: "" }));
      setStep(1);
      await listOrganizations(saved);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  }
  async function listOrganizations(current = instance) {
    if (!current) return;
    try {
      const result = await request<GiteaOrganization[]>(
        `gitea/instances/${current.id}/organizations`,
        { method: "POST" },
      );
      setOrgs(result);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
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
      setSelected([]);
      setStep(2);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  }
  async function saveRepositories() {
    if (!instance) return;
    setBusy(true);
    setError("");
    try {
      await request(`gitea/instances/${instance.id}/repositories`, {
        method: "POST",
        body: JSON.stringify({
          repositories: selected.map((name) => {
            const repo = repos.find((item) => item.full_name === name)!;
            return {
              owner: repo.owner.login,
              name: repo.name,
              full_name: repo.full_name,
            };
          }),
        }),
      });
      setEditing(false);
      setMessage("Conexão e repositórios atualizados.");
      await load();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    if (
      !instance ||
      !window.confirm(
        `Remover ${instance.name} e seus repositórios? Esta ação não pode ser desfeita.`,
      )
    )
      return;
    setBusy(true);
    setError("");
    try {
      await request(`gitea/instances/${instance.id}`, { method: "DELETE" });
      setInstance(null);
      setRepositories([]);
      setMessage("Conexão removida. Você pode cadastrar outra.");
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  }

  if (editing)
    return (
      <GiteaWizard
        form={form}
        step={step}
        orgs={orgs}
        repos={repos}
        selected={selected}
        organization={organization}
        busy={busy}
        error={error}
        instance={instance}
        update={update}
        onClose={() => setEditing(false)}
        onStep={setStep}
        onValidate={validateAndSave}
        onOrganization={chooseOrganization}
        onToggle={(name) =>
          setSelected((value) =>
            value.includes(name)
              ? value.filter((item) => item !== name)
              : [...value, name],
          )
        }
        onSave={saveRepositories}
      />
    );
  return (
    <section className="gitea-page" aria-labelledby="gitea-connection-title">
      {message && (
        <div className="banner success" role="status">
          <CheckCircle2 size={16} />
          {message}
        </div>
      )}
      {error && (
        <div className="banner error" role="alert">
          {error}
        </div>
      )}
      {!instance ? (
        <div className="gitea-empty">
          <span className="gitea-mark">
            <Github size={25} />
          </span>
          <h2 id="gitea-connection-title">Nenhuma conexão configurada</h2>
          <p>
            Conecte uma instância Gitea para escolher a organização e os
            repositórios monitorados.
          </p>
          <button className="primary-button" onClick={startNew}>
            <Plus size={17} />
            Cadastrar conexão
          </button>
        </div>
      ) : (
        <div className="gitea-card">
          <div className="gitea-card-head">
            <div className="gitea-identity">
              <span className="gitea-mark">
                <Github size={22} />
              </span>
              <div>
                <span className="eyebrow">Conexão Gitea</span>
                <h2 id="gitea-connection-title">{instance.name}</h2>
                <span className="status-chip online">
                  <i />
                  Ativa
                </span>
              </div>
            </div>
            <div className="gitea-card-actions">
              <button
                className="secondary-button"
                onClick={startEdit}
                disabled={busy}
              >
                <Pencil size={16} />
                Editar
              </button>
              <button
                className="danger-button"
                onClick={() => void remove()}
                disabled={busy}
              >
                {busy ? (
                  <Loader2 className="spin" size={16} />
                ) : (
                  <Trash2 size={16} />
                )}
                Remover
              </button>
            </div>
          </div>
          <div className="gitea-facts">
            <div>
              <small>Endpoint</small>
              <strong>{instance.base_url}</strong>
            </div>
            <div>
              <small>Usuário bot</small>
              <strong>{instance.bot_username}</strong>
            </div>
            <div>
              <small>Organização</small>
              <strong>{organization || "Não selecionada"}</strong>
            </div>
          </div>
          <div className="gitea-repositories">
            <div className="section-title">
              <div>
                <h3>Repositórios monitorados</h3>
                <p>
                  Estes repositórios serão enviados para o fluxo de revisão.
                </p>
              </div>
              <span>{repositories.length}</span>
            </div>
            {repositories.length ? (
              <div className="gitea-repo-list">
                {repositories.map((repo) => (
                  <div key={repo.id}>
                    <Server size={15} />
                    <strong>{repo.full_name}</strong>
                    <span>{repo.private ? "Privado" : "Público"}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="gitea-muted">Nenhum repositório selecionado.</p>
            )}
          </div>
        </div>
      )}
    </section>
  );
}

function GiteaWizard({
  form,
  step,
  orgs,
  repos,
  selected,
  organization,
  busy,
  error,
  instance,
  update,
  onClose,
  onStep,
  onValidate,
  onOrganization,
  onToggle,
  onSave,
}: {
  form: Form;
  step: number;
  orgs: GiteaOrganization[];
  repos: GiteaRepository[];
  selected: string[];
  organization: string;
  busy: boolean;
  error: string;
  instance: GiteaInstance | null;
  update: (field: keyof Form, value: string) => void;
  onClose: () => void;
  onStep: (step: number) => void;
  onValidate: () => void;
  onOrganization: (value: string) => void;
  onToggle: (name: string) => void;
  onSave: () => void;
}) {
  return (
    <StepperModal
      className="gitea-wizard"
      steps={steps}
      step={step}
      eyebrow={`Etapa ${step + 1} de ${steps.length}`}
      title={
        [
          "Configure a conexão",
          "Escolha a organização",
          "Selecione os repositórios",
        ][step]
      }
      description={
        [
          "Informe os dados e valide o acesso ao Gitea.",
          "Escolha onde estão os projetos que serão monitorados.",
          "Defina quais repositórios entram no fluxo de revisão.",
        ][step]
      }
      brand={<Github size={18} />}
      heading={instance ? "Editar conexão" : "Nova conexão"}
      railDescription="Configure o acesso e o escopo da integração."
      footnote="O token é armazenado com segurança e nunca é exibido."
      error={error}
      onClose={onClose}
      footer={
        <GiteaFooter
          step={step}
          busy={busy}
          orgs={orgs}
          onBack={() => (step ? onStep(step - 1) : onClose())}
          onNext={
            step === 0 ? onValidate : step === 1 ? () => undefined : onSave
          }
        />
      }
    >
      {step === 0 && (
        <div className="connection-form">
          <div className="form-grid">
            <label className="full">
              Nome da conexão
              <input
                value={form.name}
                onChange={(event) => update("name", event.target.value)}
                placeholder="Gitea principal"
              />
            </label>
            <label>
              Usuário bot
              <input
                value={form.bot_username}
                onChange={(event) => update("bot_username", event.target.value)}
                placeholder="forgereview-bot"
              />
            </label>
            <label>
              URL da instância
              <input
                type="url"
                value={form.base_url}
                onChange={(event) => update("base_url", event.target.value)}
                placeholder="https://gitea.exemplo.com"
              />
            </label>
            <label className="full">
              Token de acesso
              <input
                type="password"
                value={form.token}
                onChange={(event) => update("token", event.target.value)}
                autoComplete="new-password"
                placeholder={
                  instance
                    ? "Deixe vazio para manter o token atual"
                    : "Token do bot"
                }
              />
              <small>
                {instance
                  ? "O token atual será preservado se você não informar outro."
                  : "O token será usado apenas para validar e cadastrar."}
              </small>
            </label>
          </div>
          <div className="validation-note">
            <CheckCircle2 size={17} />
            <div>
              <strong>Validação obrigatória</strong>
              <p>
                O Gitea será consultado antes de qualquer alteração ser salva.
              </p>
            </div>
          </div>
        </div>
      )}
      {step === 1 && (
        <div className="gitea-step-list">
          {!orgs.length && (
            <div className="gitea-muted">
              Nenhuma organização encontrada para este usuário.
            </div>
          )}
          {orgs.map((org) => (
            <button
              key={org.id}
              className={organization === org.name ? "selected" : ""}
              onClick={() => void onOrganization(org.name)}
            >
              <span className="gitea-list-icon">
                <Server size={17} />
              </span>
              <span>
                <strong>{org.full_name || org.name}</strong>
                <small>{org.name}</small>
              </span>
              <ArrowRight size={16} />
            </button>
          ))}
        </div>
      )}
      {step === 2 && (
        <div className="gitea-step-list">
          <div className="gitea-selection-head">
            <span>{repos.length} repositórios encontrados</span>
            <small>{selected.length} selecionados</small>
          </div>
          {repos.map((repo) => (
            <label
              key={repo.id}
              className={selected.includes(repo.full_name) ? "selected" : ""}
            >
              <input
                type="checkbox"
                checked={selected.includes(repo.full_name)}
                onChange={() => onToggle(repo.full_name)}
              />
              <span>
                <strong>{repo.full_name}</strong>
                <small>{repo.private ? "Privado" : "Público"}</small>
              </span>
            </label>
          ))}
        </div>
      )}
    </StepperModal>
  );
}

function GiteaFooter({
  step,
  busy,
  orgs,
  onBack,
  onNext,
}: {
  step: number;
  busy: boolean;
  orgs: GiteaOrganization[];
  onBack: () => void;
  onNext: () => void;
}) {
  return (
    <footer>
      <button className="secondary-button" onClick={onBack} disabled={busy}>
        <ArrowLeft size={17} />
        {step ? "Voltar" : "Cancelar"}
      </button>
      <span>
        Etapa {step + 1} de {steps.length}
      </span>
      <button
        className="primary-button"
        onClick={onNext}
        disabled={busy || (step === 1 && !orgs.length)}
      >
        {busy ? (
          <Loader2 className="spin" size={17} />
        ) : step === 2 ? (
          <Save size={17} />
        ) : (
          <ArrowRight size={17} />
        )}
        {busy ? "Processando..." : step === 2 ? "Salvar seleção" : "Continuar"}
      </button>
    </footer>
  );
}
