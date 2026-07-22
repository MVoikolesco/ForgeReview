"use client";

import { Braces, Plus } from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";
import { getIntegrations, getModelProfiles } from "../../lib/api";
import type { Integration, ModelProfile } from "../../lib/types";
import { ConnectionWizard } from "./ConnectionWizard";
import { IntegrationList } from "./IntegrationList";
import styles from "./IntegrationsWorkspace.module.scss";
import { useCurrentUser } from "../auth/AuthGate";

export function IntegrationsWorkspace() {
	const user = useCurrentUser();
  const [items, setItems] = useState<Integration[]>([]);
  const [profiles, setProfiles] = useState<ModelProfile[]>([]);
  const [showWizard, setShowWizard] = useState(false);
  const [message, setMessage] = useState("Carregando conexões...");
  const load = async () => {
    try {
      const [connections, modelProfiles] = await Promise.all([getIntegrations(), getModelProfiles()]);
      setItems(connections);
      setProfiles(modelProfiles);
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
              Cadastre conexões reutilizáveis com Token/API key enviado uma vez,
              sem armazenamento no navegador.
            </p>
          </div>
          <button disabled={user?.role !== "admin"} onClick={() => setShowWizard(true)}>
            <Plus size={16} /> Nova conexão
          </button>
        </div>
        {message ? (
          <p className={styles.message} role="status">
            {message}
          </p>
        ) : (
            <IntegrationList items={items} profiles={profiles} role={user?.role} onChanged={() => void load()} />
        )}
      </section>
      {showWizard && (
        <ConnectionWizard
          items={items}
          onClose={() => setShowWizard(false)}
          onCreated={(item) => {
            setItems((all) => [...all, item]);
            void load();
            setMessage("");
          }}
        />
      )}
    </main>
  );
}
