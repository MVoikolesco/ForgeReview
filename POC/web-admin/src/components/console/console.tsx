"use client";

import { ConnectionWizard } from "@/components/connections/connection-wizard";
import { ConnectionsDashboard } from "@/components/connections/connections-dashboard";
import { ExecutionsFlow } from "@/components/executions/executions-flow";
import { GiteaArea } from "@/components/gitea/gitea-area";
import { ManualReviewModal } from "@/components/manual-review/manual-review-modal";
import { SettingsArea } from "@/components/settings/settings-area";
import { ThemeToggle, type Theme } from "@/components/theme-toggle";
import { createAdminClient } from "@/lib/admin-client";
import type { Connection, ConnectionData } from "@/lib/contracts";
import { Edit3, Menu, Play, RefreshCw, RotateCcw, Save, Send } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { ConsoleView } from "./sidebar";
import { Sidebar } from "./sidebar";

type Props = {
  credentials: { username: string; password: string };
  onLogout: () => void;
  theme: Theme;
  onThemeToggle: () => void;
};

const emptyData: ConnectionData = {
  providers: [],
  connections: [],
  models: [],
  profiles: [],
};

const viewLabels: Record<ConsoleView, string> = {
  connections: "Conexões",
  gitea: "Gitea",
  executions: "Pipeline",
  settings: "Configurações",
};

