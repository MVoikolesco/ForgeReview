import type { Integration, ModelProfile } from "../../lib/types";
import styles from "./IntegrationList.module.scss";

export function IntegrationList({
  items,
  profiles,
}: {
  items: Integration[];
  profiles: ModelProfile[];
}) {
  const groups = [
    {
      title: "Conexões Gitea",
      items: items.filter((item) => item.type === "gitea"),
      empty: "Nenhuma conexão Gitea cadastrada.",
    },
  ];
  return (
    <div className={styles.list}>
      <section>
        <h2>Modelos reutilizáveis</h2>
        {profiles.length ? (
          profiles.map((profile) => (
            <article key={profile.key}>
              <i />
              <strong>{profile.name}</strong>
              <small>{profile.model}</small>
              <em>{profile.status === "active" ? "Ativo" : "Desativado"}</em>
            </article>
          ))
        ) : (
          <p>Nenhum perfil cadastrado. Crie uma conexão de modelo para gerar o primeiro perfil.</p>
        )}
      </section>
      {groups.map((group) => (
        <section key={group.title}>
          <h2>{group.title}</h2>
          {group.items.length ? (
            group.items.map((item) => (
              <article key={item.key}>
                <i />
                <strong>{item.name}</strong>
                <small>
                  {item.config.base_url}
                </small>
                <em>
                  {item.secret_configured ? "Segredo configurado" : "Sem segredo"}
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
