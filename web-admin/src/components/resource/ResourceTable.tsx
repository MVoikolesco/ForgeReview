import type { Resource, Row } from "../../types";

const boolKeys = [
  "unload_model_after_review",
  "log_sensitive_data",
  "review_wip_pull_requests",
  "review_own_pull_requests",
  "publish_manual_reviews",
  "allow_autonomous_rejection",
];
const isBoolean = (key: string) =>
  key.startsWith("is_") ||
  key.startsWith("supports_") ||
  boolKeys.includes(key);
const friendly = (key: string) =>
  (
    ({
      id: "ID",
      name: "Nome",
      display_name: "Nome",
      base_url: "URL base",
      auth_type: "Autenticação",
      provider_id: "Provider",
      connection_id: "Conexão",
      model_id: "Modelo",
      profile_id: "Profile",
      review_profile_id: "Profile de revisão",
      gitea_instance_id: "Instância Gitea",
      provider_model_name: "Modelo no provider",
      full_name: "Repositório",
      prompt_type: "Tipo",
      version: "Versão",
      stack: "Stack",
      temperature: "Temperature",
      top_p: "Top P",
      keep_alive: "Keep alive",
      timeout_seconds: "Timeout",
    }) as Record<string, string>
  )[key] || key.replace(/_/g, " ");

type Props = {
  resource: Resource;
  rows: Row[];
  references: Record<string, Row[]>;
  total: number;
  loading: boolean;
  query: string;
  setQuery: (v: string) => void;
  onReload: () => void;
  onCreate: () => void;
  onEdit: (r: Row) => void;
  onDelete: (r: Row) => void;
  onDefault: (r: Row) => void;
  onTest: (r: Row) => void;
};

function displayValue(
  resource: Resource,
  key: string,
  value: any,
  references: Record<string, Row[]>,
) {
  const field = resource.fields.find((item) => item.key === key);
  if (field?.options)
    return (
      field.options.find((option) => String(option.value) === String(value))
        ?.label ?? value
    );
  if (!field?.reference) return value;
  const related = (references[field.reference.path] || []).find(
    (row) => String(row.id) === String(value),
  );
  return related
    ? field.reference.labelKeys
        .map((labelKey) => String(related[labelKey] ?? "").trim())
        .filter(Boolean)
        .join(" — ")
    : value;
}

export function ResourceTable(p: Props) {
  const keys = p.rows.length
    ? Object.keys(p.rows[0]).filter(
        (k) => !["content", "created_at", "updated_at"].includes(k),
      )
    : [];
  return (
    <section className="resource-card">
      <div className="resource-toolbar">
        <div className="search">
          <span>⌕</span>
          <input
            value={p.query}
            onChange={(e) => p.setQuery(e.target.value)}
            placeholder="Buscar registros..."
          />
        </div>
        <div className="toolbar-actions">
          <button className="button button-secondary" onClick={p.onReload}>
            ↻ Atualizar
          </button>
          {!p.resource.seeded && (
            <button className="button button-primary" onClick={p.onCreate}>
              <span>+</span> Novo {p.resource.singular}
            </button>
          )}
        </div>
      </div>
      {p.loading ? (
        <div className="loading-state">
          <span />
          <p>Carregando registros...</p>
        </div>
      ) : !p.rows.length ? (
        <div className="empty-state">
          <span className="empty-icon">{p.resource.icon}</span>
          <h3>Nenhum registro encontrado</h3>
          <p>
            {p.query
              ? "Tente buscar por outro termo."
              : `Cadastre o primeiro ${p.resource.singular.toLowerCase()} para começar.`}
          </p>
          {!p.resource.seeded && !p.query && (
            <button className="button button-primary" onClick={p.onCreate}>
              Criar registro
            </button>
          )}
        </div>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                {keys.map((key) => (
                  <th key={key}>{friendly(key)}</th>
                ))}
                <th className="actions-head">Ações</th>
              </tr>
            </thead>
            <tbody>
              {p.rows.map((row) => (
                <tr key={row.id}>
                  {keys.map((key) => {
                    const shown = displayValue(
                      p.resource,
                      key,
                      row[key],
                      p.references,
                    );
                    return (
                      <td key={key}>
                        {isBoolean(key) ? (
                          <span
                            className={`badge ${Number(row[key]) ? "badge-success" : "badge-neutral"}`}
                          >
                            <i />
                            {Number(row[key]) ? "Sim" : "Não"}
                          </span>
                        ) : key === "id" ? (
                          <span className="id-cell">#{row[key]}</span>
                        ) : (
                          <span title={String(shown ?? "")}>
                            {shown ?? "—"}
                          </span>
                        )}
                      </td>
                    );
                  })}
                  <td className="row-actions">
                    <button onClick={() => p.onEdit(row)}>Editar</button>
                    {p.resource.canTest && (
                      <button onClick={() => p.onTest(row)}>Testar</button>
                    )}
                    {p.resource.canDefault && (
                      <button onClick={() => p.onDefault(row)}>Padrão</button>
                    )}
                    {!p.resource.seeded && (
                      <button
                        className="delete-action"
                        onClick={() => p.onDelete(row)}
                      >
                        Excluir
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}{" "}
      {!!p.rows.length && (
        <footer className="table-footer">
          <span>
            Exibindo {p.rows.length} de {p.total} registros
          </span>
          <span>Dados administrativos do SQLite</span>
        </footer>
      )}
    </section>
  );
}