export function Console({
  credentials,
  onLogout,
  theme,
  onThemeToggle,
}: Props) {
  const request = useMemo(
    () => createAdminClient(credentials.username, credentials.password),
    [credentials],
  );
  const [data, setData] = useState<ConnectionData>(emptyData);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [wizardOpen, setWizardOpen] = useState(false);
  const [manualReviewOpen, setManualReviewOpen] = useState(false);
  const [toast, setToast] = useState<{
    message: string;
    error?: boolean;
  } | null>(null);
  const [wizardConnection, setWizardConnection] = useState<Connection | null>(
    null,
  );
  const [collapsed, setCollapsed] = useState(false);
  const [mobileMenu, setMobileMenu] = useState(false);
  const [view, setView] = useState<ConsoleView>("connections");
  const [pipelineName, setPipelineName] = useState("");
  const [pipelineEditing, setPipelineEditing] = useState(false);
  const [pipelineEditRequest, setPipelineEditRequest] = useState(0);
  const [pipelineSaveRequest, setPipelineSaveRequest] = useState(0);
  const [pipelinePublishRequest, setPipelinePublishRequest] = useState(0);
  const [pipelineDiscardRequest, setPipelineDiscardRequest] = useState(0);

  const refresh = useCallback(async () => {
    try {
      const [providers, connections, models, profiles] = await Promise.all([
        request<ConnectionData["providers"]>("ai/providers"),
        request<ConnectionData["connections"]>("ai/connections"),
        request<ConnectionData["models"]>("ai/models"),
        request<ConnectionData["profiles"]>("review/profiles"),
      ]);
      setData({ providers, connections, models, profiles });
      setError("");
    } catch (failure) {
      if (failure instanceof Error && failure.message === "AUTH")
        return onLogout();
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setLoading(false);
    }
  }, [onLogout, request]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return (
    <div
      className={`console-shell ${collapsed ? "sidebar-is-collapsed" : ""} ${mobileMenu ? "mobile-menu-open" : ""}`}
    >
      <Sidebar
        collapsed={collapsed}
        onCollapse={() => setCollapsed((value) => !value)}
        onLogout={onLogout}
        onNew={() => setWizardOpen(true)}
        active={view}
        onNavigate={(nextView) => {
          setView(nextView);
          if (nextView !== "executions") setPipelineName("");
          if (nextView !== "executions") setPipelineEditing(false);
          setMobileMenu(false);
        }}
      />
      {mobileMenu && (
        <button
          className="menu-backdrop"
          onClick={() => setMobileMenu(false)}
          aria-label="Fechar menu"
        />
      )}
      <main className="console-main">
        <header className="topbar">
          <button
            className="mobile-menu-button"
            onClick={() => setMobileMenu(true)}
            aria-label="Abrir menu"
          >
            <Menu size={21} />
          </button>
          <div>
            <span className="breadcrumb">Workspace /</span> {view === "executions" ? "Pipeline" : viewLabels[view]}{view === "executions" && pipelineName ? ` / ${pipelineName}` : ""}
          </div>
          <div className="topbar-actions">
            {view === "executions" && (pipelineEditing ? <><button className="secondary-button" onClick={() => setPipelineDiscardRequest((value) => value + 1)}><RotateCcw size={16} /> Fechar edição</button><button className="secondary-button" onClick={() => setPipelineSaveRequest((value) => value + 1)}><Save size={16} /> Salvar draft</button><button className="primary-button" onClick={() => setPipelinePublishRequest((value) => value + 1)}><Send size={16} /> Publicar</button></> : <button className="primary-button" onClick={() => setPipelineEditRequest((value) => value + 1)}><Edit3 size={16} /> Editar pipeline</button>)}
            <ThemeToggle theme={theme} onToggle={onThemeToggle} />
            {view === "connections" && (
              <button
                className="icon-button"
                onClick={() => void refresh()}
                title="Atualizar"
              >
                <RefreshCw size={18} />
              </button>
            )}
            <span className="user-avatar">
              {credentials.username.slice(0, 2).toUpperCase()}
            </span>
          </div>
        </header>
        <div className="page-content">
          {view === "connections" ? (
            <>
              <div className="page-heading">
                <div>
                  <span className="eyebrow">Infraestrutura de IA</span>
                  <h1>Conexões</h1>
                  <p>
                    Gerencie providers e defina a rota usada nas próximas
                    revisões.
                  </p>
                </div>
                <button
                  className="primary-button"
                  onClick={() => setManualReviewOpen(true)}
                >
                  <Play size={18} />
                  Disparar review manual
                </button>
              </div>
              {error && <div className="banner error">{error}</div>}
              <ConnectionsDashboard
                data={data}
                request={request}
                loading={loading}
                onRefresh={refresh}
                onNew={() => setWizardOpen(true)}
                onAddModel={(connection) => {
                  setWizardConnection(connection);
                  setWizardOpen(true);
                }}
              />
            </>
          ) : view === "gitea" ? (
            <>
              <div className="page-heading">
                <div>
                  <span className="eyebrow">Integrações</span>
                  <h1>Gitea</h1>
                </div>
              </div>
              <GiteaArea request={request} onAuthError={onLogout} />
            </>
          ) : view === "executions" ? (
            <ExecutionsFlow request={request} onAuthError={onLogout} onPipelineName={setPipelineName} editRequest={pipelineEditRequest} saveRequest={pipelineSaveRequest} publishRequest={pipelinePublishRequest} discardRequest={pipelineDiscardRequest} onEditingChange={setPipelineEditing} />
          ) : (
            <>
              <div className="page-heading">
                <div>
                  <span className="eyebrow">Governança do workspace</span>
                  <h1>Configurações</h1>
                  <p>
                    Visualize os perfis, pipelines e contratos que orientam as
                    próximas revisões.
                  </p>
                </div>
              </div>
              <SettingsArea request={request} onAuthError={onLogout} />
            </>
          )}
        </div>
      </main>
      {toast && (
        <div
          className={`console-toast ${toast.error ? "error" : "success"}`}
          role={toast.error ? "alert" : "status"}
        >
          {toast.message}
        </div>
      )}
      {wizardOpen && (
        <ConnectionWizard
          request={request}
          existingProfiles={data.profiles.length}
          existingConnection={
            wizardConnection
              ? {
                  id: wizardConnection.id,
                  provider: data.providers.find(
                    (item) => item.id === wizardConnection.provider_id,
                  )?.name as "ollama" | "openrouter",
                  name: wizardConnection.name,
                  base_url: wizardConnection.base_url,
                  api_key_configured: wizardConnection.api_key_configured,
                  http_referer: wizardConnection.http_referer,
                  app_title: wizardConnection.app_title,
                }
              : undefined
          }
          onClose={() => {
            setWizardOpen(false);
            setWizardConnection(null);
          }}
          onComplete={async () => {
            setWizardOpen(false);
            setWizardConnection(null);
            await refresh();
          }}
        />
      )}
      {manualReviewOpen && (
        <ManualReviewModal
          request={request}
          onAuthError={onLogout}
          onClose={() => setManualReviewOpen(false)}
          onSuccess={(message) => {
            setToast({ message });
            window.setTimeout(() => setToast(null), 5000);
          }}
          onFailure={(message) => {
            setToast({ message, error: true });
            window.setTimeout(() => setToast(null), 5000);
          }}
        />
      )}
    </div>
  );
}
