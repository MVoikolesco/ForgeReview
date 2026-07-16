"use client";

import { ConnectionWizard } from "@/components/connections/connection-wizard";
import { ConnectionsDashboard } from "@/components/connections/connections-dashboard";
import { ExecutionsFlow } from "@/components/executions/executions-flow";
import { ThemeToggle, type Theme } from "@/components/theme-toggle";
import { createAdminClient } from "@/lib/admin-client";
import type { ConnectionData } from "@/lib/contracts";
import { Menu, Plus, RefreshCw } from "lucide-react";
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
  const [collapsed, setCollapsed] = useState(false);
  const [mobileMenu, setMobileMenu] = useState(false);
  const [view, setView] = useState<ConsoleView>("connections");

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
            <span className="breadcrumb">Workspace /</span>{" "}
            {view === "connections" ? "Conexões" : "Execuções"}
          </div>
          <div className="topbar-actions">
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
                  onClick={() => setWizardOpen(true)}
                >
                  <Plus size={18} />
                  Nova conexão
                </button>
              </div>
              {error && <div className="banner error">{error}</div>}
              <ConnectionsDashboard
                data={data}
                request={request}
                loading={loading}
                onRefresh={refresh}
                onNew={() => setWizardOpen(true)}
              />
            </>
          ) : (
            <>
              <div className="page-heading execution-heading">
                <div>
                  <span className="eyebrow">Observabilidade</span>
                  <h1>Execuções</h1>
                  <p>
                    Acompanhe cada etapa da revisão e os logs produzidos pelo
                    pipeline.
                  </p>
                </div>
              </div>
              <ExecutionsFlow request={request} onAuthError={onLogout} />
            </>
          )}
        </div>
      </main>
      {wizardOpen && (
        <ConnectionWizard
          request={request}
          existingProfiles={data.profiles.length}
          onClose={() => setWizardOpen(false)}
          onComplete={async () => {
            setWizardOpen(false);
            await refresh();
          }}
        />
      )}
    </div>
  );
}
