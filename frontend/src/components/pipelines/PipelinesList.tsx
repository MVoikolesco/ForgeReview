import { Trash2 } from "lucide-react";
import type { WorkflowSummary } from "../../lib/types";
import {
  canPublishVersion,
  workflowVersionStatusLabel,
} from "../../lib/workflow";
import styles from "./PipelinesList.module.scss";

type PipelinesListProps = {
  items: WorkflowSummary[];
  publishingID?: number;
  deletingID?: number;
  canDelete: boolean;
  onPublish: (versionID: number) => void;
  onDelete: (workflow: WorkflowSummary, version: WorkflowSummary["versions"][number]) => void;
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
  deletingID,
  canDelete,
  onPublish,
  onDelete,
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
                  <Link href={version.status === "published" ? `/studio?workflow=${encodeURIComponent(workflow.key)}` : `/studio?version=${version.version_id}`}>
                    <Pencil size={13} /> Abrir no Studio
                 </Link>
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
                 {canDelete && version.status !== "published" && (
                  <button
                    className={styles.delete}
                    disabled={Boolean(deletingID)}
                    onClick={() => onDelete(workflow, version)}
                  >
                    <Trash2 size={13} /> {deletingID === version.version_id ? "Excluindo..." : "Excluir versão"}
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
import { Pencil } from "lucide-react";
import Link from "next/link";
