"use client";

import { RefreshCw, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { deleteWorkflowVersion, getWorkflows, publishWorkflow } from "../../lib/api";
import type { WorkflowSummary, WorkflowVersionSummary } from "../../lib/types";
import { PipelinesList } from "./PipelinesList";
import styles from "./PipelinesWorkspace.module.scss";
import { AppShell } from "../shell/AppShell";
import { useCurrentUser } from "../auth/AuthGate";
import { ModalShell } from "../common/ModalShell";

export function PipelinesWorkspace() {
  const user = useCurrentUser();
  const [items, setItems] = useState<WorkflowSummary[]>([]);
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(true);
  const [publishingID, setPublishingID] = useState<number>();
  const [deleting, setDeleting] = useState<{ workflow: WorkflowSummary; version: WorkflowVersionSummary }>();
  const [deletingID, setDeletingID] = useState<number>();

  const load = async () => {
    setLoading(true);
    setMessage("");
    try {
      setItems(await getWorkflows());
    } catch (error) {
      setMessage(
        error instanceof Error
          ? error.message
          : "Não foi possível carregar os pipelines.",
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const publish = async (versionID: number) => {
    setPublishingID(versionID);
    setMessage("Validando e publicando a versão selecionada...");
    try {
      const published = await publishWorkflow(versionID);
      await load();
      setMessage(`Versão ${published.version} publicada.`);
    } catch (error) {
      setMessage(
        error instanceof Error
          ? error.message
          : "Não foi possível publicar a versão.",
      );
    } finally {
      setPublishingID(undefined);
    }
  };
  const remove = async () => {
    if (!deleting) return;
    setDeletingID(deleting.version.version_id);
    setMessage("Excluindo versão...");
    try {
      await deleteWorkflowVersion(deleting.version.version_id);
      setDeleting(undefined);
      await load();
      setMessage("Versão excluída.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Não foi possível excluir a versão.");
    } finally {
      setDeletingID(undefined);
    }
  };

  return (
    <AppShell title="Pipelines e versões" eyebrow="CICLO DE VIDA" actions={<button className={styles.refresh} disabled={Boolean(publishingID || deletingID)} onClick={() => void load()}><RefreshCw size={16} /> Atualizar</button>}>
      <section className={styles.content}>
        <div className={styles.intro}>
          <div>
            <p>
              Rascunhos podem ser publicados. A publicação arquiva a versão
              publicada anterior do mesmo pipeline.
            </p>
          </div>
        </div>
        {loading ? (
          <p className={styles.message} role="status">
            Carregando pipelines...
          </p>
        ) : (
          <>
            {message && (
              <p className={styles.message} role="status">
                {message}
              </p>
            )}
            <PipelinesList
              items={items}
              publishingID={publishingID}
              deletingID={deletingID}
              canDelete={user?.role === "admin"}
              onPublish={(versionID) => void publish(versionID)}
              onDelete={(workflow, version) => setDeleting({ workflow, version })}
            />
          </>
        )}
      </section>
      {deleting && (
        <ModalShell
          title="Excluir versão definitivamente?"
          eyebrow="AÇÃO IRREVERSÍVEL"
          description={`A versão ${deleting.version.version} de “${deleting.workflow.name}” será removida. A exclusão será bloqueada se houver execuções, publicações, webhooks ou auditoria que precisem ser retidos.`}
          onClose={() => !deletingID && setDeleting(undefined)}
          footer={<><button type="button" disabled={Boolean(deletingID)} onClick={() => setDeleting(undefined)}>Cancelar</button><button type="button" className={styles.deleteConfirm} disabled={Boolean(deletingID)} onClick={() => void remove()}><Trash2 size={14} /> {deletingID ? "Excluindo..." : "Excluir definitivamente"}</button></>}
        ><p role="alert">Esta ação não pode ser desfeita. Versões publicadas exigem a publicação de outra versão primeiro.</p></ModalShell>
      )}
    </AppShell>
  );
}
