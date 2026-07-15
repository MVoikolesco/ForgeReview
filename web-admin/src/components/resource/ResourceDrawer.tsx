import { FormEvent } from "react";
import type { Field, Resource, Row, SelectOption } from "../../types";

type Props = {
  resource: Resource;
  editingID: number | null;
  form: Record<string, any>;
  references: Record<string, Row[]>;
  setField: (k: string, v: any) => void;
  onClose: () => void;
  onSave: (e: FormEvent) => void;
};

const referenceLabel = (row: Row, keys: string[]) =>
  keys
    .map((key) => String(row[key] ?? "").trim())
    .filter(Boolean)
    .join(" — ") || "Registro sem nome";

function fieldOptions(
  field: Field,
  references: Record<string, Row[]>,
): SelectOption[] {
  if (field.options) return field.options;
  if (!field.reference) return [];
  return (references[field.reference.path] || []).map((row) => ({
    value: row.id,
    label: referenceLabel(row, field.reference!.labelKeys),
  }));
}

export function ResourceDrawer(p: Props) {
  return (
    <div
      className="drawer-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) p.onClose();
      }}
    >
      <aside className="drawer">
        <header>
          <div>
            <span className="section-kicker">
              {p.editingID ? "Editar registro" : "Novo registro"}
            </span>
            <h2>{p.resource.singular}</h2>
            <p>{p.resource.description}</p>
          </div>
          <button className="drawer-close" onClick={p.onClose}>
            ×
          </button>
        </header>
        <form onSubmit={p.onSave}>
          <div className="form-body">
            {p.resource.fields.map((field) => {
              const options = fieldOptions(field, p.references);
              const selectable = Boolean(field.reference || field.options);
              return (
                <label
                  key={field.key}
                  className={field.type === "boolean" ? "toggle" : ""}
                >
                  <span className="field-copy">
                    <strong>
                      {field.label} {field.required && <b>*</b>}
                    </strong>
                    {field.hint && <small>{field.hint}</small>}
                  </span>
                  {field.type === "boolean" ? (
                    <input
                      checked={Boolean(p.form[field.key])}
                      onChange={(e) => p.setField(field.key, e.target.checked)}
                      type="checkbox"
                    />
                  ) : field.type === "textarea" ? (
                    <textarea
                      value={p.form[field.key] ?? ""}
                      onChange={(e) => p.setField(field.key, e.target.value)}
                      rows={14}
                      required={field.required}
                    />
                  ) : selectable ? (
                    <>
                      <select
                        value={p.form[field.key] ?? ""}
                        onChange={(e) =>
                          p.setField(
                            field.key,
                            field.type === "number" && e.target.value !== ""
                              ? Number(e.target.value)
                              : e.target.value,
                          )
                        }
                        required={field.required}
                      >
                        <option value="">
                          {field.reference?.placeholder ||
                            `Selecione ${field.label.toLowerCase()}`}
                        </option>
                        {options.map((option) => (
                          <option key={option.value} value={option.value}>
                            {option.label}
                          </option>
                        ))}
                      </select>
                      {field.reference && !options.length && (
                        <small className="select-empty">
                          Cadastre uma opção relacionada antes de continuar.
                        </small>
                      )}
                    </>
                  ) : (
                    <input
                      value={p.form[field.key] ?? ""}
                      onChange={(e) => p.setField(field.key, e.target.value)}
                      type={field.type === "number" ? "number" : "text"}
                      step={
                        ["temperature", "top_p", "repeat_penalty"].includes(
                          field.key,
                        )
                          ? "any"
                          : undefined
                      }
                      required={field.required}
                    />
                  )}
                </label>
              );
            })}
          </div>
          <footer>
            <button
              type="button"
              className="button button-secondary"
              onClick={p.onClose}
            >
              Cancelar
            </button>
            <button className="button button-primary">Salvar alterações</button>
          </footer>
        </form>
      </aside>
    </div>
  );
}
