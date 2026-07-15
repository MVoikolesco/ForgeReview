import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { adminClient } from "./api/adminClient";
import { PageHeader } from "./components/layout/PageHeader";
import { Sidebar, type AppView } from "./components/layout/Sidebar";
import { ResourceDrawer } from "./components/resource/ResourceDrawer";
import { ResourceTable } from "./components/resource/ResourceTable";
import type { Theme } from "./components/ui/ThemeToggle";
import { Toast, type ToastState } from "./components/ui/Toast";
import { resources } from "./config/resources";
import { DashboardPage } from "./pages/DashboardPage";
import { LoginPage } from "./pages/LoginPage";
import { ObservabilityPage } from "./pages/ObservabilityPage";
import { SetupWizardPage } from "./pages/SetupWizardPage";
import type { Resource, Row } from "./types";

const viewCopy: Record<AppView, [string, string]> = {
  dashboard: [
    "Visão geral",
    "Acompanhe a configuração e a operação do ForgeReview.",
  ],
  setup: [
    "Configurar IA",
    "Conecte um provider e prepare o reviewer em um fluxo orientado.",
  ],
  observability: [
    "Operação",
    "Monitore workers, fila, execuções e logs em tempo real.",
  ],
  advanced: [
    "Cadastros avançados",
    "Edite individualmente os registros que sustentam a configuração.",
  ],
};

