"use client";

import { Braces, Plus } from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";
import { getIntegrations } from "../../lib/api";
import type { Integration } from "../../lib/types";
import { ConnectionWizard } from "./ConnectionWizard";
import { IntegrationList } from "./IntegrationList";
import styles from "./IntegrationsWorkspace.module.scss";

export function IntegrationsWorkspace() {
  const [items, setItems] = useState<Integration[]>([]);
  const [showWizard, setShowWizard] = useState(false);
  const [message, setMessage] = useState("Carregando conexões...");
  const load = async () => {
    try {
      setItems(await getIntegrations());
      setMessage("");
    } catch {
      setMessage("Não foi possível carregar as integrações.");
    }
  };
  useEffect(() => {
    void load();
  }, []);
  return (
    <main className={styles.page}>
      <header>
        <strong>
          <Braces size={20} /> ForgeReview <small>INTEGRAÇÕES</small>
        </strong>
        <nav aria-label="Navegação principal">
          <Link href="/studio">Studio</Link>
          <Link href="/pipelines">Pipelines</Link>
        </nav>
      </header>
      <section className={styles.content}>
        <div className={styles.intro}>
          <div>
            <span>CONEXÕES</span>
            <h1>Integrações e modelos</h1>
            <p>
              Cadastre conexões reutilizáveis sem expor valores de credenciais
              no navegador.
            </p>
          </div>
          <button onClick={() => setShowWizard(true)}>
            <Plus size={16} /> Nova conexão
          </button>
        </div>
        {message ? (
          <p className={styles.message} role="status">
            {message}
          </p>
        ) : (
          <IntegrationList items={items} />
        )}
      </section>
      {showWizard && (
        <ConnectionWizard
          items={items}
          onClose={() => setShowWizard(false)}
          onCreated={(item) => {
            setItems((all) => [...all, item]);
            setMessage("");
          }}
        />
      )}
    </main>
  );
}
