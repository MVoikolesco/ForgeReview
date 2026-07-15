export type Theme = "light" | "dark";

type Props = {
  theme: Theme;
  onToggle: () => void;
};

export function ThemeToggle({ theme, onToggle }: Props) {
  const dark = theme === "dark";
  return (
    <button
      type="button"
      className="theme-toggle"
      onClick={onToggle}
      aria-label={dark ? "Ativar tema claro" : "Ativar tema escuro"}
      title={dark ? "Tema claro" : "Tema escuro"}
    >
      <span aria-hidden="true">{dark ? "☀" : "☾"}</span>
      <span>{dark ? "Claro" : "Escuro"}</span>
    </button>
  );
}
