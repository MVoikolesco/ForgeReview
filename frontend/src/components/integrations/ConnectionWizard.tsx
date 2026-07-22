"use client";

import {
  Check,
  ChevronLeft,
  ChevronRight,
  Cloud,
  Cpu,
  Server,
  Settings2,
  ShieldCheck,
} from "lucide-react";
import { useState } from "react";
import { createIntegration, createModelProfile } from "../../lib/api";
import type { Integration } from "../../lib/types";
import { ModalShell } from "../common/ModalShell";
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
  const [model, setModel] = useState("");
  const [secret, setSecret] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const steps = [
    ["Tipo", "Escolha Gitea ou LLM"],
    ["Conexão", "URL e segredo"],
    [
      family === "llm" ? "Modelo" : "Revisar",
      family === "llm" ? "Defina o modelo" : "Confirme a conexão",
    ],
  ] as const;
  const choose = (nextFamily: Family, nextProvider?: Provider) => {
    setFamily(nextFamily);
    if (nextProvider) {
      setProvider(nextProvider);
      setBaseURL(providerURL[nextProvider]);
    } else setBaseURL("");
    setStep(1);
    setError("");
  };
  const next = () => {
    if (step === 1 && (!key || !name || !baseURL || !secret)) {
      setError(
        "Preencha identificação, URL e Token/API key para continuar.",
      );
      return;
    }
    setError("");
    setStep(2);
  };
  const submit = async () => {
    setSaving(true);
    setError("");
    try {
      const type =
        family === "gitea"
          ? "gitea"
          : provider === "openrouter"
            ? "openai"
            : "ollama";
      const integration = await createIntegration({
        key,
        name,
        type,
        status: "active",
        secret,
        config: { base_url: baseURL },
      });
      if (family === "llm") {
        await createModelProfile({
          key: `${key}-profile`,
          name: `${name} · ${model}`,
          integration_key: integration.key,
          model,
          status: "active",
        });
      }
      onCreated(integration);
      setSecret("");
      onClose();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "Erro inesperado.");
    } finally {
      setSaving(false);
    }
  };
  const existing = items.filter(
    (item) =>
      item.type === "gitea" || item.type === "ollama" || item.type === "openai",
  );
  return (
    <div className={styles.wizard}>
      <aside className={styles.rail}>
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
              : family === "llm"
                ? "Defina o modelo"
                : "Revise a conexão"
        }
        eyebrow="NOVA CONEXÃO"
        description={
          family === "gitea"
            ? "Gitea fornece o contexto do pull request e recebe a publicação."
              : "A conexão protege o acesso; o modelo será salvo como perfil reutilizável."
        }
        onClose={onClose}
        footer={
          <>
            <span>
              Etapa {step + 1} de {steps.length}
            </span>
            <div className={styles.actions}>
              {step > 0 && (
                <button
                  type="button"
                  onClick={() => {
                    setStep(step - 1);
                    setError("");
                  }}
                >
                  <ChevronLeft size={15} /> Voltar
                </button>
              )}
              {step < 2 ? (
                <button className={styles.primary} type="button" onClick={next}>
                  Continuar <ChevronRight size={15} />
                </button>
              ) : (
                <button
                  className={styles.primary}
                  type="button"
                  disabled={saving || (family === "llm" && !model)}
                  onClick={() => void submit()}
                >
                  {saving ? "Salvando" : "Concluir conexão"}
                </button>
              )}
            </div>
          </>
        }
        className={styles.modal}
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
                {existing.length ? (
                  existing.map((item) => (
                    <article key={item.key}>
                      <i />
                      <strong>{item.name}</strong>
                      <small>
                        {item.type === "gitea" ? "Gitea" : item.config.model}
                      </small>
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
              {family === "llm" && (
                <fieldset>
                  <legend>Provider</legend>
                  {(
                    ["ollama-local", "ollama-cloud", "openrouter"] as Provider[]
                  ).map((item) => (
                    <button
                      key={item}
                      type="button"
                      className={provider === item ? styles.selected : ""}
                      onClick={() => choose("llm", item)}
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
                  placeholder={
                    family === "gitea" ? "gitea-principal" : "modelo-review"
                  }
                  pattern="[A-Za-z0-9_-]+"
                />
              </label>
              <label>
                Nome de exibição
                <input
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder={
                    family === "gitea"
                      ? "Gitea da engenharia"
                      : "Modelo de revisão"
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
          {step === 2 && (
            <div className={styles.form}>
              {family === "llm" ? (
                <label>
                  Modelo
                  <input
                    value={model}
                    onChange={(event) => setModel(event.target.value)}
                    placeholder={
                      provider === "openrouter"
                        ? "openai/gpt-oss-20b"
                        : "qwen2.5-coder:14b"
                    }
                  />
                  <small>
                    Esse modelo será criado como um perfil reutilizável nos cards de IA.
                  </small>
                </label>
              ) : (
                <Summary
                  icon={<Server size={22} />}
                  title={name || "Conexão Gitea"}
                  value={baseURL || "URL não definida"}
                  note="A conexão será usada nos cards Buscar dados e Publicar."
                />
              )}
              <Summary
                icon={<ShieldCheck size={22} />}
                title="Segredo protegido"
                value={
                  secret ? "Token/API key informado" : "Token/API key não informado"
                }
                note="Será cifrado no backend; o navegador não o armazena."
              />
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

function Summary({
  icon,
  title,
  value,
  note,
}: {
  icon: React.ReactNode;
  title: string;
  value: string;
  note: string;
}) {
  return (
    <div className={styles.summary}>
      {icon}
      <div>
        <strong>{title}</strong>
        <span>{value}</span>
        <small>{note}</small>
      </div>
    </div>
  );
}
