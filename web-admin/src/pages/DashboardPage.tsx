import type { AdminRequest } from "../api/adminClient";
import { ConfigurationHealth } from "../components/dashboard/ConfigurationHealth";

type Props = {
  status: Record<string, any>;
  operational: Record<string, any>;
  request: AdminRequest;
  onStartSetup: () => void;
  onObserve: () => void;
  onAdvanced: () => void;
};

export function DashboardPage({
  status,
  operational,
  request,
  onStartSetup,
  onObserve,
  onAdvanced,
}: Props) {
  const workers = operational?.queue?.workers || [];
  const readyWorkers = workers.filter(
    (worker: any) => worker.state === "idle" || worker.state === "processing",
  ).length;
  const checklist = [
    ["Conexão de IA", status.connections > 0],
    ["Modelo", status.models > 0],
    ["Parâmetros", status.parameters > 0],
    ["Profile padrão", Boolean(status.default_profile)],
    ["Policy", status.policies > 0],
  ] as const;
  const complete = checklist.filter(([, done]) => done).length;
  const isConfigured = complete === checklist.length;
  return (
    <>
      <section className="hero-card dashboard-hero">
        <div>
          <span className="eyebrow">Central de operações</span>
          <h2>
            {status.configured
              ? "ForgeReview está pronto para revisar."
              : "Conclua a configuração do reviewer."}
          </h2>
          <p>
            {status.configured
              ? `As revisões estão usando ${status.default_provider} com o modelo ${status.default_model}.`
              : "Use o assistente guiado para conectar o provider, escolher o modelo e definir o comportamento do review."}
          </p>
          <div className="hero-actions">
            <button className="button button-primary" onClick={onStartSetup}>
              {status.configured ? "Nova configuração" : "Iniciar configuração"}
            </button>
            <button className="button button-secondary" onClick={onObserve}>
              Ver operação
            </button>
          </div>
        </div>
        <span
          className={`config-state ${status.configured ? "ready" : "pending"}`}
        >
          <i />
          {status.configured ? "Configuração ativa" : "Ação necessária"}
        </span>
      </section>
      <section className="metric-grid functional-metrics">
        <article>
          <div className="metric-icon violet">AI</div>
          <div>
            <span>Provider ativo</span>
            <strong className="metric-word">
              {status.default_provider || "—"}
            </strong>
            <small>{status.connections || 0} conexões cadastradas</small>
          </div>
        </article>
        <article>
          <div className="metric-icon blue">WK</div>
          <div>
            <span>Workers online</span>
            <strong>{readyWorkers}</strong>
            <small>
              {workers.some((w: any) => w.state === "processing")
                ? "processando agora"
                : "aguardando jobs"}
            </small>
          </div>
        </article>
        <article>
          <div className="metric-icon emerald">Q</div>
          <div>
            <span>Fila pendente</span>
            <strong>{operational?.queue?.pending || 0}</strong>
            <small>
              {operational?.queue?.stream_length || 0} jobs na stream
            </small>
          </div>
        </article>
        <article>
          <div className="metric-icon amber">PR</div>
          <div>
            <span>Reviews registrados</span>
            <strong>{operational?.reviews?.total || 0}</strong>
            <small>{status.repositories || 0} repositórios mapeados</small>
          </div>
        </article>
      </section>
      <section className="dashboard-main-layout">
        <article className="panel current-config">
          <div className="panel-heading">
            <div>
              <span className="section-kicker">Execução padrão</span>
              <h3>Rota ativa de revisão</h3>
            </div>
            <span className="status-dot" />
          </div>
          <dl>
            <div>
              <dt>Provider</dt>
              <dd>{status.default_provider || "Não configurado"}</dd>
            </div>
            <div>
              <dt>Modelo</dt>
              <dd>{status.default_model || "Não configurado"}</dd>
            </div>
            <div>
              <dt>Profile</dt>
              <dd>{status.default_profile || "Não configurado"}</dd>
            </div>
            <div>
              <dt>Worker</dt>
              <dd>
                {readyWorkers ? `${readyWorkers} online` : "Sem heartbeat"}
              </dd>
            </div>
          </dl>
          <div className="panel-actions">
            <button className="text-action" onClick={onObserve}>
              Abrir observabilidade <span>→</span>
            </button>
            <button className="text-action subtle" onClick={onAdvanced}>
              Cadastros avançados
            </button>
          </div>
        </article>
        <div className="dashboard-config-area">
          {!isConfigured && (
            <article className="panel setup-health">
              <div className="panel-heading">
                <div>
                  <span className="section-kicker">Saúde da configuração</span>
                  <h3>
                    {complete} de {checklist.length} etapas concluídas
                  </h3>
                </div>
                <span className="progress-badge">
                  {Math.round((complete / checklist.length) * 100)}%
                </span>
              </div>
              <div className="progress-track">
                <span
                  style={{ width: `${(complete / checklist.length) * 100}%` }}
                />
              </div>
              <ul>
                {checklist.map(([label, done]) => (
                  <li key={label} className={done ? "done" : ""}>
                    <span>{done ? "✓" : "·"}</span>
                    {label}
                  </li>
                ))}
              </ul>
              <button className="text-action" onClick={onStartSetup}>
                Abrir configuração guiada <span>→</span>
              </button>
            </article>
          )}
          <ConfigurationHealth request={request} onAdvanced={onAdvanced} />
        </div>
      </section>
    </>
  );
}
