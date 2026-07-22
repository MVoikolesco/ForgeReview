"use client";

import { createContext, FormEvent, useContext, useEffect, useState } from "react";
import { getCurrentUser, login, logout, type CurrentUser } from "../../lib/api";

const AuthContext = createContext<CurrentUser | undefined>(undefined);
export const useCurrentUser = () => useContext(AuthContext);

export function AuthGate({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<CurrentUser>();
  const [ready, setReady] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => { void getCurrentUser().then(setUser).catch(() => undefined).finally(() => setReady(true)); }, []);
  if (!ready) return <main style={{ padding: 32 }}>Carregando sessão…</main>;
  if (!user) return <main style={{ maxWidth: 360, margin: "12vh auto", padding: 24 }}><h1>ForgeReview</h1><p>Entre para acessar o ambiente local.</p><form onSubmit={(event: FormEvent<HTMLFormElement>) => { event.preventDefault(); const data = new FormData(event.currentTarget); setError(""); void login(String(data.get("email")), String(data.get("password"))).then(setUser).catch((reason: unknown) => setError(reason instanceof Error ? reason.message : "Não foi possível entrar.")); }}><label>E-mail<input required name="email" type="email" autoComplete="username" /></label><label>Senha<input required name="password" type="password" autoComplete="current-password" /></label><button type="submit">Entrar</button>{error && <p role="alert">{error}</p>}</form></main>;
  return <AuthContext.Provider value={user}>{children}<button aria-label="Sair" style={{ position: "fixed", right: 16, bottom: 16 }} onClick={() => void logout().finally(() => setUser(undefined))}>Sair ({user.role})</button></AuthContext.Provider>;
}
