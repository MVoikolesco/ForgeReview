"use client";

import { Cpu, LoaderCircle, Server } from "lucide-react";
import { useState } from "react";
import {
  deleteIntegration,
  disableIntegration,
  discoverResources,
  getResources,
  replaceModels,
  replaceRepositories,
  updateIntegration,
} from "../../lib/api";
import type { Integration, ModelProfile, Repository } from "../../lib/types";
import { ModalShell } from "../common/ModalShell";
import { ToggleSwitch } from "../common/ToggleSwitch";
import styles from "./IntegrationList.module.scss";

type Role = "viewer" | "editor" | "admin";
type LifecycleAction = "disable" | "delete";

export function IntegrationList({
  items,
  profiles,
  role,
  onChanged,
}: {
  items: Integration[];
  profiles: ModelProfile[];
  role?: Role;
  onChanged: () => void;
}) {
  const [detail, setDetail] = useState<Integration | null>(null);
  const [editing, setEditing] = useState<Integration | null>(null);
  const [managing, setManaging] = useState<Integration | null>(null);
  const [lifecycle, setLifecycle] = useState<{
    item: Integration;
    action: LifecycleAction;
  } | null>(null);
  const [name, setName] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [secret, setSecret] = useState("");
  const [organizations, setOrganizations] = useState<string[]>([]);
  const [organization, setOrganization] = useState("");
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [models, setModels] = useState<string[]>([]);
  const [savedSelections, setSavedSelections] = useState<string[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const toggle = (value: string) =>
    setSelected((current) =>
      current.includes(value)
        ? current.filter((item) => item !== value)
        : [...current, value],
    );
  const report = (message: string) => {
    setError(message);
  };
  const openEdit = (item: Integration) => {
    setEditing(item);
    setName(item.name);
    setBaseURL(item.config.base_url);
    setSecret("");
    setError("");
  };
  const loadRepositories = async (
    item: Integration,
    nextOrganization: string,
    preselected = savedSelections,
  ) => {
    if (!nextOrganization) {
      setRepositories([]);
      setSelected([]);
      return;
    }
    setBusy(true);
    setError("");
    try {
      const result = await discoverResources(item.key, nextOrganization);
      const repos = result.repositories ?? [];
      setRepositories(repos);
      setOrganization(nextOrganization);
      setSelected(
        repos
          .map((repo) => `${repo.owner}/${repo.name}`)
          .filter((value) => preselected.includes(value)),
      );
    } catch {
      report("Não foi possível carregar os repositórios desta organização.");
    } finally {
      setBusy(false);
    }
  };
  const openManage = async (item: Integration) => {
    setManaging(item);
    setBusy(true);
    setError("");
    setNotice("");
    setQuery("");
    setRepositories([]);
    setModels([]);
    setSelected([]);
    setOrganization("");
    try {
      const persisted = await getResources(item.key);
      const persistedValues =
        item.type === "gitea"
          ? (persisted.repositories ?? []).map(
              (repo) => `${repo.owner}/${repo.name}`,
            )
          : (persisted.models ?? []);
      setSavedSelections(persistedValues);
      if (item.type === "gitea") {
        const result = await discoverResources(item.key);
        const orgs = result.organizations ?? [];
        setOrganizations(orgs);
        const initialOrganization = (persisted.repositories ?? [])[0]?.owner;
        if (initialOrganization && orgs.includes(initialOrganization))
          await loadRepositories(item, initialOrganization, persistedValues);
      } else {
        const result = await discoverResources(item.key);
        setModels(result.models ?? []);
        setSelected(persistedValues);
      }
    } catch {
      report("Não foi possível descobrir os recursos desta conexão.");
    } finally {
      setBusy(false);
    }
  };
  const saveResources = async () => {
    if (!managing) return;
    setBusy(true);
    setError("");
    try {
      if (managing.type === "gitea")
        await replaceRepositories(
          managing.key,
          repositories.filter((repo) =>
            selected.includes(`${repo.owner}/${repo.name}`),
          ),
        );
      else await replaceModels(managing.key, selected);
      setManaging(null);
      setNotice("Recursos atualizados.");
      onChanged();
    } catch {
      report("Não foi possível salvar as seleções.");
    } finally {
      setBusy(false);
    }
  };
  const saveEdit = async () => {
    if (!editing) return;
    setBusy(true);
    setError("");
    try {
      await updateIntegration(editing.key, {
        name,
        config: { base_url: baseURL },
        status: editing.status,
        ...(secret ? { secret } : {}),
      });
      setEditing(null);
      setNotice("Conexão atualizada.");
      onChanged();
    } catch {
      report("Não foi possível editar a conexão.");
    } finally {
      setBusy(false);
    }
  };
  const applyLifecycle = async () => {
    if (!lifecycle) return;
    setBusy(true);
    setError("");
    try {
      if (lifecycle.action === "disable")
        await disableIntegration(lifecycle.item.key);
      else await deleteIntegration(lifecycle.item.key);
      setNotice(
        lifecycle.action === "disable"
          ? "Conexão desativada."
          : "Conexão excluída.",
      );
      setLifecycle(null);
      onChanged();
    } catch (failure) {
      report(
        failure instanceof Error
          ? failure.message
          : "Não foi possível concluir a ação.",
      );
    } finally {
      setBusy(false);
    }
  };
  const filtered = (
    managing?.type === "gitea"
      ? repositories.map((repo) => ({
          value: `${repo.owner}/${repo.name}`,
          label: `${repo.owner}/${repo.name}`,
        }))
      : models.map((model) => ({ value: model, label: model }))
  ).filter((item) =>
    item.label.toLocaleLowerCase().includes(query.toLocaleLowerCase()),
  );
  const gitea = items.filter((item) => item.type === "gitea");
  return (
    <div className={styles.list}>
      {notice && (
        <p className={styles.notice} role="status">
          {notice}
        </p>
      )}
      <section>
        <h2>Modelos reutilizáveis</h2>
        {profiles.length ? (
          profiles.map((profile) => (
            <article key={profile.key}>
              <i />
              <strong>{profile.name}</strong>
              <small>{profile.model}</small>
              <em>{profile.status === "active" ? "Ativo" : "Desativado"}</em>
            </article>
          ))
        ) : (
          <p>
            Nenhum perfil cadastrado. Crie uma conexão de modelo para gerar o
            primeiro perfil.
          </p>
        )}
      </section>
      <section>
        <h2>Conexões Gitea</h2>
        {gitea.length ? (
          gitea.map((item) => (
            <article key={item.key}>
              <i />
              <strong>{item.name}</strong>
              <small>{item.config.base_url}</small>
              <em>
                {item.status === "active"
                  ? item.secret_configured
                    ? "Segredo configurado"
                    : "Sem segredo"
                  : "Desativada"}
              </em>
              <div className={styles.itemActions}>
                {(role === "editor" || role === "admin") && item.status === "active" && (
                  <button type="button" onClick={() => void openManage(item)}>
                    Gerenciar recursos
                  </button>
                )}
                <button
                  type="button"
                  onClick={() => {
                    setDetail(item);
                    setError("");
                  }}
                >
                  Ver detalhes
                </button>
                {role === "admin" && (
                  <button type="button" onClick={() => openEdit(item)}>
                    Editar
                  </button>
                )}
                {role === "admin" && item.status === "active" && (
                  <button
                    type="button"
                    onClick={() => setLifecycle({ item, action: "disable" })}
                  >
                    Desativar
                  </button>
                )}
                {role === "admin" && (
                  <button
                    type="button"
                    onClick={() => setLifecycle({ item, action: "delete" })}
                  >
                    Excluir
                  </button>
                )}
              </div>
            </article>
          ))
        ) : (
          <p>Nenhuma conexão Gitea cadastrada.</p>
        )}
      </section>
      <section className={styles.llmSection}>
        <h2>Conexões de modelos</h2>
        {items.filter((item) => item.type !== "gitea").length ? (
          items
            .filter((item) => item.type !== "gitea")
            .map((item) => (
              <article key={item.key}>
                <i />
                <strong>{item.name}</strong>
                <small>{item.config.base_url}</small>
                <em>
                  {item.status === "active"
                    ? item.secret_configured
                      ? "Segredo configurado"
                      : "Sem segredo"
                    : "Desativada"}
                </em>
                <div className={styles.itemActions}>
                  {(role === "editor" || role === "admin") && item.status === "active" && (
                    <button type="button" onClick={() => void openManage(item)}>
                      Gerenciar modelos
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => {
                      setDetail(item);
                      setError("");
                    }}
                  >
                    Ver detalhes
                  </button>
                  {role === "admin" && (
                    <button type="button" onClick={() => openEdit(item)}>
                      Editar
                    </button>
                  )}
                  {role === "admin" && item.status === "active" && (
                    <button
                      type="button"
                      onClick={() => setLifecycle({ item, action: "disable" })}
                    >
                      Desativar
                    </button>
                  )}
                  {role === "admin" && (
                    <button
                      type="button"
                      onClick={() => setLifecycle({ item, action: "delete" })}
                    >
                      Excluir
                    </button>
                  )}
                </div>
              </article>
            ))
        ) : (
          <p>Nenhuma conexão de modelo cadastrada.</p>
        )}
      </section>
      {detail && (
        <ModalShell
          title={detail.name}
          eyebrow="DETALHES DA CONEXÃO"
          description="Resumo seguro da conexão registrada."
          onClose={() => setDetail(null)}
          footer={
            <button type="button" onClick={() => setDetail(null)}>
              Fechar
            </button>
          }
        >
          <div className={styles.modalBody}>
            <dl>
              <dt>Tipo</dt>
              <dd>{detail.type}</dd>
              <dt>URL base</dt>
              <dd>{detail.config.base_url}</dd>
              <dt>Segredo</dt>
              <dd>
                {detail.secret_configured ? "Configurado" : "Não configurado"}
              </dd>
              <dt>Status</dt>
              <dd>{detail.status === "active" ? "Ativa" : "Desativada"}</dd>
            </dl>
          </div>
        </ModalShell>
      )}
      {editing && (
        <ModalShell
          title={`Editar ${editing.name}`}
          eyebrow="CONEXÃO"
          description="Deixe o novo segredo vazio para manter o segredo cifrado atual."
          onClose={() => setEditing(null)}
          footer={
            <>
              <button
                type="button"
                disabled={busy}
                onClick={() => setEditing(null)}
              >
                Cancelar
              </button>
              <button
                className={styles.primary}
                type="button"
                disabled={busy}
                onClick={() => void saveEdit()}
              >
                {busy && <LoaderCircle className={styles.spinner} size={15} />}
                {busy ? "Salvando…" : "Salvar alterações"}
              </button>
            </>
          }
        >
          <div className={styles.modalBody}>
            <label>
              Nome
              <input
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </label>
            <label>
              URL base
              <input
                type="url"
                value={baseURL}
                onChange={(event) => setBaseURL(event.target.value)}
              />
            </label>
            <label>
              Novo segredo (opcional)
              <input
                type="password"
                value={secret}
                onChange={(event) => setSecret(event.target.value)}
                autoComplete="new-password"
              />
            </label>
            {error && (
              <p className={styles.error} role="alert">
                {error}
              </p>
            )}
          </div>
        </ModalShell>
      )}
      {managing && (
        <ModalShell
          title={
            managing.type === "gitea"
              ? `Recursos de ${managing.name}`
              : `Modelos de ${managing.name}`
          }
          eyebrow="GERENCIAR RECURSOS"
          description="As seleções existentes são carregadas para edição."
          onClose={() => setManaging(null)}
          footer={
            <>
              <button
                type="button"
                disabled={busy}
                onClick={() => setManaging(null)}
              >
                Cancelar
              </button>
              <button
                className={styles.primary}
                type="button"
                disabled={busy || (managing.type === "gitea" && !organization)}
                onClick={() => void saveResources()}
              >
                {busy && <LoaderCircle className={styles.spinner} size={15} />}
                {busy ? "Salvando…" : "Salvar seleções"}
              </button>
            </>
          }
        >
          <div className={styles.modalBody}>
            {managing.type === "gitea" && (
              <label>
                Organização
                <select
                  value={organization}
                  disabled={busy}
                  onChange={(event) =>
                    void loadRepositories(managing, event.target.value)
                  }
                >
                  <option value="">Selecione uma organização</option>
                  {organizations.map((item) => (
                    <option key={item} value={item}>
                      {item}
                    </option>
                  ))}
                </select>
              </label>
            )}
            <label>
              Buscar {managing.type === "gitea" ? "repositórios" : "modelos"}
              <input
                type="search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Filtrar resultados"
              />
            </label>
            {busy && (
              <p className={styles.loading} role="status">
                <LoaderCircle className={styles.spinner} size={15} /> Carregando
                recursos…
              </p>
            )}
            <strong className={styles.selectionCount}>{selected.length} selecionado{selected.length === 1 ? "" : "s"}</strong>
            <div className={styles.resourceList} aria-label={managing.type === "gitea" ? "Repositórios selecionáveis" : "Modelos selecionáveis"}>
              {filtered.length
                ? filtered.map((item) => (
                    <ToggleSwitch
                      key={item.value}
                      variant="card"
                      checked={selected.includes(item.value)}
                      disabled={busy || role === "viewer"}
                      onChange={() => toggle(item.value)}
                      label={item.label}
                      description={managing.type === "gitea" ? `Repositório Gitea · ${organization}` : `${managing.type === "openai" ? "OpenAI compatível" : "Ollama"} · modelo`}
                      leading={managing.type === "gitea" ? <Server size={16} /> : <Cpu size={16} />}
                    />
                  ))
                : !busy && <p>Nenhum recurso disponível.</p>}
            </div>
            {error && (
              <p className={styles.error} role="alert">
                {error}
              </p>
            )}
          </div>
        </ModalShell>
      )}
      {lifecycle && (
        <ModalShell
          title={
            lifecycle.action === "disable"
              ? "Desativar conexão?"
              : "Excluir conexão?"
          }
          eyebrow="CONFIRMAR AÇÃO"
          description={
            lifecycle.action === "disable"
              ? "A conexão deixará de estar disponível para novos usos. O histórico será preservado."
              : "A exclusão só é permitida quando não há histórico de workflow referenciando a conexão."
          }
          onClose={() => setLifecycle(null)}
          footer={
            <>
              <button
                type="button"
                disabled={busy}
                onClick={() => setLifecycle(null)}
              >
                Cancelar
              </button>
              <button
                className={styles.danger}
                type="button"
                disabled={busy}
                onClick={() => void applyLifecycle()}
              >
                {busy && <LoaderCircle className={styles.spinner} size={15} />}
                {busy
                  ? "Processando…"
                  : lifecycle.action === "disable"
                    ? "Desativar"
                    : "Excluir"}
              </button>
            </>
          }
        >
          <div className={styles.modalBody}>
            <p>{lifecycle.item.name}</p>
            {error && (
              <p className={styles.error} role="alert">
                {error}
              </p>
            )}
          </div>
        </ModalShell>
      )}
    </div>
  );
}
