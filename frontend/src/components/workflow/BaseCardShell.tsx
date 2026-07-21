import type { CSSProperties, ReactNode } from "react";
import styles from "./BaseCardShell.module.scss";

type BaseCardShellProps = {
  accent: string;
  status: string;
  category: string;
  children: ReactNode;
};

const statusLabel: Record<string, string> = {
  idle: "rascunho",
  running: "executando",
  completed: "concluído",
  failed: "falhou",
};

export function BaseCardShell({
  accent,
  status,
  category,
  children,
}: BaseCardShellProps) {
  return (
    <article
      className={`${styles.card} ${styles[status] ?? ""}`}
      style={{ "--accent": accent } as CSSProperties}
    >
      <header>
        <i />
        <span>{category}</span>
        <em>{statusLabel[status] ?? status}</em>
      </header>
      {children}
    </article>
  );
}
