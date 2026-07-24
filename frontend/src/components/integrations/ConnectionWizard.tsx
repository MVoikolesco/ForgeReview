"use client";

import {
  Check,
  ChevronLeft,
  ChevronRight,
  Cloud,
  Cpu,
  LoaderCircle,
  Server,
  Settings2,
} from "lucide-react";
import { useState } from "react";
import {
  createIntegration,
  discoverCandidateRepositories,
  replaceModels,
  replaceRepositories,
  validateIntegration,
} from "../../lib/api";
import type { Integration, NewIntegration, Repository } from "../../lib/types";
import { ModalShell } from "../common/ModalShell";
import { ToggleSwitch } from "../common/ToggleSwitch";
import styles from "./ConnectionWizard.module.scss";

type Family = "gitea" | "llm";
type Provider = "ollama-local" | "ollama-cloud" | "openrouter";
const providerURL: Record<Provider, string> = {
  "ollama-local": "http://host.docker.internal:11434",
  "ollama-cloud": "https://ollama.com",
  openrouter: "https://openrouter.ai/api/v1",
};

export function ConnectionWizard({
  items,
  onClose,
  onCreated,
}: {
  items: Integration[];
  onClose: () => void;
  onCreated: (item: Integration) => void;
}) {
  const [step, setStep] = useState(0);
  const [family, setFamily] = useState<Family>("gitea");
  const [provider, setProvider] = useState<Provider>("ollama-local");
  const [key, setKey] = useState("");
  const [name, setName] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [secret, setSecret] = useState("");
  const [organizations, setOrganizations] = useState<string[]>([]);
  const [organization, setOrganization] = useState("");
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [models, setModels] = useState<string[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState<
    "validation" | "repositories" | "saving" | null
  >(null);
  const isGitea = family === "gitea";
  const steps = isGitea
    ? [
        ["Tipo", "Escolha uma integração"],
        ["Conexão", "URL e segredo"],
        ["Organização", "Escolha a origem"],
        ["Repositórios", "Selecione recursos"],
      ]
    : [
        ["Tipo", "Escolha uma integração"],
        ["Conexão", "URL e segredo"],
        ["Modelos", "Selecione recursos"],
      ];
  const candidate = (): NewIntegration => ({
    key,
    name,
    secret,
    status: "active",
    config: { base_url: baseURL },
    type: isGitea ? "gitea" : provider === "openrouter" ? "openai" : "ollama",
  });
  const select = (value: string) =>
    setSelected((current) =>
      current.includes(value)
        ? current.filter((item) => item !== value)
        : [...current, value],
    );
  const choose = (nextFamily: Family, nextProvider?: Provider) => {
    setFamily(nextFamily);
    setProvider(nextProvider ?? "ollama-local");
    setBaseURL(nextProvider ? providerURL[nextProvider] : "");
    setStep(1);
    setError("");
    setSelected([]);
    setModels([]);
    setRepositories([]);
    setOrganizations([]);
    setOrganization("");
  };
  const validate = async () => {
    if (!key || !name || !baseURL || !secret) {
      setError("Preencha identificação, URL e Token/API key para continuar.");
      return;
    }
    setBusy("validation");
    setError("");
    try {
      const result = await validateIntegration(candidate());
      if (isGitea) {
        setOrganizations(result.organizations ?? []);
        setStep(2);
      } else {
        setModels(result.models ?? []);
        setSelected([]);
        setStep(2);
      }
    } catch (failure) {
      setError(
        failure instanceof Error
          ? failure.message
          : "Não foi possível validar a conexão.",
      );
    } finally {
      setBusy(null);
    }
  };
  const loadRepositories = async () => {
    if (!organization) {
      setError("Selecione uma organização para continuar.");
      return;
    }
    setBusy("repositories");
    setError("");
    try {
      const result = await discoverCandidateRepositories(
        candidate(),
        organization,
      );
      setRepositories(result.repositories ?? []);
      setSelected([]);
      setStep(3);
    } catch (failure) {
      setError(
        failure instanceof Error
          ? failure.message
          : "Não foi possível carregar os repositórios.",
      );
    } finally {
      setBusy(null);
    }
  };
  const submit = async () => {
    if (!selected.length) {
      setError(
        isGitea
          ? "Selecione ao menos um repositório."
          : "Selecione ao menos um modelo.",
      );
      return;
    }
    setBusy("saving");
    setError("");
    try {
      const integration = await createIntegration(candidate());
      if (isGitea)
        await replaceRepositories(
          integration.key,
          repositories.filter((repo) =>
            selected.includes(`${repo.owner}/${repo.name}`),
          ),
        );
      else await replaceModels(integration.key, selected);
      setSecret("");
      onCreated(integration);
      onClose();
    } catch (failure) {
      setError(
        failure instanceof Error
          ? failure.message
          : "Não foi possível salvar a conexão.",
      );
    } finally {
      setBusy(null);
    }
  };
  const available = (
    isGitea
      ? repositories.map((repo) => ({
          value: `${repo.owner}/${repo.name}`,
          label: `${repo.owner}/${repo.name}`,
        }))
      : models.map((model) => ({ value: model, label: model }))
  ).filter((item) =>
    item.label.toLocaleLowerCase().includes(query.toLocaleLowerCase()),
  );
  const continuing =
    step === 1 ? validate : step === 2 && isGitea ? loadRepositories : submit;
  const busyLabel =
    busy === "validation"
      ? "Validando conexão…"
      : busy === "repositories"
        ? "Carregando repositórios…"
        : "Salvando…";

  return (
    <div className={styles.wizard}>
      <aside className={styles.rail} aria-label="Etapas da conexão">
        <div>
          <span className={styles.mark}>
            <Settings2 size={17} />
          </span>
          <strong>Conexões</strong>
          <p>Integrações reutilizáveis para todos os workflows.</p>
        </div>
        <ol>
          {steps.map(([title, subtitle], index) => (
            <li
              key={title}
              className={
                index === step
                  ? styles.active
                  : index < step
                    ? styles.complete
                    : ""
              }
            >
              <span>{index < step ? <Check size={14} /> : index + 1}</span>
              <div>
                <strong>{title}</strong>
                <small>{subtitle}</small>
              </div>
              {index === step && <ChevronRight size={15} />}
            </li>
          ))}
        </ol>
        <small>
          O Token/API key é enviado uma vez e não é armazenado no navegador.
        </small>
      </aside>
      <ModalShell
        title={
          step === 0
            ? "O que deseja conectar?"
            : step === 1
              ? "Configure o acesso"
              : isGitea && step === 2
                ? "Selecione a organização"
                : isGitea
                  ? "Selecione repositórios"
                  : "Selecione modelos"
        }
        eyebrow="NOVA CONEXÃO"
        description={
          isGitea
            ? "Valide o acesso antes de escolher uma organização e seus repositórios."
            : "A conexão protege o acesso; os modelos descobertos serão perfis reutilizáveis."
        }
        onClose={onClose}
        className={styles.modal}
        footer={
          <>
            <span>
              Etapa {step + 1} de {steps.length}
            </span>
            <div className={styles.actions}>
              {step > 0 && (
                <button
                  type="button"
                  disabled={!!busy}
                  onClick={() => {
                    setStep(step - 1);
                    setError("");
                  }}
                >
                  <ChevronLeft size={15} /> Voltar
                </button>
              )}
              {step > 0 && (
                <button
                  className={styles.primary}
                  type="button"
                  disabled={
                    !!busy || (step === 2 && !isGitea && !selected.length)
                  }
                  onClick={() => void continuing()}
                >
                  {busy ? (
                    <>
                      <LoaderCircle className={styles.spinner} size={15} />{" "}
                      {busyLabel}
                    </>
                  ) : step === steps.length - 1 ? (
                    "Concluir conexão"
                  ) : (
                    "Continuar"
                  )}{" "}
                  {!busy && <ChevronRight size={15} />}
                </button>
              )}
            </div>
          </>
        }
      >
        <div className={styles.body}>
          {step === 0 && (
            <div className={styles.choices}>
              <button type="button" onClick={() => choose("gitea")}>
                <Server size={22} />
                <strong>Gitea</strong>
                <span>Pull requests, diff, comentários e publicação.</span>
              </button>
              <button
                type="button"
                onClick={() => choose("llm", "ollama-local")}
              >
                <Cpu size={22} />
                <strong>LLMs</strong>
                <span>Ollama local/cloud ou OpenRouter.</span>
              </button>
              <div className={styles.existing}>
                <h3>Conexões existentes</h3>
                {items.length ? (
                  items.map((item) => (
                    <article key={item.key}>
                      <i />
                      <strong>{item.name}</strong>
                      <small>{item.type === "gitea" ? "Gitea" : "LLM"}</small>
                    </article>
                  ))
                ) : (
                  <p>Nenhuma conexão cadastrada.</p>
                )}
              </div>
            </div>
          )}
          {step === 1 && (
            <div className={styles.form}>
              {!isGitea && (
                <fieldset>
                  <legend>Provider</legend>
                  {(
                    ["ollama-local", "ollama-cloud", "openrouter"] as Provider[]
                  ).map((item) => (
                    <button
                      key={item}
                      type="button"
                      className={provider === item ? styles.selected : ""}
                      onClick={() => {
                        setProvider(item);
                        setBaseURL(providerURL[item]);
                      }}
                    >
                      {item === "ollama-local" ? (
                        <Cpu size={16} />
                      ) : (
                        <Cloud size={16} />
                      )}
                      {item === "ollama-local"
                        ? "Ollama local"
                        : item === "ollama-cloud"
                          ? "Ollama Cloud"
                          : "OpenRouter"}
                    </button>
                  ))}
                </fieldset>
              )}
              <label>
                Identificador
                <input
                  value={key}
                  onChange={(event) => setKey(event.target.value)}
                  placeholder={isGitea ? "gitea-principal" : "modelo-review"}
                  pattern="[A-Za-z0-9_-]+"
                />
              </label>
              <label>
                Nome de exibição
                <input
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder={
                    isGitea ? "Gitea da engenharia" : "Modelo de revisão"
                  }
                />
              </label>
              <label>
                URL base
                <input
                  type="url"
                  value={baseURL}
                  onChange={(event) => setBaseURL(event.target.value)}
                  placeholder="https://..."
                />
              </label>
              <label>
                Token ou API key
                <input
                  type="password"
                  value={secret}
                  onChange={(event) => setSecret(event.target.value)}
                  placeholder="Cole o Token/API key"
                  autoComplete="new-password"
                />
                <small>
                  Enviado apenas nesta criação; não é salvo no navegador nem
                  exibido novamente.
                </small>
              </label>
            </div>
          )}
          {isGitea && step === 2 && (
            <div className={styles.form}>
              <p className={styles.hint}>
                Conexão validada. Os repositórios serão carregados apenas para a
                organização escolhida.
              </p>
              {organizations.length ? (
                <label>
                  Organização
                  <select
                    value={organization}
                    onChange={(event) => setOrganization(event.target.value)}
                  >
                    <option value="">Selecione uma organização</option>
                    {organizations.map((item) => (
                      <option key={item} value={item}>
                        {item}
                      </option>
                    ))}
                  </select>
                </label>
              ) : (
                <p className={styles.empty}>
                  Nenhuma organização foi encontrada para esta conta.
                </p>
              )}
            </div>
          )}
          {((isGitea && step === 3) || (!isGitea && step === 2)) && (
            <div className={styles.resources}>
              <label>
                Buscar {isGitea ? "repositórios" : "modelos"}
                <input
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="Filtrar resultados"
                  type="search"
                />
              </label>
              <p className={styles.hint}>
                {isGitea
                  ? `Organização: ${organization}`
                  : "Modelos descobertos na conexão validada."}{" "}
                Selecione um ou mais recursos.
              </p>
              <strong className={styles.selectionCount}>{selected.length} selecionado{selected.length === 1 ? "" : "s"}</strong>
              <div
                className={styles.resourceList}
                aria-label={
                  isGitea ? "Repositórios disponíveis" : "Modelos disponíveis"
                }
              >
                {available.length ? (
                  available.map((item) => (
                    <ToggleSwitch
                      key={item.value}
                      variant="card"
                      checked={selected.includes(item.value)}
                      onChange={() => select(item.value)}
                      label={item.label}
                      description={isGitea ? `Repositório · ${organization}` : `${provider === "openrouter" ? "OpenRouter" : "Ollama"} · modelo`}
                      leading={isGitea ? <Server size={16} /> : <Cpu size={16} />}
                    />
                  ))
                ) : (
                  <p className={styles.empty}>
                    Nenhum recurso corresponde à busca.
                  </p>
                )}
              </div>
            </div>
          )}
          {error && (
            <p className={styles.error} role="alert">
              {error}
            </p>
          )}
        </div>
      </ModalShell>
    </div>
  );
}
