"use client";

import { Braces, Menu, X } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { breadcrumbs, visibleNavigation } from "../../lib/navigation";
import { useCurrentUser } from "../auth/AuthGate";
import styles from "./AppShell.module.scss";

export function AppShell({ title, eyebrow, actions, headerActions, compact = false, children }: { title: string; eyebrow?: string; actions?: React.ReactNode; headerActions?: React.ReactNode; compact?: boolean; children: React.ReactNode }) {
  const user = useCurrentUser(); const pathname = usePathname(); const [open, setOpen] = useState(false); const button = useRef<HTMLButtonElement>(null);
  useEffect(() => { const close = (event: KeyboardEvent) => { if (event.key === "Escape") { setOpen(false); button.current?.focus(); } }; window.addEventListener("keydown", close); return () => window.removeEventListener("keydown", close); }, []);
  const trail = breadcrumbs(pathname, title);
  return <main className={styles.shell}>
    <header className={styles.header}><Link className={styles.brand} href="/"><Braces size={18} /> ForgeReview <small>CONTROL</small></Link>
      <button ref={button} className={styles.menuButton} onClick={() => setOpen(!open)} aria-expanded={open} aria-controls="primary-navigation">{open ? <X /> : <Menu />}<span>Menu</span></button>
      <nav id="primary-navigation" className={open ? styles.open : ""} aria-label="Navegação principal">{visibleNavigation(user?.role).map((item) => <Link key={item.href} href={item.href} aria-current={pathname === item.href ? "page" : undefined} onClick={() => setOpen(false)}>{item.label}</Link>)}</nav>
      <div className={styles.headerRight}><span className={styles.role}>{user?.role}</span>{headerActions && <div className={styles.headerActions}>{headerActions}</div>}</div>
    </header>
    {compact ? children : <section className={styles.content}>
      <div className={styles.pageHeading}><div><nav className={styles.breadcrumbs} aria-label="Breadcrumb">{trail.map((item, index) => <span key={item.label}>{item.href ? <Link href={item.href}>{item.label}</Link> : <span aria-current="page" title={item.label}>{item.label}</span>}{index < trail.length - 1 && <b aria-hidden="true">/</b>}</span>)}</nav>{eyebrow && <p>{eyebrow}</p>}<h1>{title}</h1></div>{actions && <div className={styles.actions}>{actions}</div>}</div>
      {children}
    </section>}
  </main>;
}
