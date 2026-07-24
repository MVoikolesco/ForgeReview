"use client";

import { FormEvent, useState } from "react";
import { ArrowRight, Eye, EyeOff, LockKeyhole, ShieldCheck } from "lucide-react";
import { Brand } from "@/components/brand";
import { createAdminClient } from "@/lib/admin-client";
import { ThemeToggle, type Theme } from "@/components/theme-toggle";

type Props = { onLogin: (credentials: { username: string; password: string }) => void; theme: Theme; onThemeToggle: () => void };

export function Login({ onLogin, theme, onThemeToggle }: Props) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await createAdminClient(username, password)<unknown>("status");
      onLogin({ username, password });
    } catch (failure) {
      setError(
        failure instanceof Error && failure.message === "AUTH"
          ? "Usuário ou senha incorretos."
          : "Não foi possível acessar o servidor.",
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="login-page">
      <section className="login-story">
        <Brand />
        <div className="story-copy">
          <span className="eyebrow light">Console de inteligência</span>
          <h1>Reviews melhores começam com a rota certa.</h1>
          <p>Conecte seus modelos, defina padrões e mantenha cada revisão sob controle.</p>
        </div>
        <div className="story-status">
          <span className="pulse-dot" />
          <div><strong>Ambiente protegido</strong><small>Credenciais enviadas por conexão segura</small></div>
          <ShieldCheck size={21} />
        </div>
        <span className="story-grid" aria-hidden="true" />
      </section>

      <section className="login-panel">
        <ThemeToggle theme={theme} onToggle={onThemeToggle} className="login-theme-toggle" />
        <div className="login-form-wrap">
          <div className="mobile-brand"><Brand /></div>
          <span className="login-icon"><LockKeyhole size={22} /></span>
          <h2>Bem-vindo de volta</h2>
          <p>Entre para gerenciar o ForgeReview.</p>
          <form onSubmit={submit}>
            <label>Usuário
              <input autoFocus autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} placeholder="Seu usuário" required />
            </label>
            <label>Senha
              <span className="password-field">
                <input type={showPassword ? "text" : "password"} autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="Sua senha" required />
                <button type="button" onClick={() => setShowPassword((value) => !value)} aria-label={showPassword ? "Ocultar senha" : "Mostrar senha"}>
                  {showPassword ? <EyeOff size={18} /> : <Eye size={18} />}
                </button>
              </span>
            </label>
            {error && <div className="form-error" role="alert">{error}</div>}
            <button className="primary-button login-submit" disabled={busy}>
              {busy ? "Verificando..." : "Entrar no console"}<ArrowRight size={18} />
            </button>
          </form>
          <small className="login-footnote">Acesso restrito a administradores autorizados.</small>
        </div>
      </section>
    </main>
  );
}
