import type { WorkflowSummary } from "../../lib/types";
import {
  canPublishVersion,
  workflowVersionStatusLabel,
} from "../../lib/workflow";
import styles from "./PipelinesList.module.scss";

type PipelinesListProps = {
  items: WorkflowSummary[];
  publishingID?: number;
  onPublish: (versionID: number) => void;
};

const formattedDate = (value: string) => {
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? value
    : new Intl.DateTimeFormat("pt-BR", {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(date);
};

export function PipelinesList({
  items,
  publishingID,
  onPublish,
}: PipelinesListProps) {
  if (!items.length)
    return (
      <p className={styles.empty}>
        Nenhum pipeline salvo. Crie um rascunho no Studio para vê-lo aqui.
      </p>
    );

  return (
    <div className={styles.list}>
      {items.map((workflow) => (
        <article key={workflow.key}>
          <header>
            <div>
              <span>PIPELINE</span>
              <h2>{workflow.name}</h2>
              <p>{workflow.description || "Sem descrição."}</p>
            </div>
            <code>{workflow.key}</code>
          </header>
          <ol aria-label={`Versões de ${workflow.name}`}>
            {workflow.versions.map((version) => (
              <li key={version.version_id}>
                <div>
                  <strong>Versão {version.version}</strong>
                  <time dateTime={version.created_at}>
                    {formattedDate(version.created_at)}
                  </time>
                </div>
                <span className={styles[version.status]}>
                  {workflowVersionStatusLabel(version.status)}
                </span>
                {canPublishVersion(version.status) && (
                  <button
                    disabled={publishingID === version.version_id}
                    onClick={() => onPublish(version.version_id)}
                  >
                    {publishingID === version.version_id
                      ? "Publicando..."
                      : "Publicar"}
                  </button>
                )}
              </li>
            ))}
          </ol>
        </article>
      ))}
    </div>
  );
}
