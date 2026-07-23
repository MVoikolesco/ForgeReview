"use client";

import { Plus } from "lucide-react";
import { useEffect, useState } from "react";
import { getIntegrations, getModelProfiles } from "../../lib/api";
import type { Integration, ModelProfile } from "../../lib/types";
import { ConnectionWizard } from "./ConnectionWizard";
import { IntegrationList } from "./IntegrationList";
import styles from "./IntegrationsWorkspace.module.scss";
import { useCurrentUser } from "../auth/AuthGate";
import { AppShell } from "../shell/AppShell";

export function IntegrationsWorkspace() {
	const user = useCurrentUser();
  const [items, setItems] = useState<Integration[]>([]);
  const [profiles, setProfiles] = useState<ModelProfile[]>([]);
  const [showWizard, setShowWizard] = useState(false);
  const [message, setMessage] = useState("Carregando conexões...");
  const [feedback, setFeedback] = useState("");
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
    <AppShell title="Integrações e modelos" eyebrow="CONEXÕES" actions={<button className={styles.newConnection} disabled={user?.role !== "admin"} onClick={() => setShowWizard(true)}><Plus size={16} /> Nova conexão</button>}>
      <section className={styles.content}>
        <div className={styles.intro}>
          <div>
            <p>
              Cadastre conexões reutilizáveis com Token/API key enviado uma vez,
              sem armazenamento no navegador.
            </p>
          </div>
        </div>
        {message ? (
          <p className={styles.message} role="status">
            {message}
          </p>
        ) : (
            <>{feedback && <p className={styles.feedback} role="status">{feedback}</p>}<IntegrationList items={items} profiles={profiles} role={user?.role} onChanged={() => void load()} /></>
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
            setFeedback("Conexão criada e recursos selecionados.");
          }}
        />
      )}
    </AppShell>
  );
}
