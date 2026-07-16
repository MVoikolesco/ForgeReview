import { Moon, Sun } from "lucide-react";

export type Theme = "light" | "dark";

export function ThemeToggle({ theme, onToggle, className = "" }: { theme: Theme; onToggle: () => void; className?: string }) {
  const nextTheme = theme === "light" ? "escuro" : "claro";
  return (
    <button className={`theme-toggle ${className}`} onClick={onToggle} aria-label={`Ativar modo ${nextTheme}`} title={`Modo ${nextTheme}`}>
      <Sun size={16} />
      <span className="theme-toggle-track"><i /></span>
      <Moon size={15} />
    </button>
  );
}
