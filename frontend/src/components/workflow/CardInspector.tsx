import { useState } from "react";
import { X } from "lucide-react";
import type { CardData, Integration, ModelProfile, WebhookRegistration } from "../../lib/types";
import { configList, configNumber, configText } from "../../lib/workflow";
import { ToggleSwitch } from "../common/ToggleSwitch";
import styles from "./CardInspector.module.scss";

export function CardInspector({
  selected,
  integrations,
  modelProfiles,
  hasErrorRoute,
  onChange,
  readOnly = false,
  webhookRegistrations = [],
  canAdministerWebhooks = false,
  workflowKey,
  onRegisterWebhook,
  onClose,
}: {
  selected: CardData;
  integrations: Integration[];
  modelProfiles: ModelProfile[];
  hasErrorRoute: boolean;
  onChange: (patch: Partial<CardData>) => void;
  readOnly?: boolean;
  webhookRegistrations?: WebhookRegistration[];
  canAdministerWebhooks?: boolean;
  workflowKey: string;
  onRegisterWebhook?: (input: { key: string; name: string; workflow_key: string; trigger_node_key: string; secret: string; active: boolean }) => Promise<void>;
  onClose: () => void;
}) {
  const [webhookKey, setWebhookKey] = useState("");
  const [webhookName, setWebhookName] = useState("");
  const [webhookSecret, setWebhookSecret] = useState("");
  const [webhookBusy, setWebhookBusy] = useState(false);
  const updateConfig = (key: string, value: unknown) =>
    !readOnly && onChange({ config: { ...selected.config, [key]: value } });
  const gitea = integrations.filter(
    (item) => item.type === "gitea" && item.status === "active",
  );
  const models = modelProfiles.filter((item) => item.status === "active");
  const connectionFields = (
    <label>
      {selected.type === "model" ? "Perfil de modelo" : "Integração"}
      <select
        value={configText(selected.config[selected.type === "model" ? "model_profile" : "integration"])}
        onChange={(event) =>
          updateConfig(selected.type === "model" ? "model_profile" : "integration", event.target.value)
        }
      >
        <option value="">{selected.type === "model" ? "Selecione um perfil" : "Selecione uma conexão"}</option>
        {(selected.type === "model" ? models : gitea).map((item) => (
          <option key={item.key} value={item.key}>
            {item.name}
            {selected.type === "model" && "model" in item ? ` · ${item.model}` : ""}
          </option>
        ))}
      </select>
    </label>
  );
  const numberField = (key: string, label: string, fallback: number) => (
    <label>
      {label}
      <input
        type="number"
        min="1"
        value={configNumber(selected.config[key], fallback)}
        onChange={(event) =>
          updateConfig(key, event.target.valueAsNumber || fallback)
        }
      />
    </label>
  );
  const boundedNumberField = (
    key: string,
    label: string,
    fallback: number,
    max: number,
  ) => (
    <label>
      {label}
      <input
        type="number"
        min="0"
        max={max}
        value={configNumber(selected.config[key], fallback)}
        onChange={(event) => updateConfig(key, event.target.valueAsNumber || 0)}
      />
    </label>
  );
  return (
    <aside
      className={styles.inspector}
      aria-label="Inspector do card selecionado"
    >
      <header>
        <div>
          <span>CONFIGURAÇÃO</span>
          <b>{selected.name}</b>
        </div>
        <button type="button" onClick={onClose} aria-label="Fechar Inspector" title="Fechar Inspector">
          <X size={16} aria-hidden="true" />
        </button>
      </header>
      <fieldset className={styles.fields} disabled={readOnly}>
      <label>
        Nome do card
        <input
          value={selected.name}
          onChange={(event) => onChange({ name: event.target.value })}
        />
      </label>
      <dl>
        <dt>Tipo</dt>
        <dd>{selected.type}</dd>
        <dt>Portas</dt>
        <dd>
          {selected.inputs.length} entrada(s) · {selected.outputs.length + (selected.config.on_error === "route" && selected.errorOutput ? 1 : 0)}{" "}
          saída(s)
        </dd>
      </dl>
      <section className={styles.config}>
        <strong>Parâmetros</strong>
        {selected.type === "trigger" && (
          <>
            <label>
              Modo de acionamento
              <select value={configText(selected.config.mode) || "manual"} onChange={(event) => updateConfig("mode", event.target.value)}>
                <option value="manual">Manual (Studio)</option>
                <option value="api">API autenticada</option>
                <option value="webhook">Webhook Gitea</option>
              </select>
            </label>
            {(configText(selected.config.mode) || "manual") === "api" && (
              <small>Após publicar, envie POST para <code>/api/workflows/{workflowKey}/triggers/{selected.key}/executions</code> com um objeto JSON.</small>
            )}
            {configText(selected.config.mode) === "webhook" && (
              <section>
                <small>Publique este trigger antes de registrar. O segredo é criptografado separadamente e nunca volta para o navegador.</small>
                {webhookRegistrations.filter((item) => item.workflow_key === workflowKey && item.trigger_node_key === selected.key).map((item) => (
                  <p key={item.key}><b>{item.name}</b> · <code>/webhooks/gitea/{item.key}</code> · {item.active ? "ativo" : "inativo"}</p>
                ))}
                {canAdministerWebhooks && (
                  <>
                    <label>Chave pública<input value={webhookKey} onChange={(event) => setWebhookKey(event.target.value)} placeholder="gitea-review" /></label>
                    <label>Nome<input value={webhookName} onChange={(event) => setWebhookName(event.target.value)} placeholder="Gitea principal" /></label>
                    <label>Segredo de assinatura<input type="password" autoComplete="new-password" value={webhookSecret} onChange={(event) => setWebhookSecret(event.target.value)} /></label>
                    <button type="button" disabled={webhookBusy || !webhookKey || !webhookName || !webhookSecret} onClick={async () => {
                      if (!onRegisterWebhook) return;
                      setWebhookBusy(true);
                      try { await onRegisterWebhook({ key: webhookKey, name: webhookName, workflow_key: workflowKey, trigger_node_key: selected.key, secret: webhookSecret, active: true }); setWebhookSecret(""); }
                      finally { setWebhookBusy(false); }
                    }}>{webhookBusy ? "Registrando..." : "Registrar webhook"}</button>
                  </>
                )}
              </section>
            )}
          </>
        )}
        {selected.type === "template" && (
          <label>
            Template
            <textarea
              value={configText(selected.config.template)}
              onChange={(event) => updateConfig("template", event.target.value)}
              placeholder="Descreva a revisão e use variáveis declaradas."
            />
          </label>
        )}
        {selected.type === "fetch" && (
          <>
            {connectionFields}
            <label>
              Organização
              <input
                value={configText(selected.config.owner)}
                onChange={(event) => updateConfig("owner", event.target.value)}
                placeholder="acme"
              />
            </label>
            <label>
              Repositório
              <input
                value={configText(selected.config.repo)}
                onChange={(event) => updateConfig("repo", event.target.value)}
                placeholder="api"
              />
            </label>
            {numberField("pull_request", "Número do PR", 0)}
          </>
        )}
        {selected.type === "model" && (
          <>
            {connectionFields}
            {numberField("max_tokens", "Máximo de tokens", 2000)}
            {boundedNumberField("retry_limit", "Tentativas de correção", 0, 3)}
            {boundedNumberField("retry_delay_ms", "Espera entre tentativas (ms)", 0, 60000)}
            <small>
              Quando a saída for ligada diretamente a Validar e falhar no formato,
              o modelo repete o prompt com uma instrução de correção. Zero desativa;
              no máximo 3 tentativas e 60.000 ms de espera.
            </small>
          </>
        )}
        {selected.type === "publish" && (
          <>
            {connectionFields}
            <label>
              Organização
              <input
                value={configText(selected.config.owner)}
                onChange={(event) => updateConfig("owner", event.target.value)}
                placeholder="acme"
              />
            </label>
            <label>
              Repositório
              <input
                value={configText(selected.config.repo)}
                onChange={(event) => updateConfig("repo", event.target.value)}
                placeholder="api"
              />
            </label>
            {numberField("pull_request", "Número do PR", 0)}
            <label>
              Evento para severidade média
              <select
                value={configText(selected.config.medium_severity_event) || "COMMENT"}
                onChange={(event) => updateConfig("medium_severity_event", event.target.value)}
              >
                <option value="COMMENT">Comentar</option>
                <option value="REQUEST_CHANGES">Solicitar mudanças</option>
              </select>
            </label>
            <ToggleSwitch
              checked={selected.config.allow_autonomous_rejection === true}
              onChange={(checked) => updateConfig("allow_autonomous_rejection", checked)}
              label="Permitir rejeição autônoma"
              description="Achados altos ou críticos poderão solicitar mudanças automaticamente."
            />
          </>
        )}
        {selected.type === "filter" && (
          <>
            <label>
              Extensões incluídas
              <input
                value={configList(selected.config.include_extensions)}
                onChange={(event) =>
                  updateConfig(
                    "include_extensions",
                    event.target.value
                      .split(",")
                      .map((item) => item.trim())
                      .filter(Boolean),
                  )
                }
                placeholder=".go, .ts, .tsx"
              />
            </label>
            <label>
              Extensões excluídas
              <input
                value={configList(selected.config.exclude_extensions)}
                onChange={(event) =>
                  updateConfig(
                    "exclude_extensions",
                    event.target.value
                      .split(",")
                      .map((item) => item.trim())
                      .filter(Boolean),
                  )
                }
                placeholder=".min.js, .lock"
              />
            </label>
            <ToggleSwitch checked={Boolean(selected.config.ignore_generated)} onChange={(checked) => updateConfig("ignore_generated", checked)} label="Ignorar arquivos gerados" description="Remove artefatos gerados antes da análise." />
          </>
        )}
        {selected.type === "group" && (
          <>
            {numberField("max_files", "Máximo de arquivos", 8)}
            {numberField("max_characters", "Máximo de caracteres", 12000)}
            <ToggleSwitch checked={Boolean(selected.config.group_by_extension)} onChange={(checked) => updateConfig("group_by_extension", checked)} label="Separar por extensão" description="Mantém linguagens diferentes em grupos distintos." />
          </>
        )}
        {selected.type === "loop" && (
          <>
            {numberField("max_iterations", "Máximo de iterações", 20)}
            <label>
              Concorrência
              <input value="1" disabled />
              <small>
                A execução por grupo é sequencial nesta fase para preservar a
                ordem e a rastreabilidade dos resultados.
              </small>
            </label>
            <p>
              Use <code>item</code> para os cards do grupo e conecte apenas
              <code>results</code> ao consolidar. Assim consolidar, formatar e
              publicar executam uma vez no escopo raiz.
            </p>
          </>
        )}
        {selected.type === "cache" && (
          <>
            <label>
              Chave do cache
              <input
                value={configText(selected.config.key)}
                onChange={(event) =>
                  onChange({
                    config: {
                      ...selected.config,
                      key: event.target.value,
                      mode: configText(selected.config.mode) || "read",
                    },
                  })
                }
                placeholder="review:pr:42"
              />
            </label>
            <label>
              Operação
              <select
                value={configText(selected.config.mode) || "read"}
                onChange={(event) => {
                  const mode = event.target.value;
                  onChange({
                    config: {
                      ...selected.config,
                      mode,
                      ...(mode === "write" && selected.config.ttl_seconds === undefined
                        ? { ttl_seconds: 3600 }
                        : {}),
                    },
                  });
                }}
              >
                <option value="read">Ler</option>
                <option value="write">Gravar</option>
                <option value="delete">Excluir</option>
              </select>
            </label>
            {(configText(selected.config.mode) || "read") === "write" && (
              <label>
                TTL (segundos)
                <input
                  type="number"
                  min="1"
                  max="86400"
                  value={configNumber(selected.config.ttl_seconds, 3600)}
                  onChange={(event) =>
                    updateConfig("ttl_seconds", event.target.valueAsNumber || 1)
                  }
                />
              </label>
            )}
          </>
        )}
        {selected.type === "error_control" && (
          <>
            <label>
              Ao receber erro roteado
              <select
                value={configText(selected.config.on_error) || "continue"}
                onChange={(event) => updateConfig("on_error", event.target.value)}
              >
                <option value="fail">Encerrar workflow</option>
                <option value="continue">Encerrar rota e continuar</option>
                <option value="fallback">Emitir resultado de fallback</option>
              </select>
            </label>
            {configText(selected.config.on_error) === "fallback" && (
              <label>
                Resultado de fallback
                <input
                  value={configText(selected.config.fallback_result)}
                  onChange={(event) => updateConfig("fallback_result", event.target.value)}
                  placeholder="Resultado seguro"
                />
              </label>
            )}
          </>
        )}
        {selected.type === "condition" && (
          <label>
            Valor esperado
            <input
              value={configText(selected.config.equals)}
              onChange={(event) => updateConfig("equals", event.target.value)}
              placeholder="php"
            />
          </label>
        )}
        {selected.type === "validate" && (
          <ToggleSwitch checked={Boolean(selected.config.validate_paths)} onChange={(checked) => updateConfig("validate_paths", checked)} label="Validar caminhos do PR" description="Recusa achados para arquivos ausentes nos dados buscados." />
        )}
        {selected.type === "response_filter" && (
          <label>
            Severidade mínima
            <select
              value={configText(selected.config.minimum_severity) || "low"}
              onChange={(event) =>
                updateConfig("minimum_severity", event.target.value)
              }
            >
              <option value="low">Baixa</option>
              <option value="medium">Média</option>
              <option value="high">Alta</option>
              <option value="critical">Crítica</option>
            </select>
          </label>
        )}
        {![
          "template",
          "trigger",
          "fetch",
          "model",
          "publish",
          "filter",
          "group",
          "loop",
          "cache",
          "condition",
          "validate",
          "response_filter",
          "error_control",
        ].includes(selected.type) && (
          <p>Este card não possui parâmetros obrigatórios nesta fase.</p>
        )}
      </section>
      {selected.type !== "error_control" && selected.errorOutput && (
        <section className={styles.errorPolicy}>
          <strong>Política de erro</strong>
          <label>
            Ao falhar
            <select
              value={configText(selected.config.on_error) || "fail"}
              onChange={(event) => updateConfig("on_error", event.target.value)}
            >
              <option value="fail">Interromper workflow</option>
              <option value="continue">Continuar sem saída</option>
              <option value="partial">Continuar parcialmente</option>
              <option value="route">Rotear para porta Erro</option>
            </select>
          </label>
          {configText(selected.config.on_error) === "route" && (
            <p className={hasErrorRoute ? styles.valid : styles.invalid}>
              {hasErrorRoute
                ? "A rota Erro está conectada a uma porta tipada."
                : "Conecte a nova porta Erro a uma entrada compatível do grafo antes de salvar."}
            </p>
          )}
        </section>
      )}
      <p className={styles.note}>
        O backend valida cada conexão e configuração antes de executar uma
        versão.
      </p>
      </fieldset>
    </aside>
  );
}
