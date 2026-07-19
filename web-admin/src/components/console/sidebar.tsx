import { Brand } from "@/components/brand";
import { Cable, ChevronLeft, GitBranch, LogOut, Settings2 } from "lucide-react";

export type ConsoleView = "connections" | "executions" | "gitea" | "settings";

type Props = {
  collapsed: boolean;
  onCollapse: () => void;
  onLogout: () => void;
  onNew: () => void;
  active: ConsoleView;
  onNavigate: (view: ConsoleView) => void;
};

export function Sidebar({
  collapsed,
  onCollapse,
  onLogout,
  onNew,
  active,
  onNavigate,
}: Props) {
  return (
    <aside className={`sidebar ${collapsed ? "collapsed" : ""}`}>
      <div className="sidebar-top">
        <Brand compact={collapsed} />
      </div>
      <nav aria-label="Navegação principal">
        <span className="nav-caption">{collapsed ? "" : "Workspace"}</span>
        <button
          className={`nav-item ${active === "connections" ? "active" : ""}`}
          aria-current={active === "connections" ? "page" : undefined}
          onClick={() => onNavigate("connections")}
        >
          <Cable size={19} />
          <span>Conexões</span>
        </button>
        <button
          className={`nav-item ${active === "gitea" ? "active" : ""}`}
          aria-current={active === "gitea" ? "page" : undefined}
          onClick={() => onNavigate("gitea")}
        >
          <GitBranch size={19} />
          <span>Gitea</span>
        </button>
        <button
          className={`nav-item ${active === "executions" ? "active" : ""}`}
          aria-current={active === "executions" ? "page" : undefined}
          onClick={() => onNavigate("executions")}
        >
          <GitBranch size={19} />
          <span>Execuções</span>
        </button>
        <button
          className={`nav-item ${active === "settings" ? "active" : ""}`}
          aria-current={active === "settings" ? "page" : undefined}
          onClick={() => onNavigate("settings")}
        >
          <Settings2 size={19} />
          <span>Configurações</span>
        </button>
      </nav>
      <div className="sidebar-callout">
        <span>+</span>
        <strong>Nova rota de IA</strong>
        <small>Conecte outro provider ao workspace.</small>
        <button onClick={onNew}>Cadastrar conexão</button>
      </div>
      <div className="sidebar-bottom">
        <button className="nav-item" onClick={onLogout}>
          <LogOut size={19} />
          <span>Sair</span>
        </button>
        <button
          className="collapse-button"
          onClick={onCollapse}
          aria-label="Recolher menu"
        >
          <ChevronLeft size={18} />
        </button>
      </div>
    </aside>
  );
}
