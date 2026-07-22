"use client";

import { Eye, EyeOff, KeyRound, LoaderCircle, LogIn, ShieldCheck, SlidersHorizontal } from "lucide-react";
import { createContext, FormEvent, useContext, useEffect, useState } from "react";
import { loginErrorMessage } from "../../lib/auth";
import { getCurrentUser, login, logout, type CurrentUser } from "../../lib/api";
import styles from "./AuthGate.module.scss";

const AuthContext = createContext<CurrentUser | undefined>(undefined);
export const useCurrentUser = () => useContext(AuthContext);

export function AuthGate({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<CurrentUser>();
  const [ready, setReady] = useState(false);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [passwordVisible, setPasswordVisible] = useState(false);

  useEffect(() => {
    void getCurrentUser().then(setUser).catch(() => undefined).finally(() => setReady(true));
  }, []);

  if (!ready) return <main className={styles.loading} aria-live="polite"><LoaderCircle aria-hidden="true" /><span>Verificando sessão local…</span></main>;

  if (!user) return <main className={styles.page}>
    <section className={styles.intro} aria-labelledby="product-title">
      <div className={styles.wordmark}><ShieldCheck aria-hidden="true" /><span>ForgeReview</span><small>STUDIO</small></div>
      <div className={styles.introContent}>
        <p className={styles.eyebrow}>WORKFLOW CONTROL PLANE</p>
        <h1 id="product-title">Review workflows, <em>under your control.</em></h1>
        <p className={styles.description}>Acesse o Studio local para acompanhar pipelines, conexões e revisões automatizadas sem levar credenciais ao navegador.</p>
      </div>
      <div className={styles.signalList} aria-label="Características do ambiente">
        <span><SlidersHorizontal aria-hidden="true" /> Pipelines versionadas</span>
        <span><KeyRound aria-hidden="true" /> Sessão local protegida</span>
      </div>
    </section>
    <section className={styles.accessPanel} aria-labelledby="login-title">
      <div className={styles.panelHeading}>
        <p className={styles.eyebrow}>ACESSO LOCAL</p>
        <h2 id="login-title">Entrar no Studio</h2>
        <p>Use uma conta criada neste ambiente ForgeReview.</p>
      </div>
      <form className={styles.form} onSubmit={(event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        setError("");
        setSubmitting(true);
        void login(String(data.get("email")), String(data.get("password")))
          .then(setUser)
          .catch((reason: unknown) => setError(loginErrorMessage(reason)))
          .finally(() => setSubmitting(false));
      }}>
        <label htmlFor="login-email">E-mail</label>
        <input id="login-email" required name="email" type="email" autoComplete="username" inputMode="email" placeholder="voce@exemplo.com" disabled={submitting} />
        <label htmlFor="login-password">Senha</label>
        <div className={styles.passwordField}>
          <input id="login-password" required name="password" type={passwordVisible ? "text" : "password"} autoComplete="current-password" disabled={submitting} />
          <button type="button" className={styles.visibility} onClick={() => setPasswordVisible(!passwordVisible)} aria-label={passwordVisible ? "Ocultar senha" : "Mostrar senha"} aria-pressed={passwordVisible}>
            {passwordVisible ? <EyeOff aria-hidden="true" /> : <Eye aria-hidden="true" />}
          </button>
        </div>
        {error && <p className={styles.error} role="alert">{error}</p>}
        <button className={styles.submit} type="submit" disabled={submitting}>{submitting ? <LoaderCircle className={styles.spin} aria-hidden="true" /> : <LogIn aria-hidden="true" />} {submitting ? "Entrando…" : "Entrar no ForgeReview"}</button>
      </form>
      <aside className={styles.firstRun} aria-labelledby="first-run-title">
        <h3 id="first-run-title">Primeira inicialização?</h3>
        <p>Antes de iniciar o backend, defina <code>FORGEREVIEW_BOOTSTRAP_ADMIN_EMAIL</code> e <code>FORGEREVIEW_BOOTSTRAP_ADMIN_PASSWORD</code> no ambiente. Eles criam somente a primeira conta de administrador.</p>
        <p>Depois que houver usuários, alterar essas variáveis não modifica nenhuma conta. Recupere o acesso com a administração do banco conforme a documentação.</p>
      </aside>
    </section>
  </main>;

  return <AuthContext.Provider value={user}>{children}<button className={styles.logout} aria-label="Sair da sessão" onClick={() => void logout().finally(() => setUser(undefined))}>Sair <span>{user.role}</span></button></AuthContext.Provider>;
}
