import { Check, ChevronRight, X } from "lucide-react";
import type { ReactNode } from "react";

export type StepperStep = readonly [title: string, subtitle: string];

export function StepperModal({
  steps,
  step,
  eyebrow,
  title,
  description,
  brand,
  heading,
  railDescription,
  footnote,
  error,
  onClose,
  children,
  footer,
  className = "",
}: {
  steps: readonly StepperStep[];
  step: number;
  eyebrow: string;
  title: string;
  description: string;
  brand: ReactNode;
  heading: string;
  railDescription: string;
  footnote: string;
  error?: string;
  onClose: () => void;
  children: ReactNode;
  footer: ReactNode;
  className?: string;
}) {
  return (
    <div
      className="wizard-overlay"
      role="dialog"
      aria-modal="true"
      aria-label={heading}
    >
      <div className={`wizard-modal ${className}`.trim()}>
        <aside className="wizard-rail">
          <div>
            <span className="rail-logo">{brand}</span>
            <strong>{heading}</strong>
            <p>{railDescription}</p>
          </div>
          <ol>
            {steps.map(([stepTitle, subtitle], index) => (
              <li
                key={stepTitle}
                className={
                  index === step ? "active" : index < step ? "complete" : ""
                }
              >
                <span>{index < step ? <Check size={15} /> : index + 1}</span>
                <div>
                  <strong>{stepTitle}</strong>
                  <small>{subtitle}</small>
                </div>
                {index === step && <ChevronRight size={16} />}
              </li>
            ))}
          </ol>
          <small className="rail-footnote">{footnote}</small>
        </aside>
        <div className="wizard-content">
          <header>
            <div>
              <span className="eyebrow">{eyebrow}</span>
              <h2>{title}</h2>
              <p>{description}</p>
            </div>
            <button
              className="close-button"
              onClick={onClose}
              aria-label="Fechar"
            >
              <X size={20} />
            </button>
          </header>
          {error && <div className="banner error">{error}</div>}
          <div className="wizard-body">{children}</div>
          {footer}
        </div>
      </div>
    </div>
  );
}
