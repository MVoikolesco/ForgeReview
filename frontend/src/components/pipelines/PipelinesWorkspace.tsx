"use client";

import { Braces, RefreshCw } from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";
import { getWorkflows, publishWorkflow } from "../../lib/api";
import type { WorkflowSummary } from "../../lib/types";
import { PipelinesList } from "./PipelinesList";
import styles from "./PipelinesWorkspace.module.scss";

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
    <main className={styles.page}>
      <header>
        <strong>
          <Braces size={20} /> ForgeReview <small>PIPELINES</small>
        </strong>
        <nav aria-label="Navegação principal">
          <Link href="/studio">Studio</Link>
          <Link href="/integrations">Integrações</Link>
        </nav>
      </header>
      <section className={styles.content}>
        <div className={styles.intro}>
          <div>
            <span>CICLO DE VIDA</span>
            <h1>Pipelines e versões</h1>
            <p>
              Rascunhos podem ser publicados. A publicação arquiva a versão
              publicada anterior do mesmo pipeline.
            </p>
          </div>
          <button disabled={Boolean(publishingID)} onClick={() => void load()}>
            <RefreshCw size={16} /> Atualizar
          </button>
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
    </main>
  );
}
