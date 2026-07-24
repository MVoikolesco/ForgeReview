import type { ReactNode } from "react";
import styles from "./ToggleSwitch.module.scss";

export function ToggleSwitch({
  checked,
  onChange,
  label,
  description,
  disabled = false,
  leading,
  variant = "default",
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
  description?: string;
  disabled?: boolean;
  leading?: ReactNode;
  variant?: "default" | "card";
}) {
  return (
    <label
      className={`${styles.toggle} ${variant === "card" ? styles.card : ""} ${checked ? styles.selected : ""}`}
    >
      {leading && <span className={styles.leading} aria-hidden="true">{leading}</span>}
      <span className={styles.copy}>
        <strong>{label}</strong>
        {description && <small>{description}</small>}
      </span>
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span className={styles.track} aria-hidden="true"><span /></span>
    </label>
  );
}
