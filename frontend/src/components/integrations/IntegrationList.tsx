import type { Integration } from "../../lib/types";
import styles from "./IntegrationList.module.scss";

export function IntegrationList({ items }: { items: Integration[] }) {
  const groups = [
    {
      title: "Modelos reutilizáveis",
      items: items.filter((item) => item.type !== "gitea"),
      empty:
        "Nenhum modelo cadastrado. Crie uma conexão de modelo para selecioná-la em cards de IA.",
    },
    {
      title: "Conexões Gitea",
      items: items.filter((item) => item.type === "gitea"),
      empty: "Nenhuma conexão Gitea cadastrada.",
    },
  ];
  return (
    <div className={styles.list}>
      {groups.map((group) => (
        <section key={group.title}>
          <h2>{group.title}</h2>
          {group.items.length ? (
            group.items.map((item) => (
              <article key={item.key}>
                <i />
                <strong>{item.name}</strong>
                <small>
                  {item.type === "gitea"
                    ? item.config.base_url
                    : `${item.type === "ollama" ? "Ollama" : "OpenAI-compatible"} · ${item.config.model}`}
                </small>
                <em>
                  {item.type === "gitea"
                    ? item.secret_configured
                      ? "Segredo referenciado"
                      : "Sem segredo"
                    : item.status === "active"
                      ? "Ativo"
                      : "Desativado"}
                </em>
              </article>
            ))
          ) : (
            <p>{group.empty}</p>
          )}
        </section>
      ))}
    </div>
  );
}
