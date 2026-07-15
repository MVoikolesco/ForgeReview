export type AppView = "dashboard" | "setup" | "observability" | "advanced";
type Props = {
  username: string;
  active: AppView;
  onNavigate: (view: AppView) => void;
  onLogout: () => void;
};
const navigation = [
  {
    view: "dashboard",
    icon: "⌂",
    label: "Visão geral",
    help: "Status do ambiente",
  },
  {
    view: "setup",
    icon: "＋",
    label: "Configurar IA",
    help: "Assistente guiado",
  },
  {
    view: "observability",
    icon: "◉",
    label: "Operação",
    help: "Workers, fila e logs",
  },
  {
    view: "advanced",
    icon: "⚙",
    label: "Avançado",
    help: "Cadastros individuais",
  },
] as const;

export function Sidebar({ username, active, onNavigate, onLogout }: Props) {
  return (
    <aside className="sidebar">
      <button className="brand" onClick={() => onNavigate("dashboard")}>
        <span className="brand-mark">F</span>
        <span>
          <strong>ForgeReview</strong>
          <small>Administration</small>
        </span>
      </button>
      <nav>
        <div className="nav-group">
          <span className="nav-label">Workspace</span>
          {navigation.map((item) => (
            <button
              className={`nav-item nav-item-rich ${active === item.view ? "active" : ""}`}
              key={item.view}
              onClick={() => onNavigate(item.view)}
            >
              <span className="nav-icon">{item.icon}</span>
              <span>
                <strong>{item.label}</strong>
                <small>{item.help}</small>
              </span>
            </button>
          ))}
        </div>
      </nav>
      <div className="sidebar-user">
        <span className="user-avatar">
          {username.slice(0, 1).toUpperCase()}
        </span>
        <span className="user-meta">
          <strong>{username}</strong>
          <small>Administrador</small>
        </span>
        <button title="Sair" onClick={onLogout}>
          ↗
        </button>
      </div>
    </aside>
  );
}
