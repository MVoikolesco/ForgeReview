"use client";

import {
  Activity,
  ArrowUpRight,
  CheckCircle2,
  CircleAlert,
  Clock3,
  GitPullRequest,
  Network,
  PlugZap,
  RefreshCw,
  ShieldCheck,
} from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";
import {
  apiURL,
  getExecutions,
  getHealth,
  getIntegrations,
  getModelProfiles,
  getWorkflowVersion,
  getWorkflows,
} from "../../lib/api";
import {
  publishedOfficialVersion,
  reviewPipelineState,
} from "../../lib/dashboard";
import type {
  ExecutionSummary,
  Integration,
  ModelProfile,
  WorkflowDefinition,
  WorkflowSummary,
} from "../../lib/types";
import { useCurrentUser } from "../auth/AuthGate";
import { AppShell } from "../shell/AppShell";
import styles from "./DashboardWorkspace.module.scss";

type LoadState = "loading" | "ready" | "error";

export function DashboardWorkspace() {
  const user = useCurrentUser();
  const [connections, setConnections] = useState<Integration[]>([]);
  const [profiles, setProfiles] = useState<ModelProfile[]>([]);
  const [workflows, setWorkflows] = useState<WorkflowSummary[]>([]);
  const [executions, setExecutions] = useState<ExecutionSummary[]>([]);
  const [official, setOfficial] = useState<WorkflowDefinition>();
  const [states, setStates] = useState<Record<string, LoadState>>({
    health: "loading",
    connections: "loading",
    workflows: "loading",
    executions: "loading",
  });

  const load = async () => {
    setStates({
      health: "loading",
      connections: "loading",
      workflows: "loading",
      executions: "loading",
    });
    const [health, connectionData, profileData, workflowData, executionData] =
      await Promise.allSettled([
        getHealth(),
        getIntegrations(),
        getModelProfiles(),
        getWorkflows(),
        getExecutions(10),
      ]);
    setStates({
      health: health.status === "fulfilled" ? "ready" : "error",
      connections:
        connectionData.status === "fulfilled" &&
        profileData.status === "fulfilled"
          ? "ready"
          : "error",
      workflows: workflowData.status === "fulfilled" ? "ready" : "error",
      executions: executionData.status === "fulfilled" ? "ready" : "error",
    });
    if (connectionData.status === "fulfilled")
      setConnections(connectionData.value);
    if (profileData.status === "fulfilled") setProfiles(profileData.value);
    if (workflowData.status === "fulfilled") {
      setWorkflows(workflowData.value);
      const version = publishedOfficialVersion(workflowData.value);
      if (version) {
        try {
          setOfficial(await getWorkflowVersion(version.version_id));
        } catch {
          setOfficial(undefined);
        }
      } else setOfficial(undefined);
    }
    if (executionData.status === "fulfilled")
      setExecutions(executionData.value);
  };

  useEffect(() => {
    void load();
  }, []);
  useEffect(() => {
    const source = new EventSource(`${apiURL}/api/execution-events`, {
      withCredentials: true,
    });
    source.addEventListener("execution", () => void load());
    return () => source.close();
  }, []);
  const gitea = connections.filter((item) => item.type === "gitea");
  const models = connections.filter((item) => item.type !== "gitea");
  const connectionReady = (items: Integration[]) =>
    items.some((item) => item.status === "active" && item.secret_configured);
  const pipeline = reviewPipelineState(official, connections, profiles);

  return (
    <AppShell
      title="Painel de controle"
      eyebrow="OPERAÇÕES DE REVISÃO"
      actions={
        <button className={styles.refresh} onClick={() => void load()}>
          <RefreshCw size={16} /> Atualizar
        </button>
      }
    >
      <section className={styles.hero} aria-label="Resumo operacional">
        <div>
          <p>PRONTIDÃO DE REVISÃO</p>
          <span>
            Estado seguro das conexões, da pipeline oficial e das execuções
            recentes.
          </span>
        </div>
        <div
          className={`${styles.service} ${states.health === "ready" ? styles.ok : styles.alert}`}
          role="status"
        >
          <Activity size={16} />{" "}
          {states.health === "loading"
            ? "Verificando serviço"
            : states.health === "ready"
              ? "Serviço disponível"
              : "Serviço indisponível"}
        </div>
      </section>

      <section aria-labelledby="connections-title">
        <div className={styles.sectionHeading}>
          <div>
            <p>CONEXÕES</p>
            <h2 id="connections-title">Saúde operacional</h2>
          </div>
          {user?.role === "admin" && (
            <Link href="/integrations">
              Gerenciar <ArrowUpRight size={15} />
            </Link>
          )}
        </div>
        {states.connections === "error" ? (
          <PanelError text="Não foi possível carregar o estado das conexões." />
        ) : (
          <div className={styles.connectionGrid}>
            <ConnectionCard
              icon={<PlugZap size={19} />}
              label="Gitea"
              items={gitea}
              ready={connectionReady(gitea)}
              loading={states.connections === "loading"}
            />
            <ConnectionCard
              icon={<Network size={19} />}
              label="Modelos"
              items={models}
              ready={connectionReady(models)}
              loading={states.connections === "loading"}
            />
          </div>
        )}
      </section>

      <div className={styles.mainGrid}>
        <section className={styles.pipeline} aria-labelledby="pipeline-title">
          <div className={styles.sectionHeading}>
            <div>
              <p>PIPELINE OFICIAL</p>
              <h2 id="pipeline-title">Revisão Gitea de PR</h2>
            </div>
            <Link href="/pipelines">
              Ver versões <ArrowUpRight size={15} />
            </Link>
          </div>
          {states.workflows === "error" ? (
            <PanelError text="Não foi possível carregar a pipeline oficial." />
          ) : states.workflows === "loading" ? (
            <p className={styles.muted} role="status">
              Carregando definição publicada…
            </p>
          ) : !official ? (
            <p className={styles.empty}>
              Nenhuma versão oficial publicada foi encontrada.
            </p>
          ) : (
            <>
              <div className={styles.pipelineStatus}>
                <span
                  className={
                    pipeline.readiness === "Pronta para revisão"
                      ? styles.ok
                      : styles.warning
                  }
                >
                  <CheckCircle2 size={15} /> {pipeline.readiness}
                </span>
                <span className={pipeline.safe ? styles.ok : styles.warning}>
                  <ShieldCheck size={15} />{" "}
                  {pipeline.safe
                    ? "Publicação conservadora"
                    : "Política requer revisão"}
                </span>
              </div>
              <ol
                className={styles.steps}
                aria-label="Etapas da definição publicada"
              >
                {pipeline.steps.map((step) => (
                  <li key={step}>{step}</li>
                ))}
              </ol>
              <p className={styles.note}>
                A definição publicada é exibida sem configurações ou
                credenciais.
              </p>
            </>
          )}
        </section>

        <section
          className={styles.executions}
          aria-labelledby="executions-title"
        >
          <div className={styles.sectionHeading}>
            <div>
              <p>ATIVIDADE</p>
              <h2 id="executions-title">Execuções recentes</h2>
            </div>
          </div>
          {states.executions === "error" ? (
            <PanelError text="Não foi possível carregar as execuções." />
          ) : states.executions === "loading" ? (
            <p className={styles.muted} role="status">
              Carregando execuções…
            </p>
          ) : executions.length === 0 ? (
            <p className={styles.empty}>Ainda não há execuções registradas.</p>
          ) : (
            <ul className={styles.executionList}>
              {executions.map((execution) => (
                <li key={execution.execution_id}>
                  <span
                    className={`${styles.status} ${styles[execution.status] ?? ""}`}
                    aria-label={`Status: ${execution.status}`}
                  >
                    {execution.status}
                  </span>
                  <div>
                    <strong>{execution.workflow.name}</strong>
                    <small>
                      {execution.review ? (
                        <>
                          <GitPullRequest size={13} /> {execution.review.owner}/
                          {execution.review.repo} #
                          {execution.review.pull_request}
                        </>
                      ) : (
                        "Contexto de PR não configurado"
                      )}
                    </small>
                  </div>
                  <time dateTime={execution.started_at}>
                    <Clock3 size={13} /> {formatDate(execution.started_at)}
                  </time>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </AppShell>
  );
}

function ConnectionCard({
  icon,
  label,
  items,
  ready,
  loading,
}: {
  icon: React.ReactNode;
  label: string;
  items: Integration[];
  ready: boolean;
  loading: boolean;
}) {
  return (
    <article className={styles.connectionCard}>
      <div className={styles.cardIcon}>{icon}</div>
      <div>
        <p>{label}</p>
        <strong>
          {loading ? "Carregando" : ready ? "Pronta" : "Atenção necessária"}
        </strong>
        <small>
          {loading
            ? "Consultando estado seguro…"
            : items.length
              ? `${items.length} conexão(ões) cadastrada(s)`
              : "Nenhuma conexão cadastrada"}
        </small>
      </div>
      <span className={ready ? styles.ok : styles.warning}>
        {ready ? <CheckCircle2 size={15} /> : <CircleAlert size={15} />}
      </span>
    </article>
  );
}

function PanelError({ text }: { text: string }) {
  return (
    <p className={styles.error} role="alert">
      {text}
    </p>
  );
}

function formatDate(value: string) {
  const date = new Date(
    value.replace(" ", "T") + (value.endsWith("Z") ? "" : "Z"),
  );
  return Number.isNaN(date.valueOf())
    ? "Data indisponível"
    : new Intl.DateTimeFormat("pt-BR", {
        dateStyle: "short",
        timeStyle: "short",
      }).format(date);
}
