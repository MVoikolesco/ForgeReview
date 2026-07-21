import type { CardData, Integration } from "../../lib/types";
import { configList, configNumber, configText } from "../../lib/workflow";
import styles from "./CardInspector.module.scss";

export function CardInspector({
  selected,
  integrations,
  onChange,
}: {
  selected: CardData;
  integrations: Integration[];
  onChange: (patch: Partial<CardData>) => void;
}) {
  const updateConfig = (key: string, value: unknown) =>
    onChange({ config: { ...selected.config, [key]: value } });
  const gitea = integrations.filter(
    (item) => item.type === "gitea" && item.status === "active",
  );
  const models = integrations.filter(
    (item) => item.type !== "gitea" && item.status === "active",
  );
  const connectionFields = (
    <label>
      Integração
      <select
        value={configText(selected.config.integration)}
        onChange={(event) => updateConfig("integration", event.target.value)}
      >
        <option value="">Selecione uma conexão</option>
        {(selected.type === "model" ? models : gitea).map((item) => (
          <option key={item.key} value={item.key}>
            {item.name}
            {item.type !== "gitea" ? ` · ${item.config.model}` : ""}
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
          {selected.inputs.length} entrada(s) · {selected.outputs.length}{" "}
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
          </>
        )}
        {selected.type === "model" && (
          <>
            {connectionFields}
            {numberField("max_tokens", "Máximo de tokens", 2000)}
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
            <label>
              Ao falhar uma iteração
              <select
                value={configText(selected.config.on_error) || "fail"}
                onChange={(event) =>
                  updateConfig("on_error", event.target.value)
                }
              >
                <option value="fail">Interromper workflow</option>
                <option value="partial">Continuar com resultados parciais</option>
              </select>
            </label>
            <p>
              Use <code>item</code> para os cards do grupo e conecte apenas
              <code>results</code> ao consolidar. Assim consolidar, formatar e
              publicar executam uma vez no escopo raiz.
            </p>
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
          "condition",
          "validate",
          "response_filter",
        ].includes(selected.type) && (
          <p>Este card não possui parâmetros obrigatórios nesta fase.</p>
        )}
      </section>
      <p className={styles.note}>
        O backend valida cada conexão e configuração antes de executar uma
        versão.
      </p>
    </aside>
  );
}
