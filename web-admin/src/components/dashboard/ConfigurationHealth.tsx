import { useCallback, useEffect, useMemo, useState } from "react";
import type { AdminRequest } from "../../api/adminClient";
import type { Row } from "../../types";

type CatalogModel = {
  id: string;
  name: string;
  context_length?: number;
  max_completion_tokens?: number;
  supported_parameters?: string[];
  size?: number;
};
type Props = { request: AdminRequest; onAdvanced: () => void };

const asBool = (value: any) => Number(value) === 1 || value === true;
const providerLabel = (provider?: Row) =>
  String(provider?.display_name || provider?.name || "Provider");
const modelLabel = (model?: Row) =>
  String(model?.display_name || model?.provider_model_name || "Modelo");

export function ConfigurationHealth({ request, onAdvanced }: Props) {
  const [providers, setProviders] = useState<Row[]>([]),
    [connections, setConnections] = useState<Row[]>([]),
    [models, setModels] = useState<Row[]>([]),
    [parameters, setParameters] = useState<Row[]>([]),
    [profiles, setProfiles] = useState<Row[]>([]);
  const [selectedID, setSelectedID] = useState<number | null>(null),
    [connectionDraft, setConnectionDraft] = useState<Row | null>(null),
    [modelDraft, setModelDraft] = useState<Row | null>(null),
    [catalog, setCatalog] = useState<CatalogModel[]>([]),
    [catalogSearch, setCatalogSearch] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const selected = connections.find((item) => Number(item.id) === selectedID);
  const providerByID = useMemo(
    () => Object.fromEntries(providers.map((item) => [item.id, item])),
    [providers],
  );
  const modelsForSelected = useMemo(
    () => models.filter((model) => Number(model.connection_id) === selectedID),
    [models, selectedID],
  );
  const paramsByModel = useMemo(
    () => Object.fromEntries(parameters.map((item) => [item.model_id, item])),
    [parameters],
  );
  const profileByModel = useMemo(
    () => Object.fromEntries(profiles.map((item) => [item.model_id, item])),
    [profiles],
  );
  const visibleCatalog = useMemo(() => {
    const term = catalogSearch.trim().toLowerCase();
    return (
      term
        ? catalog.filter((item) =>
            (item.id + " " + item.name).toLowerCase().includes(term),
          )
        : catalog
    ).slice(0, 60);
  }, [catalog, catalogSearch]);

  const refresh = useCallback(async () => {
    setBusy(true);
    try {
      const [
        providerRows,
        connectionRows,
        modelRows,
        parameterRows,
        profileRows,
      ] = await Promise.all([
        request("ai/providers"),
        request("ai/connections"),
        request("ai/models"),
        request("ai/model-parameters"),
        request("review/profiles"),
      ]);
      setProviders(providerRows);
      setConnections(connectionRows);
      setModels(modelRows);
      setParameters(parameterRows);
      setProfiles(profileRows);
      setError("");
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  }, [request]);
  useEffect(() => {
    refresh();
  }, [refresh]);
  const chooseConnection = (connection: Row) => {
    setSelectedID(Number(connection.id));
    setConnectionDraft({ ...connection });
    setModelDraft(null);
    setCatalog([]);
    setError("");
    setNotice("");
  };
  const closeConnection = () => {
    setSelectedID(null);
    setConnectionDraft(null);
    setModelDraft(null);
    setCatalog([]);
  };
  const updateConnection = (key: string, value: any) =>
    setConnectionDraft((current) =>
      current ? { ...current, [key]: value } : current,
    );
  const updateModel = (key: string, value: any) =>
    setModelDraft((current) =>
      current ? { ...current, [key]: value } : current,
    );
  const saveConnection = async () => {
    if (!connectionDraft) return;
    setBusy(true);
    try {
      const body = {
        name: connectionDraft.name,
        base_url: connectionDraft.base_url,
        api_key_env_name: connectionDraft.api_key_env_name || "",
        http_referer: connectionDraft.http_referer || "",
        app_title: connectionDraft.app_title || "",
        is_enabled: asBool(connectionDraft.is_enabled) ? 1 : 0,
      };
      await request(`ai/connections/${connectionDraft.id}`, {
        method: "PUT",
        body: JSON.stringify(body),
      });
      setNotice("Conexão atualizada.");
      await refresh();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  };
  const loadCatalog = async () => {
    if (!selected) return;
    const provider = providerByID[selected.provider_id];
    setBusy(true);
    setError("");
    try {
      const result = await request("setup/catalog", {
        method: "POST",
        body: JSON.stringify({
          provider: provider?.name,
          connection: {
            name: selected.name,
            base_url: selected.base_url,
            api_key_env_name: selected.api_key_env_name || "",
            http_referer: selected.http_referer || "",
            app_title: selected.app_title || "",
          },
        }),
      });
      setCatalog(result.models || []);
      setCatalogSearch("");
      if (!result.models?.length)
        setError("A conexão respondeu, mas não retornou modelos.");
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  };
  const createModel = async (catalogModel: CatalogModel) => {
    if (!selected) return;
    const provider = providerByID[selected.provider_id];
    setBusy(true);
    try {
      const model = await request("ai/models", {
        method: "POST",
        body: JSON.stringify({
          connection_id: selected.id,
          provider_model_name: catalogModel.id,
          display_name: catalogModel.name || catalogModel.id,
          context_window: catalogModel.context_length || 0,
          max_output_tokens:
            provider?.name === "openrouter"
              ? Math.min(catalogModel.max_completion_tokens || 4096, 4096)
              : 0,
          supports_json:
            catalogModel.supported_parameters?.includes("response_format") ||
            catalogModel.supported_parameters?.includes("structured_outputs")
              ? 1
              : 0,
          supports_tools: catalogModel.supported_parameters?.includes("tools")
            ? 1
            : 0,
          supports_streaming: 1,
          is_default: 0,
          is_enabled: 1,
        }),
      });
      await request("ai/model-parameters", {
        method: "POST",
        body: JSON.stringify({
          model_id: model.id,
          temperature: 0.2,
          top_p: 0.9,
          repeat_penalty: 1.1,
          num_ctx: 0,
          num_threads: 0,
          num_predict: 0,
          keep_alive: "5m",
          timeout_seconds: 900,
          unload_model_after_review: 0,
        }),
      });
      setNotice(`Modelo ${catalogModel.name || catalogModel.id} cadastrado.`);
      setCatalog([]);
      await refresh();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  };
  const saveModel = async () => {
    if (!modelDraft) return;
    setBusy(true);
    try {
      await request(`ai/models/${modelDraft.id}`, {
        method: "PUT",
        body: JSON.stringify({
          display_name: modelDraft.display_name,
          provider_model_name: modelDraft.provider_model_name,
          context_window: Number(modelDraft.context_window) || 0,
          max_output_tokens: Number(modelDraft.max_output_tokens) || 0,
          supports_json: asBool(modelDraft.supports_json) ? 1 : 0,
          supports_tools: asBool(modelDraft.supports_tools) ? 1 : 0,
          supports_streaming: asBool(modelDraft.supports_streaming) ? 1 : 0,
          is_enabled: asBool(modelDraft.is_enabled) ? 1 : 0,
        }),
      });
      setNotice("Modelo atualizado.");
      setModelDraft(null);
      await refresh();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  };
  const useModel = async (model: Row) => {
    setBusy(true);
    try {
      await request(`ai/models/${model.id}/set-default`, { method: "POST" });
      const profile = profiles.find((item) => asBool(item.is_default));
      if (profile)
        await request(`review/profiles/${profile.id}`, {
          method: "PUT",
          body: JSON.stringify({
            name: profile.name,
            description: profile.description || "",
            model_id: model.id,
            is_default: 1,
            is_enabled: asBool(profile.is_enabled) ? 1 : 0,
          }),
        });
      setNotice(`${modelLabel(model)} será usado nas próximas revisões.`);
      await refresh();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  };
  const activeModelID = profiles.find((item) =>
    asBool(item.is_default),
  )?.model_id;
  return (
    <section className="configuration-health">
      {error && <div className="configuration-message error">{error}</div>}
      {notice && <div className="configuration-message success">{notice}</div>}
      <div className="connection-layout">
        <div className="connection-list">
          {!connections.length ? (
            <div className="configuration-empty">
              Nenhuma conexão cadastrada.
            </div>
          ) : (
            connections.map((connection) => (
              <button
                key={connection.id}
                className={`connection-card ${selectedID === Number(connection.id) ? "selected" : ""}`}
                onClick={() => chooseConnection(connection)}
              >
                <span className="connection-avatar">
                  {String(providerLabel(providerByID[connection.provider_id]))
                    .slice(0, 2)
                    .toUpperCase()}
                </span>
                <span className="connection-card-copy">
                  <strong>{connection.name}</strong>
                  <small>
                    {providerLabel(providerByID[connection.provider_id])}
                  </small>
                  <small>{connection.base_url}</small>
                </span>
                <span
                  className={`connection-status ${asBool(connection.is_enabled) ? "online" : "offline"}`}
                >
                  {asBool(connection.is_enabled) ? "Ativa" : "Desativada"}
                </span>
              </button>
            ))
          )}
        </div>
        {selected && (
          <div
            className="connection-modal-backdrop"
            onMouseDown={(event) => {
              if (event.target === event.currentTarget) closeConnection();
            }}
          >
            <div className="connection-detail connection-modal" role="dialog" aria-modal="true" aria-label={`Editar conexão ${selected.name}`}>
            <header>
              <div>
                <span className="section-kicker">Conexão selecionada</span>
                <h4>{selected.name}</h4>
                <p>
                  {providerLabel(providerByID[selected.provider_id])} ·{" "}
                  {selected.base_url}
                </p>
              </div>
              <span className="connection-default">
                {asBool(selected.is_default) ? "Padrão" : "Disponível"}
              </span>
              <button className="connection-modal-close" onClick={closeConnection} aria-label="Fechar">×</button>
            </header>
            <div className="connection-edit-grid">
              <label>
                <span>Nome</span>
                <input
                  value={connectionDraft?.name || ""}
                  onChange={(event) =>
                    updateConnection("name", event.target.value)
                  }
                />
              </label>
              <label>
                <span>URL base</span>
                <input
                  value={connectionDraft?.base_url || ""}
                  onChange={(event) =>
                    updateConnection("base_url", event.target.value)
                  }
                />
              </label>
              {providerByID[selected.provider_id]?.name === "openrouter" && (
                <label>
                  <span>Variável da API key</span>
                  <input
                    value={connectionDraft?.api_key_env_name || ""}
                    onChange={(event) =>
                      updateConnection("api_key_env_name", event.target.value)
                    }
                  />
                </label>
              )}
              <div className="connection-edit-actions">
                <button
                  className="button button-primary"
                  onClick={saveConnection}
                  disabled={busy}
                >
                  Salvar conexão
                </button>
                <button
                  className="button button-secondary"
                  onClick={() => {
                    setConnectionDraft({ ...selected });
                    setNotice("");
                  }}
                >
                  Descartar
                </button>
              </div>
            </div>
            <div className="model-section">
              <div className="model-section-heading">
                <div>
                  <span className="section-kicker">Modelos desta conexão</span>
                  <h4>{modelsForSelected.length} cadastrados</h4>
                </div>
                <button
                  className="button button-primary"
                  onClick={loadCatalog}
                  disabled={busy}
                >
                  {busy ? "Consultando..." : "Adicionar modelo do catálogo"}
                </button>
              </div>
              {!modelsForSelected.length ? (
                <div className="configuration-empty compact">
                  Nenhum modelo cadastrado nesta conexão.
                </div>
              ) : (
                <div className="connection-model-list">
                  {modelsForSelected.map((model) => (
                    <article
                      className={`connection-model ${String(activeModelID) === String(model.id) ? "active" : ""}`}
                      key={model.id}
                    >
                      <div className="connection-model-main">
                        <span className="model-chip">
                          {String(modelLabel(model)).slice(0, 2).toUpperCase()}
                        </span>
                        <div>
                          <strong>{modelLabel(model)}</strong>
                          <small>
                            {model.provider_model_name} ·{" "}
                            {Number(
                              model.max_output_tokens || 0,
                            ).toLocaleString("pt-BR")}{" "}
                            tokens de saída
                          </small>
                        </div>
                      </div>
                      <div className="connection-model-actions">
                        {String(activeModelID) === String(model.id) && (
                          <span className="using-badge">Em uso</span>
                        )}
                        <button
                          className="text-action"
                          onClick={() => setModelDraft({ ...model })}
                        >
                          Editar
                        </button>
                        {String(activeModelID) !== String(model.id) && (
                          <button
                            className="text-action"
                            onClick={() => useModel(model)}
                          >
                            Usar na revisão
                          </button>
                        )}
                      </div>
                    </article>
                  ))}
                </div>
              )}
            </div>
            {modelDraft && (
              <div className="inline-editor">
                <div>
                  <span className="section-kicker">Editar modelo</span>
                  <h4>{modelLabel(modelDraft)}</h4>
                </div>
                <div className="inline-editor-grid">
                  <label>
                    <span>Nome de exibição</span>
                    <input
                      value={modelDraft.display_name || ""}
                      onChange={(event) =>
                        updateModel("display_name", event.target.value)
                      }
                    />
                  </label>
                  <label>
                    <span>Máx. tokens de saída</span>
                    <input
                      type="number"
                      min="1"
                      max="32768"
                      value={modelDraft.max_output_tokens || 0}
                      onChange={(event) =>
                        updateModel(
                          "max_output_tokens",
                          Number(event.target.value),
                        )
                      }
                    />
                  </label>
                </div>
                <div className="connection-edit-actions">
                  <button
                    className="button button-primary"
                    onClick={saveModel}
                    disabled={busy}
                  >
                    Salvar modelo
                  </button>
                  <button
                    className="button button-secondary"
                    onClick={() => setModelDraft(null)}
                  >
                    Cancelar
                  </button>
                </div>
              </div>
            )}
            {catalog.length > 0 && (
              <div className="catalog-picker">
                <div className="model-section-heading">
                  <div>
                    <span className="section-kicker">Catálogo do provider</span>
                    <h4>Escolha um modelo para cadastrar</h4>
                  </div>
                  <button
                    className="button button-secondary"
                    onClick={() => setCatalog([])}
                  >
                    Fechar
                  </button>
                </div>
                <input
                  className="catalog-search"
                  value={catalogSearch}
                  onChange={(event) => setCatalogSearch(event.target.value)}
                  placeholder="Buscar modelo..."
                />
                <div className="catalog-options">
                  {visibleCatalog.map((item) => (
                    <button
                      key={item.id}
                      onClick={() => createModel(item)}
                      disabled={busy}
                    >
                      <span>
                        <strong>{item.name || item.id}</strong>
                        <small>{item.id}</small>
                      </span>
                      <span>
                        {item.context_length
                          ? `${item.context_length.toLocaleString("pt-BR")} tokens`
                          : ""}{" "}
                        <b>Adicionar</b>
                      </span>
                    </button>
                  ))}
                </div>
              </div>
            )}
            </div>
          </div>
        )}
      </div>
    </section>
  );
}
