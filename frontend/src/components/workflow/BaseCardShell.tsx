import type { CSSProperties, ReactNode } from "react";
import styles from "./BaseCardShell.module.scss";

type BaseCardShellProps = {
  accent: string;
  status: string;
  category: string;
  actions?: ReactNode;
  compact?: boolean;
  children: ReactNode;
};

const statusLabel: Record<string, string> = {
  idle: "rascunho",
  running: "executando",
  completed: "concluído",
  partial: "parcial",
  failed: "falhou",
};

export function BaseCardShell({
  accent,
  status,
  category,
  actions,
  compact = false,
  children,
}: BaseCardShellProps) {
  return (
    <article
      className={`${styles.card} ${styles[status] ?? ""} ${compact ? styles.compact : ""}`}
      style={{ "--accent": accent } as CSSProperties}
    >
      <header>
        <i />
        <span>{category}</span>
        <em>{statusLabel[status] ?? status}</em>
        {actions}
      </header>
      {children}
    </article>
  );
}
