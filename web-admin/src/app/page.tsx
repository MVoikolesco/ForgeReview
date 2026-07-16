"use client";

import { useEffect, useState } from "react";
import { Login } from "@/components/auth/login";
import { Console } from "@/components/console/console";
import type { Theme } from "@/components/theme-toggle";

type Credentials = { username: string; password: string };

export default function Home() {
  const [credentials, setCredentials] = useState<Credentials | null>(null);
  const [restored, setRestored] = useState(false);
  const [theme, setTheme] = useState<Theme>("light");

  useEffect(() => {
    const username = sessionStorage.getItem("fr.user");
    const password = sessionStorage.getItem("fr.pass");
    const savedTheme = localStorage.getItem("fr.theme") as Theme | null;
    const initialTheme = savedTheme || (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
    document.documentElement.dataset.theme = initialTheme;
    setTheme(initialTheme);
    if (username && password) setCredentials({ username, password });
    setRestored(true);
  }, []);

  function signIn(next: Credentials) {
    sessionStorage.setItem("fr.user", next.username);
    sessionStorage.setItem("fr.pass", next.password);
    setCredentials(next);
  }

  function signOut() {
    sessionStorage.removeItem("fr.user");
    sessionStorage.removeItem("fr.pass");
    setCredentials(null);
  }

  function toggleTheme() {
    const nextTheme = theme === "light" ? "dark" : "light";
    document.documentElement.dataset.theme = nextTheme;
    localStorage.setItem("fr.theme", nextTheme);
    setTheme(nextTheme);
  }

  if (!restored) return <div className="app-loading" aria-label="Carregando" />;
  if (!credentials) return <Login onLogin={signIn} theme={theme} onThemeToggle={toggleTheme} />;
  return <Console credentials={credentials} onLogout={signOut} theme={theme} onThemeToggle={toggleTheme} />;
}