export function App() {
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = localStorage.getItem("fr.theme");
    if (saved === "light" || saved === "dark") return saved;
    return window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  });
  const [username, setUsername] = useState(
      sessionStorage.getItem("fr.user") || "",
    ),
    [password, setPassword] = useState(sessionStorage.getItem("fr.pass") || ""),
    [logged, setLogged] = useState(
      Boolean(
        sessionStorage.getItem("fr.user") && sessionStorage.getItem("fr.pass"),
      ),
    );
  const [view, setView] = useState<AppView>("dashboard"),
    [resource, setResource] = useState<Resource | null>(null);
  const [rows, setRows] = useState<Row[]>([]),
    [status, setStatus] = useState<Record<string, any>>({}),
    [operational, setOperational] = useState<Record<string, any>>({});
  const [loading, setLoading] = useState(false),
    [error, setError] = useState(""),
    [query, setQuery] = useState("");
  const [editing, setEditing] = useState(false),
    [editingID, setEditingID] = useState<number | null>(null),
    [form, setForm] = useState<Record<string, any>>({}),
    [referenceRows, setReferenceRows] = useState<Record<string, Row[]>>({});
  const [toast, setToast] = useState<ToastState>({
    show: false,
    message: "",
    tone: "success",
  });
  const request = useMemo(
    () => adminClient(username, password),
    [username, password],
  );
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme;
    localStorage.setItem("fr.theme", theme);
  }, [theme]);
  const notify = (message: string, tone: "success" | "error" = "success") => {
    setToast({ show: true, message, tone });
    window.setTimeout(
      () => setToast((value) => ({ ...value, show: false })),
      3200,
    );
  };
  const handleError = useCallback((failure: unknown) => {
    if (failure instanceof Error && failure.message === "AUTH") {
      sessionStorage.clear();
      setLogged(false);
      setError("Credenciais inválidas");
    } else
      setError(failure instanceof Error ? failure.message : String(failure));
  }, []);
  const showDashboard = useCallback(async () => {
    setView("dashboard");
    setResource(null);
    setQuery("");
    setEditing(false);
    setError("");
    try {
      const [configuration, operation] = await Promise.all([
        request("status"),
        request("observability/metrics"),
      ]);
      setStatus(configuration);
      setOperational(operation);
    } catch (failure) {
      handleError(failure);
    }
  }, [request, handleError]);
  const load = useCallback(
    async (target: Resource) => {
      setLoading(true);
      setError("");
      try {
        setRows(await request(target.path));
      } catch (failure) {
        handleError(failure);
      } finally {
        setLoading(false);
      }
    },
    [request, handleError],
  );
  const loadReferences = useCallback(
    async (target: Resource) => {
      const paths = [
        ...new Set(
          target.fields.flatMap((field) =>
            field.reference ? [field.reference.path] : [],
          ),
        ),
      ];
      if (!paths.length) {
        setReferenceRows({});
        return;
      }
      try {
        const entries = await Promise.all(
          paths.map(async (path) => [path, await request(path)] as const),
        );
        setReferenceRows(Object.fromEntries(entries));
      } catch (failure) {
        handleError(failure);
      }
    },
    [request, handleError],
  );
  useEffect(() => {
    if (logged) showDashboard();
  }, []); // restore the authenticated workspace once
  const login = async (event: FormEvent) => {
    event.preventDefault();
    sessionStorage.setItem("fr.user", username);
    sessionStorage.setItem("fr.pass", password);
    setLogged(true);
    await showDashboard();
  };
  const logout = () => {
    sessionStorage.clear();
    setLogged(false);
    setPassword("");
    setResource(null);
  };
  const selectResource = async (item: Resource) => {
    setView("advanced");
    setResource(item);
    setQuery("");
    setEditing(false);
    await Promise.all([load(item), loadReferences(item)]);
  };
  const navigate = async (target: AppView) => {
    setError("");
    if (target === "dashboard") {
      await showDashboard();
      return;
    }
    if (target === "advanced") {
      await selectResource(
        resource ||
          resources.find((item) => item.path === "ai/connections") ||
          resources[0],
      );
      return;
    }
    setView(target);
    setResource(null);
    setEditing(false);
  };
  const openForm = (row?: Row) => {
    setEditingID(row?.id ?? null);
    setForm(
      Object.fromEntries(
        (resource?.fields || []).map((field) => [
          field.key,
          row?.[field.key] ??
            field.default ??
            (field.type === "boolean" ? false : ""),
        ]),
      ),
    );
    setEditing(true);
  };
  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!resource) return;
    const body: Row = {};
    for (const field of resource.fields) {
      let value = form[field.key];
      if (field.type === "number" && value !== "") value = Number(value);
      if (field.type === "boolean") value = value ? 1 : 0;
      if (value !== "" || field.required) body[field.key] = value;
      else if (field.reference) body[field.key] = null;
    }
    try {
      await request(`${resource.path}${editingID ? `/${editingID}` : ""}`, {
        method: editingID ? "PUT" : "POST",
        body: JSON.stringify(body),
      });
      setEditing(false);
      notify(`${resource.singular} salvo com sucesso.`);
      await load(resource);
    } catch (failure) {
      handleError(failure);
      notify("Não foi possível salvar o registro.", "error");
    }
  };
  const remove = async (row: Row) => {
    if (
      !resource ||
      !confirm(`Excluir ${resource.singular.toLowerCase()} #${row.id}?`)
    )
      return;
    try {
      await request(`${resource.path}/${row.id}`, { method: "DELETE" });
      notify("Registro excluído.");
      await load(resource);
    } catch (failure) {
      handleError(failure);
      notify("Não foi possível excluir o registro.", "error");
    }
  };
  const setDefault = async (row: Row) => {
    if (!resource) return;
    try {
      await request(`${resource.path}/${row.id}/set-default`, {
        method: "POST",
      });
      notify(`${resource.singular} definido como padrão.`);
      await load(resource);
    } catch (failure) {
      handleError(failure);
      notify("Não foi possível alterar o padrão.", "error");
    }
  };
  const testConnection = async (row: Row) => {
    if (!resource) return;
    try {
      const result = await request(`${resource.path}/${row.id}/test`, {
        method: "POST",
      });
      notify(
        `Conexão validada: ${result.provider} respondeu HTTP ${result.status}.`,
      );
    } catch (failure) {
      handleError(failure);
      notify("A conexão não respondeu corretamente.", "error");
    }
  };
  const filtered = useMemo(() => {
    const term = query.trim().toLowerCase();
    return term
      ? rows.filter((row) =>
          Object.values(row).some((value) =>
            String(value ?? "")
              .toLowerCase()
              .includes(term),
          ),
        )
      : rows;
  }, [query, rows]);
  if (!logged)
    return (
      <LoginPage
        username={username}
        password={password}
        error={error}
        theme={theme}
        setUsername={setUsername}
        setPassword={setPassword}
        onSubmit={login}
        onThemeToggle={() =>
          setTheme((value) => (value === "dark" ? "light" : "dark"))
        }
      />
    );
  const [title, description] = viewCopy[view];
  return (
    <div className="app-shell">
      <Sidebar
        username={username}
        active={view}
        onNavigate={navigate}
        onLogout={logout}
      />
      <main className="workspace">
        <PageHeader
          title={resource?.name || title}
          description={resource?.description || description}
          theme={theme}
          onThemeToggle={() =>
            setTheme((value) => (value === "dark" ? "light" : "dark"))
          }
        />
        {error && (
          <div className="alert">
            <span>!</span>
            <p>{error}</p>
            <button onClick={() => setError("")}>×</button>
          </div>
        )}
        {view === "dashboard" && (
          <DashboardPage
            request={request}
            status={status}
            operational={operational}
            onStartSetup={() => navigate("setup")}
            onObserve={() => navigate("observability")}
            onAdvanced={() => navigate("advanced")}
          />
        )}{" "}
        {view === "setup" && (
          <SetupWizardPage
            request={request}
            onComplete={() => {
              notify("Configuração criada e ativada com sucesso.");
              showDashboard();
            }}
          />
        )}
        {view === "observability" && <ObservabilityPage request={request} />}{" "}
        {view === "advanced" && resource && (
          <section className="advanced-page">
            <div className="resource-switcher">
              {resources.map((item) => (
                <button
                  key={item.path}
                  className={resource.path === item.path ? "active" : ""}
                  onClick={() => selectResource(item)}
                >
                  <span>{item.icon}</span>
                  {item.name}
                </button>
              ))}
            </div>
            <ResourceTable
              resource={resource}
              rows={filtered}
              references={referenceRows}
              total={rows.length}
              loading={loading}
              query={query}
              setQuery={setQuery}
              onReload={() =>
                Promise.all([load(resource), loadReferences(resource)])
              }
              onCreate={() => openForm()}
              onEdit={openForm}
              onDelete={remove}
              onDefault={setDefault}
              onTest={testConnection}
            />
          </section>
        )}
      </main>
      {editing && resource && (
        <ResourceDrawer
          resource={resource}
          editingID={editingID}
          form={form}
          references={referenceRows}
          setField={(key, value) =>
            setForm((current) => ({ ...current, [key]: value }))
          }
          onClose={() => setEditing(false)}
          onSave={save}
        />
      )}
      <Toast state={toast} />
    </div>
  );
}
