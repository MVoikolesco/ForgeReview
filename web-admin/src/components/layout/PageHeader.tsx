import { ThemeToggle, type Theme } from "../ui/ThemeToggle";

type Props = {
  title: string;
  description: string;
  theme: Theme;
  onThemeToggle: () => void;
};

export function PageHeader({
  title,
  description,
  theme,
  onThemeToggle,
}: Props) {
  return (
    <header className="topbar">
      <div>
        <span className="breadcrumb">
          ForgeReview <b>/</b> {title}
        </span>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      <div className="topbar-actions">
        <span className="environment">
          <i /> Ambiente operacional
        </span>
        <ThemeToggle theme={theme} onToggle={onThemeToggle} />
      </div>
    </header>
  );
}
