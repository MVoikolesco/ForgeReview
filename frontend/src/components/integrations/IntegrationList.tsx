"use client";
import { useState } from "react";
import { deleteIntegration, disableIntegration, discoverResources, replaceModels, replaceRepositories, updateIntegration } from "../../lib/api";
import type { Integration, ModelProfile, Repository } from "../../lib/types";
import styles from "./IntegrationList.module.scss";

export function IntegrationList({
  items,
  profiles,
  role,
  onChanged,
}: {
  items: Integration[];
  profiles: ModelProfile[];
  role?: "viewer" | "editor" | "admin";
  onChanged: () => void;
}) {
	const [managing, setManaging] = useState<Integration | null>(null);
	const [available, setAvailable] = useState<string[]>([]);
	const [repositories, setRepositories] = useState<Repository[]>([]);
	const [selected, setSelected] = useState<string[]>([]);
	const [error, setError] = useState("");
	const [viewing, setViewing] = useState<Integration | null>(null);
	const [editing, setEditing] = useState<Integration | null>(null);
	const [editName, setEditName] = useState("");
	const [editURL, setEditURL] = useState("");
	const [editSecret, setEditSecret] = useState("");
	const manage = async (item: Integration) => { try { const result = await discoverResources(item.key); setManaging(item); setRepositories(result.repositories || []); setAvailable(result.models || []); setSelected([]); setError(""); } catch { setError("Não foi possível descobrir recursos."); } };
	const save = async () => { if (!managing) return; try { if (managing.type === "gitea") await replaceRepositories(managing.key, repositories.filter((repo) => selected.includes(`${repo.owner}/${repo.name}`))); else await replaceModels(managing.key, selected); setManaging(null); onChanged(); } catch { setError("Não foi possível salvar as seleções."); } };
  const groups = [
    {
      title: "Conexões Gitea",
      items: items.filter((item) => item.type === "gitea"),
      empty: "Nenhuma conexão Gitea cadastrada.",
    },
  ];
  return (
    <div className={styles.list}>
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
          <p>Nenhum perfil cadastrado. Crie uma conexão de modelo para gerar o primeiro perfil.</p>
        )}
      </section>
      {groups.map((group) => (
        <section key={group.title}>
          <h2>{group.title}</h2>
          {group.items.length ? (
            group.items.map((item) => (
              <article key={item.key}>
                <i />
                <strong>{item.name}</strong>
                <small>
                  {item.config.base_url}
                </small>
                <em>
                  {item.secret_configured ? "Segredo configurado" : "Sem segredo"}
                </em>
                {role !== "viewer" && item.status === "active" && <button type="button" onClick={() => void manage(item)}>Gerenciar recursos</button>}
                <button type="button" onClick={() => setViewing(item)}>Ver detalhes</button>
                {role === "admin" && <button type="button" onClick={() => { setEditing(item); setEditName(item.name); setEditURL(item.config.base_url); setEditSecret(""); }}>Editar</button>}
                {role === "admin" && <><button type="button" onClick={() => void disableIntegration(item.key).then(onChanged)}>Desativar</button><button type="button" onClick={() => void deleteIntegration(item.key).then(onChanged)}>Excluir</button></>}
              </article>
            ))
          ) : (
            <p>{group.empty}</p>
          )}
        </section>
      ))}
		{managing && <section aria-label="Gerenciar recursos"><h2>Recursos de {managing.name}</h2><p>Selecione os recursos reutilizáveis.</p>{(managing.type === "gitea" ? repositories.map((repo) => ({ value: `${repo.owner}/${repo.name}`, label: `${repo.owner}/${repo.name}` })) : available.map((model) => ({ value: model, label: model }))).map((entry) => <label key={entry.value}><input type="checkbox" checked={selected.includes(entry.value)} onChange={() => setSelected((current) => current.includes(entry.value) ? current.filter((value) => value !== entry.value) : [...current, entry.value])} /> {entry.label}</label>)}<button type="button" onClick={() => void save()}>Salvar seleções</button><button type="button" onClick={() => setManaging(null)}>Cancelar</button></section>}
		{viewing && <section aria-label="Detalhes da conexão"><h2>{viewing.name}</h2><p>{viewing.type} · {viewing.config.base_url}</p><p>{viewing.secret_configured ? "Segredo configurado" : "Sem segredo"}</p><button type="button" onClick={() => setViewing(null)}>Fechar</button></section>}
		{editing && <section aria-label="Editar conexão"><h2>Editar {editing.name}</h2><label>Nome <input value={editName} onChange={(event) => setEditName(event.target.value)} /></label><label>URL base <input type="url" value={editURL} onChange={(event) => setEditURL(event.target.value)} /></label><label>Novo segredo (opcional) <input type="password" value={editSecret} onChange={(event) => setEditSecret(event.target.value)} autoComplete="new-password" /></label><button type="button" onClick={() => void updateIntegration(editing.key, { name: editName, config: { base_url: editURL }, status: editing.status, ...(editSecret ? { secret: editSecret } : {}) }).then(() => { setEditing(null); onChanged(); }).catch(() => setError("Não foi possível editar a conexão."))}>Salvar</button><button type="button" onClick={() => setEditing(null)}>Cancelar</button></section>}
		{error && <p role="alert">{error}</p>}
    </div>
  );
}
