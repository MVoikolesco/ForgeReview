const steps = [
  "Provider",
  "Conexão",
  "Modelo",
  "Parâmetros",
  "Review",
  "Confirmar",
];

export function WizardProgress({ current }: { current: number }) {
  return (
    <ol className="wizard-progress" aria-label="Progresso da configuração">
      {steps.map((label, index) => (
        <li
          key={label}
          className={
            index === current ? "active" : index < current ? "done" : ""
          }
        >
          <span>{index < current ? "✓" : index + 1}</span>
          <small>{label}</small>
        </li>
      ))}
    </ol>
  );
}
