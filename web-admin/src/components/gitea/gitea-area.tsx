"use client";
import type { AdminRequest } from "@/lib/admin-client";
import type {
  GiteaInstance,
  GiteaOrganization,
  GiteaRepository,
} from "@/lib/contracts";
import { Check, Save, Server, ShieldCheck } from "lucide-react";
import { useEffect, useState } from "react";

export function GiteaArea({
  request,
  onAuthError,
}: {
  request: AdminRequest;
  onAuthError: () => void;
}) {
  const [instances, setInstances] = useState<GiteaInstance[]>([]);
  const [instance, setInstance] = useState<GiteaInstance | null>(null);
  const [form, setForm] = useState({
    name: "",
    base_url: "",
    bot_username: "",
    token: "",
  });
  const [tested, setTested] = useState(false);
  const [orgs, setOrgs] = useState<GiteaOrganization[]>([]);
  const [repos, setRepos] = useState<GiteaRepository[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  async function load() {
    try {
      const value = await request<GiteaInstance[]>("gitea/instances");
      setInstances(value);
      if (value[0]) setInstance(value[0]);
    } catch (e) {
      if (e instanceof Error && e.message === "AUTH") onAuthError();
      else setError(String(e));
    }
  }
  useEffect(() => {
    void load();
  }, []);
  function update(field: keyof typeof form, value: string) {
    setTested(false);
    setForm((current) => ({ ...current, [field]: value }));
  }
  async function test() {
    setBusy(true);
    setTested(false);
    setError("");
    setMessage("");
    try {
      await request("gitea/instances/test", {
        method: "POST",
        body: JSON.stringify(form),
      });
      setTested(true);
      setMessage("Conexão validada. Você pode cadastrar a instância.");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  async function create() {
    setBusy(true);
    setError("");
    try {
      const saved = await request<GiteaInstance>("gitea/instances", {
        method: "POST",
        body: JSON.stringify(form),
      });
      setInstance(saved);
      setForm({ ...form, token: "" });
      await load();
      setMessage("Instância salva com segurança.");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  async function listOrganizations() {
    if (!instance) return;
    setBusy(true);
    setError("");
    try {
      setOrgs(
        await request<GiteaOrganization[]>(
          `gitea/instances/${instance.id}/organizations`,
          { method: "POST" },
        ),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  async function listRepositories(organization: string) {
    if (!instance) return;
    setBusy(true);
    setError("");
    try {
      setRepos(
        await request<GiteaRepository[]>(
          `gitea/instances/${instance.id}/repositories`,
          { method: "POST", body: JSON.stringify({ organization }) },
        ),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  async function save() {
    if (!instance) return;
    setBusy(true);
    try {
      await request(`gitea/instances/${instance.id}/repositories`, {
        method: "POST",
        body: JSON.stringify({
          repositories: selected.map((full_name) => {
            const repo = repos.find((r) => r.full_name === full_name)!;
            return {
              owner: repo.owner.login,
              name: repo.name,
              full_name: repo.full_name,
            };
          }),
        }),
      });
      setMessage("Seleção de repositórios salva.");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="connections-section" aria-labelledby="gitea-title">
      <div className="section-toolbar">
        <div>
          <span className="eyebrow">Integração de código</span>
          <h2 id="gitea-title">Gitea</h2>
          <p>
            Conecte uma instância, valide o bot e escolha os repositórios
            monitorados.
          </p>
        </div>
      </div>
      {error && (
        <div className="banner error" role="alert">
          {error}
        </div>
      )}
      {message && (
        <div className="banner success" role="status">
          <Check size={16} />
          {message}
        </div>
      )}
      <div className="connection-form">
        <div className="form-grid">
          <label>
            Nome
            <input
              value={form.name}
              onChange={(e) => update("name", e.target.value)}
              placeholder="Gitea principal"
            />
          </label>
          <label>
            Usuário bot
            <input
              value={form.bot_username}
              onChange={(e) => update("bot_username", e.target.value)}
            />
          </label>
          <label className="full">
            URL da instância
            <input
              type="url"
              value={form.base_url}
              onChange={(e) => update("base_url", e.target.value)}
              placeholder="https://gitea.exemplo.com"
            />
          </label>
          <label className="full">
            Token de acesso
            <input
              type="password"
              value={form.token}
              onChange={(e) => update("token", e.target.value)}
              autoComplete="new-password"
              placeholder="Nunca será exibido novamente"
            />
          </label>
        </div>
        <div className="topbar-actions">
          <button
            className="secondary-button"
            onClick={() => void test()}
            disabled={busy || !form.token}
          >
            <ShieldCheck size={16} />
            {busy ? "Testando..." : "Testar conexão"}
          </button>
          <button
            className="primary-button"
            onClick={() => void create()}
            disabled={busy || !tested}
          >
            <Save size={16} />
            Cadastrar
          </button>
        </div>
      </div>
      {instance && (
        <div className="connection-form">
          <h3>Repositórios monitorados</h3>
          <button
            className="secondary-button"
            onClick={() => void listOrganizations()}
            disabled={busy}
          >
            <Server size={16} />
            Listar organizações
          </button>
          {orgs.length === 0 && (
            <p>Teste e cadastre uma instância para carregar organizações.</p>
          )}
          {orgs.map((org) => (
            <button
              className="nav-item"
              key={org.id}
              onClick={() => void listRepositories(org.name)}
            >
              {org.full_name || org.name}
            </button>
          ))}
          {repos.length > 0 && (
            <>
              <div role="group" aria-label="Seleção de repositórios">
                {repos.map((repo) => (
                  <label key={repo.id}>
                    <input
                      type="checkbox"
                      checked={selected.includes(repo.full_name)}
                      onChange={() =>
                        setSelected((v) =>
                          v.includes(repo.full_name)
                            ? v.filter((x) => x !== repo.full_name)
                            : [...v, repo.full_name],
                        )
                      }
                    />{" "}
                    {repo.full_name}
                  </label>
                ))}
              </div>
              <button
                className="primary-button"
                onClick={() => void save()}
                disabled={busy}
              >
                <Save size={16} />
                Salvar seleção
              </button>
            </>
          )}
        </div>
      )}
    </section>
  );
}
