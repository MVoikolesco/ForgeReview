import { Cable, ChevronLeft, LogOut, Settings2 } from "lucide-react";
import { Brand } from "@/components/brand";

type Props = {
  collapsed: boolean;
  onCollapse: () => void;
  onLogout: () => void;
  onNew: () => void;
};

export function Sidebar({ collapsed, onCollapse, onLogout, onNew }: Props) {
  return (
    <aside className={`sidebar ${collapsed ? "collapsed" : ""}`}>
      <div className="sidebar-top"><Brand compact={collapsed} /></div>
      <nav aria-label="Navegação principal">
        <span className="nav-caption">{collapsed ? "" : "Workspace"}</span>
        <button className="nav-item active"><Cable size={19} /><span>Conexões</span></button>
        <button className="nav-item muted" title="Em breve"><Settings2 size={19} /><span>Configurações</span><small>breve</small></button>
      </nav>
      <div className="sidebar-callout">
        <span>+</span><strong>Nova rota de IA</strong><small>Conecte outro provider ao workspace.</small>
        <button onClick={onNew}>Cadastrar conexão</button>
      </div>
      <div className="sidebar-bottom">
        <button className="nav-item" onClick={onLogout}><LogOut size={19} /><span>Sair</span></button>
        <button className="collapse-button" onClick={onCollapse} aria-label="Recolher menu"><ChevronLeft size={18} /></button>
      </div>
    </aside>
  );
}
