"use client";

import { RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { getWorkflows, publishWorkflow } from "../../lib/api";
import type { WorkflowSummary } from "../../lib/types";
import { PipelinesList } from "./PipelinesList";
import styles from "./PipelinesWorkspace.module.scss";
import { AppShell } from "../shell/AppShell";

export function PipelinesWorkspace() {
  const [items, setItems] = useState<WorkflowSummary[]>([]);
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(true);
  const [publishingID, setPublishingID] = useState<number>();

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

  return (
    <AppShell title="Pipelines e versões" eyebrow="CICLO DE VIDA" actions={<button className={styles.refresh} disabled={Boolean(publishingID)} onClick={() => void load()}><RefreshCw size={16} /> Atualizar</button>}>
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
              onPublish={(versionID) => void publish(versionID)}
            />
          </>
        )}
      </section>
    </AppShell>
  );
}
