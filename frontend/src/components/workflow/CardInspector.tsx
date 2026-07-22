import type { CardData, Integration, ModelProfile } from "../../lib/types";
import { configList, configNumber, configText } from "../../lib/workflow";
import styles from "./CardInspector.module.scss";

export function CardInspector({
  selected,
  integrations,
  modelProfiles,
  hasErrorRoute,
  onChange,
}: {
  selected: CardData;
  integrations: Integration[];
  modelProfiles: ModelProfile[];
  hasErrorRoute: boolean;
  onChange: (patch: Partial<CardData>) => void;
}) {
  const updateConfig = (key: string, value: unknown) =>
    onChange({ config: { ...selected.config, [key]: value } });
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
        <span>CONFIGURAÇÃO</span>
        <b>{selected.name}</b>
      </header>
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
            <label>
              <input
                type="checkbox"
                checked={selected.config.allow_autonomous_rejection === true}
                onChange={(event) => updateConfig("allow_autonomous_rejection", event.target.checked)}
              />
              Permitir solicitar mudanças para achados altos/críticos
            </label>
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
            <label className={styles.check}>
              <input
                type="checkbox"
                checked={Boolean(selected.config.ignore_generated)}
                onChange={(event) =>
                  updateConfig("ignore_generated", event.target.checked)
                }
              />{" "}
              Ignorar arquivos gerados
            </label>
          </>
        )}
        {selected.type === "group" && (
          <>
            {numberField("max_files", "Máximo de arquivos", 8)}
            {numberField("max_characters", "Máximo de caracteres", 12000)}
            <label className={styles.check}>
              <input
                type="checkbox"
                checked={Boolean(selected.config.group_by_extension)}
                onChange={(event) =>
                  updateConfig("group_by_extension", event.target.checked)
                }
              />{" "}
              Separar por extensão
            </label>
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
          <label className={styles.check}>
            <input
              type="checkbox"
              checked={Boolean(selected.config.validate_paths)}
              onChange={(event) =>
                updateConfig("validate_paths", event.target.checked)
              }
            />{" "}
            Validar caminho contra arquivos do PR
          </label>
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
    </aside>
  );
}
