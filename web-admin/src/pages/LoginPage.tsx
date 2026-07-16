import { FormEvent } from "react";
import { ThemeToggle, type Theme } from "../components/ui/ThemeToggle";

type Props = {
  username: string;
  password: string;
  error: string;
  theme: Theme;
  setUsername: (v: string) => void;
  setPassword: (v: string) => void;
  onSubmit: (e: FormEvent) => void;
  onThemeToggle: () => void;
};

export function LoginPage(p: Props) {
  return (
    <div className="auth-layout">
      <div className="auth-theme">
        <ThemeToggle theme={p.theme} onToggle={p.onThemeToggle} />
      </div>
      <section className="auth-showcase">
        <div className="auth-brand">
          <span className="brand-mark">F</span>
          <span>ForgeReview</span>
        </div>
        <div className="showcase-copy">
          <span className="eyebrow">Code review infrastructure</span>
          <h1>
            Configuração centralizada.
            <br />
            Reviews mais consistentes.
          </h1>
          <p>
            Gerencie modelos, integrações e políticas em um único ambiente
            seguro.
          </p>
        </div>
        <p className="showcase-foot">ForgeReview · Administração</p>
      </section>
      <section className="auth-panel">
        <form className="auth-card" onSubmit={p.onSubmit}>
          <div>
            <span className="mobile-brand">
              <b>F</b> ForgeReview
            </span>
            <h2>Bem-vindo de volta</h2>
            <p>Entre com suas credenciais administrativas.</p>
          </div>
          <label>
            <span>Usuário</span>
            <input
              value={p.username}
              onChange={(e) => p.setUsername(e.target.value)}
              autoComplete="username"
              placeholder="admin"
              required
            />
          </label>
          <label>
            <span>Senha</span>
            <input
              value={p.password}
              onChange={(e) => p.setPassword(e.target.value)}
              type="password"
              autoComplete="current-password"
              placeholder="••••••••"
              required
            />
          </label>
          {p.error && <p className="form-error">{p.error}</p>}
          <button className="button button-primary button-block">
            Entrar no painel <span>→</span>
          </button>
          <p className="auth-help">
            As credenciais são definidas no ambiente da aplicação.
          </p>
        </form>
      </section>
    </div>
  );
}
