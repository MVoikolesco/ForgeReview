"use client";

import type { AdminRequest } from "@/lib/admin-client";
import type { Connection, Model, Profile, Provider } from "@/lib/contracts";
import {
  ArrowLeft,
  Check,
  CheckCircle2,
  Circle,
  Clock3,
  Cpu,
  Database,
  ExternalLink,
  KeyRound,
  Loader2,
  Plus,
  Radio,
  Server,
  Sparkles,
  Trash2,
  Zap,
} from "lucide-react";
import { useState } from "react";
import { ProviderMark } from "./provider-mark";

type Props = {
  connection: Connection;
  provider?: Provider;
  models: Model[];
  profiles: Profile[];
  request: AdminRequest;
  onBack: () => void;
  onRefresh: () => Promise<void>;
  onAddModel: () => void;
};

export function ConnectionDetail({
  connection,
  provider,
  models,
  profiles,
  request,
  onBack,
  onRefresh,
  onAddModel,
}: Props) {
  const [busy, setBusy] = useState("");
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  async function run(
    key: string,
    action: () => Promise<void>,
    success: string,
  ) {
    setBusy(key);
    setError("");
    setNotice("");
    try {
      await action();
      await onRefresh();
      setNotice(success);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy("");
    }
  }

  async function setConnectionDefault() {
    await run(
      "connection",
      async () => {
        await request(`ai/connections/${connection.id}/set-default`, {
          method: "POST",
        });
      },
      "Conexão definida como rota padrão.",
    );
  }

  async function setModelDefault(model: Model) {
    await run(
      `model-${model.id}`,
      async () => {
        await request(`ai/models/${model.id}/set-default`, { method: "POST" });
        const profile = profiles.find((item) => item.is_default === 1);
        if (profile)
          await request(`review/profiles/${profile.id}`, {
            method: "PUT",
            body: JSON.stringify({
              name: profile.name,
              description: profile.description || "",
              model_id: model.id,
              is_default: 1,
              is_enabled: profile.is_enabled,
            }),
          });
      },
      `${model.display_name} agora é o modelo padrão.`,
    );
  }

  async function removeModel(model: Model) {
    if (
      !window.confirm(`Remover ${model.display_name}? Esta ação é destrutiva.`)
    )
      return;
    await run(
      `delete-model-${model.id}`,
      async () => {
        await request(`ai/models/${model.id}`, { method: "DELETE" });
      },
      "Modelo removido.",
    );
  }
  async function removeConnection() {
    if (
      !window.confirm(
        `Remover a conexão ${connection.name} e seus modelos? Esta ação é destrutiva.`,
      )
    )
      return;
    await run(
      "delete-connection",
      async () => {
        await request(`ai/connections/${connection.id}`, { method: "DELETE" });
        onBack();
      },
      "Conexão removida.",
    );
  }

  async function testConnection() {
    await run(
      "test",
      async () => {
        await request(`ai/connections/${connection.id}/test`, {
          method: "POST",
        });
      },
      "Conexão validada com sucesso.",
    );
  }

  return (
    <section className="detail-page">
      <button className="back-button" onClick={onBack}>
        <ArrowLeft size={17} />
        Todas as conexões
      </button>
      {notice && (
        <div className="banner success">
          <CheckCircle2 size={17} />
          {notice}
        </div>
      )}
      {error && <div className="banner error">{error}</div>}
      <div className="detail-hero">
        <div className="detail-identity">
          <ProviderMark provider={provider?.name} large />
          <div>
            <span className="detail-provider">{provider?.display_name}</span>
            <h2>{connection.name}</h2>
            <span
              className={`status-chip ${connection.is_enabled ? "online" : "offline"}`}
            >
              <i />
              {connection.is_enabled ? "Conexão ativa" : "Conexão inativa"}
            </span>
          </div>
        </div>
        <div className="detail-actions">
          <button
            className="secondary-button"
            onClick={testConnection}
            disabled={Boolean(busy)}
          >
            {busy === "test" ? (
              <Loader2 className="spin" size={17} />
            ) : (
              <Zap size={17} />
            )}
            Testar conexão
          </button>
          <button
            className="primary-button"
            onClick={setConnectionDefault}
            disabled={Boolean(busy) || connection.is_default === 1}
          >
            {busy === "connection" ? (
              <Loader2 className="spin" size={17} />
            ) : connection.is_default === 1 ? (
              <Check size={17} />
            ) : (
              <Sparkles size={17} />
            )}
            {connection.is_default === 1
              ? "Conexão padrão deste provider"
              : "Definir como padrão"}
          </button>
          <button
            className="secondary-button"
            onClick={onAddModel}
            disabled={Boolean(busy)}
          >
            <Plus size={17} />
            Adicionar modelo
          </button>
          <button
            className="danger-button"
            onClick={removeConnection}
            disabled={Boolean(busy)}
          >
            <Trash2 size={17} />
            Remover conexão
          </button>
        </div>
      </div>

      <div className="detail-layout">
        <div className="detail-main">
          <div className="section-title">
            <div>
              <h3>Modelos disponíveis</h3>
              <p>
                Escolha qual modelo será usado automaticamente nesta conexão.
              </p>
            </div>
            <span>{models.length}</span>
          </div>
          {!models.length ? (
            <div className="models-empty">
              <Cpu size={25} />
              <strong>Nenhum modelo cadastrado</strong>
              <p>
                Use “Adicionar modelo” para consultar o catálogo desta conexão.
              </p>
            </div>
          ) : (
            <div className="model-list">
              {models.map((model) => (
                <div
                  className={`model-row ${model.is_default === 1 ? "selected" : ""}`}
                  key={model.id}
                >
                  <span className="model-radio">
                    {model.is_default === 1 ? <CheckCircle2 /> : <Circle />}
                  </span>
                  <div className="model-info">
                    <div>
                      <strong>{model.display_name}</strong>
                      {model.is_default === 1 && (
                        <span className="default-badge">
                          <Sparkles size={11} />
                          Padrão
                        </span>
                      )}
                    </div>
                    <code>{model.provider_model_name}</code>
                    <span>
                      <Database size={13} />
                      {formatTokens(model.context_window)} contexto <i />{" "}
                      {formatTokens(model.max_output_tokens)} saída
                    </span>
                  </div>
                  <div className="model-capabilities">
                    {model.supports_json === 1 && <span>JSON</span>}
                    {model.supports_tools === 1 && <span>Tools</span>}
                    {model.supports_streaming === 1 && <span>Stream</span>}
                  </div>
                  <button
                    className={
                      model.is_default === 1
                        ? "model-active-button"
                        : "model-select-button"
                    }
                    disabled={Boolean(busy) || model.is_default === 1}
                    onClick={() => setModelDefault(model)}
                  >
                    {busy === `model-${model.id}` ? (
                      <Loader2 className="spin" size={16} />
                    ) : model.is_default === 1 ? (
                      <>
                        <Check size={15} />
                        Em uso
                      </>
                    ) : (
                      "Usar modelo"
                    )}
                  </button>
                  <button
                    className="icon-button"
                    aria-label={`Remover ${model.display_name}`}
                    onClick={() => removeModel(model)}
                    disabled={Boolean(busy)}
                  >
                    <Trash2 size={15} />
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>
        <aside className="connection-facts">
          <h3>Dados da conexão</h3>
          <Fact icon={<Server />} label="Endpoint">
            <a href={connection.base_url} target="_blank" rel="noreferrer">
              {connection.base_url}
              <ExternalLink size={13} />
            </a>
          </Fact>
          <Fact icon={<KeyRound />} label="Autenticação">
            <code>
              {connection.api_key_configured
                ? "Chave configurada"
                : "Não necessária"}
            </code>
          </Fact>
          <Fact icon={<Radio />} label="Provider">
            <span>{provider?.display_name || "-"}</span>
          </Fact>
          <Fact icon={<Clock3 />} label="Última atualização">
            <span>{formatDate(connection.updated_at)}</span>
          </Fact>
          <div className="default-route-note">
            <Sparkles size={17} />
            <div>
              <strong>Padrões explícitos</strong>
              <p>
                Conexões padrão são únicas por provider. Escolher um modelo não
                altera o provider global.
              </p>
            </div>
          </div>
        </aside>
      </div>
    </section>
  );
}

function Fact({
  icon,
  label,
  children,
}: {
  icon: React.ReactNode;
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="fact">
      <span>{icon}</span>
      <div>
        <small>{label}</small>
        {children}
      </div>
    </div>
  );
}
function formatTokens(value: number) {
  return value > 999
    ? `${Math.round(value / 1000)}k`
    : value
      ? String(value)
      : "auto";
}
function formatDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? value
    : new Intl.DateTimeFormat("pt-BR", {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(date);
}
