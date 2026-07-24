import { X } from "lucide-react";
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import { placeCardEditor, type PopoverPlacement } from "../../lib/popover";
import {
  parseResponseSchemaText,
  responseSchemaError,
} from "../../lib/response-contracts";
import type {
  CardData,
  Integration,
  ModelProfile,
  ResponseContract,
  ReviewContractVersion,
  WebhookRegistration,
  WorkflowSummary,
} from "../../lib/types";
import { configList, configNumber, configText } from "../../lib/workflow";
import { ToggleSwitch } from "../common/ToggleSwitch";
import styles from "./CardInspector.module.scss";

export function CardInspector({
  selected,
  integrations,
  modelProfiles,
  responseContracts,
  reviewContracts,
  workflows,
  hasErrorRoute,
  onChange,
  readOnly = false,
  webhookRegistrations = [],
  canAdministerWebhooks = false,
  workflowKey,
  onRegisterWebhook,
  onClose,
  nodeID,
}: {
  selected: CardData;
  integrations: Integration[];
  modelProfiles: ModelProfile[];
  responseContracts: ResponseContract[];
  reviewContracts: ReviewContractVersion[];
  workflows: WorkflowSummary[];
  hasErrorRoute: boolean;
  onChange: (patch: Partial<CardData>) => void;
  readOnly?: boolean;
  webhookRegistrations?: WebhookRegistration[];
  canAdministerWebhooks?: boolean;
  workflowKey: string;
  onRegisterWebhook?: (input: {
    key: string;
    name: string;
    workflow_key: string;
    trigger_node_key: string;
    secret: string;
    active: boolean;
  }) => Promise<void>;
  onClose: () => void;
  nodeID: string;
}) {
  const [webhookKey, setWebhookKey] = useState("");
  const [webhookName, setWebhookName] = useState("");
  const [webhookSecret, setWebhookSecret] = useState("");
  const [webhookBusy, setWebhookBusy] = useState(false);
  const [schemaText, setSchemaText] = useState("");
  const panelRef = useRef<HTMLElement>(null);
  const openerRef = useRef<HTMLElement | null>(null);
  const [placement, setPlacement] = useState<PopoverPlacement>();
  useEffect(() => {
    openerRef.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    panelRef.current
      ?.querySelector<HTMLElement>("[data-editor-autofocus]")
      ?.focus();
    return () => openerRef.current?.focus();
  }, []);
  useLayoutEffect(() => {
    const updatePlacement = () => {
      const anchor = Array.from(
        document.querySelectorAll<HTMLElement>(".react-flow__node"),
      ).find((element) => element.dataset.id === nodeID);
      const panel = panelRef.current;
      if (!anchor || !panel) return;
      const rect = anchor.getBoundingClientRect();
      const panelRect = panel.getBoundingClientRect();
      if (!panelRect.width || !panelRect.height) return;
      const canvasRect = document
        .querySelector<HTMLElement>(".react-flow")
        ?.getBoundingClientRect();
      const viewport = canvasRect ?? {
        left: 0,
        top: 0,
        width: window.innerWidth,
        height: window.innerHeight,
      };
      const next = placeCardEditor(
        {
          left: rect.left - viewport.left,
          top: rect.top - viewport.top,
          right: rect.right - viewport.left,
          bottom: rect.bottom - viewport.top,
          width: rect.width,
          height: rect.height,
        },
        { width: viewport.width, height: viewport.height },
        panelRect,
      );
      next.x += viewport.left;
      next.y += viewport.top;
      setPlacement((current) =>
        current?.side === next.side &&
        current.x === next.x &&
        current.y === next.y
          ? current
          : next,
      );
    };
    updatePlacement();
    const viewport = document.querySelector<HTMLElement>(
      ".react-flow__viewport",
    );
    const observer = new MutationObserver(updatePlacement);
    if (viewport)
      observer.observe(viewport, {
        attributes: true,
        attributeFilter: ["style"],
      });
    window.addEventListener("resize", updatePlacement);
    window.addEventListener("scroll", updatePlacement, true);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", updatePlacement);
      window.removeEventListener("scroll", updatePlacement, true);
    };
  }, [nodeID, selected]);
  useEffect(() => {
    const closeOnOutside = (event: PointerEvent) => {
      if (!panelRef.current?.contains(event.target as globalThis.Node))
        onClose();
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        onClose();
      }
    };
    window.addEventListener("pointerdown", closeOnOutside);
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      window.removeEventListener("pointerdown", closeOnOutside);
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, [onClose]);
  useEffect(() => {
    const schema = selected.config.response_schema;
    setSchemaText(
      typeof schema === "string"
        ? schema
        : schema && typeof schema === "object"
          ? JSON.stringify(schema, null, 2)
          : "",
    );
  }, [selected.key, selected.config.response_schema]);
  const updateConfig = (key: string, value: unknown) =>
    !readOnly && onChange({ config: { ...selected.config, [key]: value } });
  const replaceConfig = (
    patch: Record<string, unknown>,
    remove: string[] = [],
  ) => {
    if (readOnly) return;
    const next = { ...selected.config, ...patch };
    for (const key of remove) delete next[key];
    onChange({ config: next });
  };
  const schemaValidation = schemaText
    ? parseResponseSchemaText(schemaText)
    : undefined;
  const configuredReviewContract = reviewContracts.find(
    (item) =>
      item.key === selected.config.review_contract_key &&
      item.version === selected.config.review_contract_version,
  );
  const gitea = integrations.filter(
    (item) => item.type === "gitea" && item.status === "active",
  );
  const models = modelProfiles.filter((item) => item.status === "active");
  const connectionFields = (
    <label>
      {selected.type === "model" || selected.type === "candidate_validator"
        ? "Perfil de modelo"
        : "Integração"}
      <select
        value={configText(
          selected.config[
            selected.type === "model" || selected.type === "candidate_validator"
              ? "model_profile"
              : "integration"
          ],
        )}
        onChange={(event) =>
          updateConfig(
            selected.type === "model" || selected.type === "candidate_validator"
              ? "model_profile"
              : "integration",
            event.target.value,
          )
        }
      >
        <option value="">
          {selected.type === "model" || selected.type === "candidate_validator"
            ? "Selecione um perfil"
            : "Selecione uma conexão"}
        </option>
        {(selected.type === "model" || selected.type === "candidate_validator"
          ? models
          : gitea
        ).map((item) => (
          <option key={item.key} value={item.key}>
            {item.name}
            {(selected.type === "model" ||
              selected.type === "candidate_validator") &&
            "model" in item
              ? ` · ${item.model}`
              : ""}
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
  const decimalField = (
    key: string,
    label: string,
    fallback: number,
    min: number,
    max: number,
    step: number,
  ) => (
    <label>
      {label}
      <input
        type="number"
        min={min}
        max={max}
        step={step}
        value={configNumber(selected.config[key], fallback)}
        onChange={(event) =>
          updateConfig(
            key,
            Number.isNaN(event.target.valueAsNumber)
              ? fallback
              : event.target.valueAsNumber,
          )
        }
      />
    </label>
  );
  const optionalDecimalField = (
    key: string,
    label: string,
    min: number,
    max: number,
    step: number,
    placeholder: string,
  ) => (
    <label>
      {label}
      <input
        type="number"
        min={min}
        max={max}
        step={step}
        value={
          typeof selected.config[key] === "number"
            ? (selected.config[key] as number)
            : ""
        }
        placeholder={placeholder}
        onChange={(event) =>
          updateConfig(
            key,
            event.target.value === "" ? undefined : event.target.valueAsNumber,
          )
        }
      />
    </label>
  );
  return (
    <section
      ref={panelRef}
      className={styles.inspector}
      role="dialog"
      aria-modal={false}
      aria-labelledby={`card-editor-${nodeID}`}
      style={
        placement
          ? ({
              "--editor-x": `${placement.x}px`,
              "--editor-y": `${placement.y}px`,
            } as CSSProperties)
          : undefined
      }
      onKeyDown={(event) => {
        if (event.key !== "Tab") return;
        const focusable = Array.from(
          panelRef.current?.querySelectorAll<HTMLElement>(
            "button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [href]",
          ) ?? [],
        ).filter((element) => !element.hasAttribute("disabled"));
        if (!focusable.length) return;
        const first = focusable[0];
        const last = focusable.at(-1)!;
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }}
    >
      <header>
        <div>
          <span>CONFIGURAÇÃO DO CARD</span>
          <b id={`card-editor-${nodeID}`}>{selected.name}</b>
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label={`Fechar editor de ${selected.name}`}
          title="Fechar editor"
        >
          <X size={16} aria-hidden="true" />
        </button>
      </header>
      <fieldset className={styles.fields} disabled={readOnly}>
        <label>
          Nome do card
          <input
            data-editor-autofocus
            value={selected.name}
            onChange={(event) => onChange({ name: event.target.value })}
          />
        </label>
        <dl>
          <dt>Tipo</dt>
          <dd>{selected.type}</dd>
          <dt>Portas</dt>
          <dd>
            {selected.inputs.length} entrada(s) ·{" "}
            {selected.outputs.length +
              (selected.config.on_error === "route" && selected.errorOutput
                ? 1
                : 0)}{" "}
            saída(s)
          </dd>
        </dl>
        <section className={styles.config}>
          <strong>Parâmetros</strong>
          {selected.type === "trigger" && (
            <>
              <label>
                Modo de acionamento
                <select
                  value={configText(selected.config.mode) || "manual"}
                  onChange={(event) => updateConfig("mode", event.target.value)}
                >
                  <option value="manual">Manual (Studio)</option>
                  <option value="api">API autenticada</option>
                  <option value="webhook">Webhook Gitea</option>
                </select>
              </label>
              {(configText(selected.config.mode) || "manual") === "api" && (
                <small>
                  Após publicar, envie POST para{" "}
                  <code>
                    /api/workflows/{workflowKey}/triggers/{selected.key}
                    /executions
                  </code>{" "}
                  com um objeto JSON.
                </small>
              )}
              {configText(selected.config.mode) === "webhook" && (
                <section>
                  <small>
                    Publique este trigger antes de registrar. O segredo é
                    criptografado separadamente e nunca volta para o navegador.
                  </small>
                  {webhookRegistrations
                    .filter(
                      (item) =>
                        item.workflow_key === workflowKey &&
                        item.trigger_node_key === selected.key,
                    )
                    .map((item) => (
                      <p key={item.key}>
                        <b>{item.name}</b> ·{" "}
                        <code>/webhooks/gitea/{item.key}</code> ·{" "}
                        {item.active ? "ativo" : "inativo"}
                      </p>
                    ))}
                  {canAdministerWebhooks && (
                    <>
                      <label>
                        Chave pública
                        <input
                          value={webhookKey}
                          onChange={(event) =>
                            setWebhookKey(event.target.value)
                          }
                          placeholder="gitea-review"
                        />
                      </label>
                      <label>
                        Nome
                        <input
                          value={webhookName}
                          onChange={(event) =>
                            setWebhookName(event.target.value)
                          }
                          placeholder="Gitea principal"
                        />
                      </label>
                      <label>
                        Segredo de assinatura
                        <input
                          type="password"
                          autoComplete="new-password"
                          value={webhookSecret}
                          onChange={(event) =>
                            setWebhookSecret(event.target.value)
                          }
                        />
                      </label>
                      <button
                        type="button"
                        disabled={
                          webhookBusy ||
                          !webhookKey ||
                          !webhookName ||
                          !webhookSecret
                        }
                        onClick={async () => {
                          if (!onRegisterWebhook) return;
                          setWebhookBusy(true);
                          try {
                            await onRegisterWebhook({
                              key: webhookKey,
                              name: webhookName,
                              workflow_key: workflowKey,
                              trigger_node_key: selected.key,
                              secret: webhookSecret,
                              active: true,
                            });
                            setWebhookSecret("");
                          } finally {
                            setWebhookBusy(false);
                          }
                        }}
                      >
                        {webhookBusy ? "Registrando..." : "Registrar webhook"}
                      </button>
                    </>
                  )}
                </section>
              )}
              <label>
                Entradas públicas
                <input
                  value={configList(selected.config.published_inputs)}
                  onChange={(event) =>
                    updateConfig(
                      "published_inputs",
                      event.target.value
                        .split(",")
                        .map((item) => item.trim())
                        .filter(Boolean),
                    )
                  }
                  placeholder="pull_request, policy"
                />
                <small>
                  Campos separados por vírgula publicados na interface desta
                  pipeline.
                </small>
              </label>
              <label>
                Contratos avançados de entrada (JSON)
                <textarea
                  key={`${nodeID}-published-input-fields`}
                  defaultValue={
                    Array.isArray(selected.config.published_input_fields)
                      ? JSON.stringify(
                          selected.config.published_input_fields,
                          null,
                          2,
                        )
                      : ""
                  }
                  onInput={(event) => event.currentTarget.setCustomValidity("")}
                  onBlur={(event) => {
                    if (!event.currentTarget.value.trim())
                      return updateConfig("published_input_fields", undefined);
                    try {
                      updateConfig(
                        "published_input_fields",
                        JSON.parse(event.currentTarget.value),
                      );
                    } catch {
                      event.currentTarget.setCustomValidity("JSON inválido");
                      event.currentTarget.reportValidity();
                    }
                  }}
                  placeholder='[{"key":"payload","label":"Payload","contract":"any","required":true}]'
                />
                <small>
                  Quando informado, substitui a lista simples e publica
                  contratos e obrigatoriedade explícitos.
                </small>
              </label>
            </>
          )}
          {selected.type === "template" && (
            <>
              <label>
                Contrato da tarefa
                <select
                  value={
                    configuredReviewContract
                      ? `${configuredReviewContract.key}@${configuredReviewContract.version}`
                      : ""
                  }
                  onChange={(event) => {
                    const contract = reviewContracts.find(
                      (item) =>
                        `${item.key}@${item.version}` === event.target.value,
                    );
                    if (!contract) {
                      replaceConfig({}, [
                        "review_contract_key",
                        "review_contract_version",
                      ]);
                      return;
                    }
                    replaceConfig({
                      review_contract_key: contract.key,
                      review_contract_version: contract.version,
                    });
                  }}
                >
                  <option value="">Prompt livre (legado)</option>
                  {reviewContracts.map((contract) => (
                    <option
                      key={`${contract.key}@${contract.version}`}
                      value={`${contract.key}@${contract.version}`}
                    >
                      {contract.name} · v{contract.version}
                    </option>
                  ))}
                </select>
                <small>
                  {configuredReviewContract
                    ? `${configuredReviewContract.checklist.items.length} checks; o schema e a checklist acompanharão a tarefa.`
                    : "Selecione um contrato persistido para uma tarefa de review tipada."}
                </small>
              </label>
              <label>
                Prompt base
                <textarea
                  value={configText(selected.config.template)}
                  onChange={(event) =>
                    updateConfig("template", event.target.value)
                  }
                  placeholder="Descreva a revisão e use variáveis declaradas."
                />
              </label>
            </>
          )}
          {selected.type === "fetch" && <>{connectionFields}</>}
          {(selected.type === "model" ||
            selected.type === "candidate_validator") && (
            <>
              {connectionFields}
              {numberField("max_tokens", "Máximo de tokens", 2000)}
              {optionalDecimalField(
                "temperature",
                "Temperatura",
                0,
                2,
                0.1,
                "Padrão do provider",
              )}
              {optionalDecimalField(
                "top_p",
                "Top-p",
                0.01,
                1,
                0.01,
                "Padrão do provider",
              )}
              {numberField(
                "timeout_seconds",
                "Timeout da chamada (segundos)",
                120,
              )}
              <label>
                Keep-alive do Ollama
                <input
                  value={configText(selected.config.keep_alive)}
                  onChange={(event) =>
                    updateConfig("keep_alive", event.target.value)
                  }
                  placeholder="5m"
                />
                <small>
                  Use 0 para descarregar imediatamente ou uma duração de até
                  24h. O parâmetro é ignorado por providers OpenAI-compatible.
                </small>
              </label>
              <label>
                Perfil de fallback
                <select
                  value={configText(selected.config.fallback_model_profile)}
                  onChange={(event) =>
                    updateConfig("fallback_model_profile", event.target.value)
                  }
                >
                  <option value="">Sem fallback</option>
                  {models
                    .filter(
                      (item) =>
                        item.key !== configText(selected.config.model_profile),
                    )
                    .map((item) => (
                      <option key={item.key} value={item.key}>
                        {item.name} · {item.model}
                      </option>
                    ))}
                </select>
              </label>
              {decimalField(
                "input_cost_per_million_usd",
                "Custo de entrada / 1M tokens (USD)",
                0,
                0,
                1000000,
                0.000001,
              )}
              {decimalField(
                "output_cost_per_million_usd",
                "Custo de saída / 1M tokens (USD)",
                0,
                0,
                1000000,
                0.000001,
              )}
              {decimalField(
                "max_cost_usd",
                "Orçamento máximo por chamada (USD)",
                0,
                0,
                1000000,
                0.000001,
              )}
              {selected.type === "model" &&
                boundedNumberField(
                  "retry_limit",
                  "Tentativas de correção",
                  0,
                  3,
                )}
              {selected.type === "model" &&
                boundedNumberField(
                  "retry_delay_ms",
                  "Espera entre tentativas (ms)",
                  0,
                  60000,
                )}
              <small>
                {selected.type === "candidate_validator"
                  ? "Cada candidato recebe uma chamada isolada. Somente CONFIRMED segue para publicação; falhas de contrato interrompem o card."
                  : "Quando a saída for ligada diretamente a Validar e falhar no formato, o modelo repete o prompt com uma instrução de correção. Zero desativa; no máximo 3 tentativas e 60.000 ms de espera."}
              </small>
            </>
          )}
          {selected.type === "publish" && (
            <>
              {connectionFields}
              <label>
                Evento para severidade média
                <select
                  value={
                    configText(selected.config.medium_severity_event) ||
                    "COMMENT"
                  }
                  onChange={(event) =>
                    updateConfig("medium_severity_event", event.target.value)
                  }
                >
                  <option value="COMMENT">Comentar</option>
                  <option value="REQUEST_CHANGES">Solicitar mudanças</option>
                </select>
              </label>
              <ToggleSwitch
                checked={selected.config.allow_autonomous_rejection === true}
                onChange={(checked) =>
                  updateConfig("allow_autonomous_rejection", checked)
                }
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
              <ToggleSwitch
                checked={Boolean(selected.config.ignore_generated)}
                onChange={(checked) =>
                  updateConfig("ignore_generated", checked)
                }
                label="Ignorar arquivos gerados"
                description="Remove artefatos gerados antes da análise."
              />
            </>
          )}
          {selected.type === "group" && (
            <>
              {numberField("max_files", "Máximo de arquivos", 8)}
              {numberField("max_characters", "Máximo de caracteres", 12000)}
              <ToggleSwitch
                checked={Boolean(selected.config.group_by_extension)}
                onChange={(checked) =>
                  updateConfig("group_by_extension", checked)
                }
                label="Separar por extensão"
                description="Mantém linguagens diferentes em grupos distintos."
              />
            </>
          )}
          {selected.type === "loop" && (
            <>
              {numberField("max_iterations", "Máximo de iterações", 20)}
              <label>
                Concorrência
                <input
                  type="number"
                  min="1"
                  max="4"
                  value={configNumber(selected.config.concurrency, 1)}
                  onChange={(event) =>
                    updateConfig(
                      "concurrency",
                      Math.max(1, Math.min(4, event.target.valueAsNumber || 1)),
                    )
                  }
                />
                <small>
                  De 1 a 4 escopos filhos; os resultados continuam ordenados
                  pela entrada.
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
                        ...(mode === "write" &&
                        selected.config.ttl_seconds === undefined
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
                      updateConfig(
                        "ttl_seconds",
                        event.target.valueAsNumber || 1,
                      )
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
                  onChange={(event) =>
                    updateConfig("on_error", event.target.value)
                  }
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
                    onChange={(event) =>
                      updateConfig("fallback_result", event.target.value)
                    }
                    placeholder="Resultado seguro"
                  />
                </label>
              )}
            </>
          )}
          {selected.type === "condition" && (
            <>
              <label>
                Ramos declarativos (JSON)
                <textarea
                  key={`${nodeID}-branches`}
                  defaultValue={
                    Array.isArray(selected.config.branches)
                      ? JSON.stringify(selected.config.branches, null, 2)
                      : ""
                  }
                  onInput={(event) => event.currentTarget.setCustomValidity("")}
                  onBlur={(event) => {
                    if (!event.currentTarget.value.trim())
                      return updateConfig("branches", undefined);
                    try {
                      updateConfig(
                        "branches",
                        JSON.parse(event.currentTarget.value),
                      );
                    } catch {
                      event.currentTarget.setCustomValidity("JSON inválido");
                      event.currentTarget.reportValidity();
                    }
                  }}
                  placeholder='[{"port":"match_1","path":"language","operator":"equals","value":"go"}]'
                />
              </label>
              {!Array.isArray(selected.config.branches) && (
                <label>
                  Valor esperado (modo legado)
                  <input
                    value={configText(selected.config.equals)}
                    onChange={(event) =>
                      updateConfig("equals", event.target.value)
                    }
                    placeholder="php"
                  />
                </label>
              )}
            </>
          )}
          {selected.type === "transform" && (
            <label>
              Operações declarativas (JSON)
              <textarea
                key={`${nodeID}-operations`}
                defaultValue={
                  Array.isArray(selected.config.operations)
                    ? JSON.stringify(selected.config.operations, null, 2)
                    : ""
                }
                onInput={(event) => event.currentTarget.setCustomValidity("")}
                onBlur={(event) => {
                  if (!event.currentTarget.value.trim())
                    return updateConfig("operations", undefined);
                  try {
                    updateConfig(
                      "operations",
                      JSON.parse(event.currentTarget.value),
                    );
                  } catch {
                    event.currentTarget.setCustomValidity("JSON inválido");
                    event.currentTarget.reportValidity();
                  }
                }}
                placeholder='[{"op":"select","path":"pull_request"}]'
              />
              <small>Operações: select, set, remove, rename e coalesce.</small>
            </label>
          )}
          {selected.type === "variable" && (
            <>
              <label>
                Operação
                <select
                  value={configText(selected.config.action) || "set"}
                  onChange={(event) =>
                    updateConfig("action", event.target.value)
                  }
                >
                  <option value="set">Definir</option>
                  <option value="get">Ler</option>
                </select>
              </label>
              <label>
                Namespace
                <select
                  value={configText(selected.config.namespace) || "execution"}
                  onChange={(event) =>
                    updateConfig("namespace", event.target.value)
                  }
                >
                  <option value="execution">Execução</option>
                  <option value="loop">Loop</option>
                  <option value="card">Card</option>
                </select>
              </label>
              <label>
                Nome da variável
                <input
                  value={configText(selected.config.name)}
                  onChange={(event) => updateConfig("name", event.target.value)}
                  placeholder="review_policy"
                />
              </label>
              {(configText(selected.config.action) || "set") === "set" && (
                <label>
                  Valor padrão
                  <input
                    value={configText(selected.config.value)}
                    onChange={(event) =>
                      updateConfig("value", event.target.value)
                    }
                    placeholder="Usado quando não há entrada"
                  />
                </label>
              )}
            </>
          )}
          {selected.type === "merge" && (
            <>
              <label>
                Política de join
                <select
                  value={configText(selected.config.mode) || "all"}
                  onChange={(event) => updateConfig("mode", event.target.value)}
                >
                  <option value="all">Todas as entradas</option>
                  <option value="any">Primeira entrada</option>
                  <option value="quorum">Quórum</option>
                </select>
              </label>
              {configText(selected.config.mode) === "quorum" &&
                numberField("quorum", "Quantidade para quórum", 2)}
              {optionalDecimalField(
                "timeout_ms",
                "Timeout para resultado parcial (ms)",
                1,
                60000,
                1,
                "Sem timeout",
              )}
            </>
          )}
          {selected.type === "workflow" && (
            <>
              <label>
                Versão publicada da subpipeline
                <select
                  value={String(
                    configNumber(selected.config.workflow_version_id, 0),
                  )}
                  onChange={(event) => {
                    const versionID = Number(event.target.value);
                    const owner = workflows.find((workflow) =>
                      workflow.versions.some(
                        (version) => version.version_id === versionID,
                      ),
                    );
                    onChange({
                      config: {
                        ...selected.config,
                        workflow_key: owner?.key || "",
                        workflow_version_id: versionID,
                      },
                    });
                  }}
                >
                  <option value="0">Selecione uma versão</option>
                  {workflows
                    .filter((workflow) => workflow.key !== workflowKey)
                    .flatMap((workflow) =>
                      workflow.versions
                        .filter((version) => version.status !== "draft")
                        .map((version) => (
                          <option
                            key={version.version_id}
                            value={version.version_id}
                          >
                            {workflow.name} · v{version.version} ·{" "}
                            {version.status === "published"
                              ? "publicada"
                              : "arquivada"}
                          </option>
                        )),
                    )}
                </select>
                <small>
                  A versão fica fixada e continua imutável mesmo após ser
                  arquivada.
                </small>
              </label>
            </>
          )}
          {selected.type === "validate" && (
            <>
              <label>
                Contrato de resposta
                <select
                  value={
                    configText(selected.config.response_contract_key) ||
                    (selected.config.response_schema === undefined
                      ? "legacy"
                      : "custom")
                  }
                  onChange={(event) => {
                    if (event.target.value === "legacy") {
                      setSchemaText("");
                      replaceConfig({}, [
                        "response_schema",
                        "response_contract_key",
                      ]);
                      return;
                    }
                    if (event.target.value === "custom") {
                      if (selected.config.response_schema === undefined) {
                        const schema = responseContracts[0]?.schema ?? {
                          type: "object",
                        };
                        setSchemaText(JSON.stringify(schema, null, 2));
                        replaceConfig({ response_schema: schema }, [
                          "response_contract_key",
                        ]);
                      } else {
                        replaceConfig({}, ["response_contract_key"]);
                      }
                      return;
                    }
                    const contract = responseContracts.find(
                      (item) => item.key === event.target.value,
                    );
                    if (!contract) return;
                    setSchemaText(JSON.stringify(contract.schema, null, 2));
                    replaceConfig({
                      response_contract_key: contract.key,
                      response_schema: contract.schema,
                    });
                  }}
                >
                  <option value="legacy">Legado · Finding[]</option>
                  {responseContracts.map((contract) => (
                    <option key={contract.key} value={contract.key}>
                      {contract.name} · v{contract.version}
                    </option>
                  ))}
                  <option value="custom">Personalizado</option>
                </select>
                <small>
                  {responseContracts.find(
                    (item) =>
                      item.key ===
                      configText(selected.config.response_contract_key),
                  )?.description ??
                    "O backend continua sendo a autoridade final do contrato."}
                </small>
              </label>
              {selected.config.response_schema !== undefined && (
                <>
                  <label>
                    JSON Schema
                    <textarea
                      className={styles.schemaEditor}
                      spellCheck={false}
                      value={schemaText}
                      onChange={(event) => {
                        const text = event.target.value;
                        setSchemaText(text);
                        const parsed = parseResponseSchemaText(text);
                        replaceConfig(
                          {
                            response_schema:
                              "schema" in parsed ? parsed.schema : text,
                          },
                          ["response_contract_key"],
                        );
                      }}
                      readOnly={Boolean(selected.config.response_contract_key)}
                    />
                  </label>
                  <div className={styles.contractActions}>
                    <button
                      type="button"
                      onClick={() => {
                        const parsed = parseResponseSchemaText(schemaText);
                        if ("schema" in parsed)
                          setSchemaText(JSON.stringify(parsed.schema, null, 2));
                      }}
                    >
                      Formatar
                    </button>
                    {Boolean(selected.config.response_contract_key) && (
                      <button
                        type="button"
                        onClick={() =>
                          replaceConfig({}, ["response_contract_key"])
                        }
                      >
                        Duplicar para editar
                      </button>
                    )}
                    {Boolean(selected.config.response_contract_key) && (
                      <button
                        type="button"
                        onClick={() => {
                          const contract = responseContracts.find(
                            (item) =>
                              item.key ===
                              selected.config.response_contract_key,
                          );
                          if (!contract) return;
                          setSchemaText(
                            JSON.stringify(contract.schema, null, 2),
                          );
                          replaceConfig({ response_schema: contract.schema });
                        }}
                      >
                        Restaurar
                      </button>
                    )}
                  </div>
                  <small
                    className={
                      schemaValidation && "schema" in schemaValidation
                        ? styles.valid
                        : styles.invalid
                    }
                  >
                    {schemaValidation && "schema" in schemaValidation
                      ? "JSON Schema válido."
                      : (schemaValidation?.error ??
                        responseSchemaError(selected.config.response_schema))}
                  </small>
                </>
              )}
              <ToggleSwitch
                checked={Boolean(selected.config.validate_paths)}
                onChange={(checked) => updateConfig("validate_paths", checked)}
                label="Validar caminhos do PR"
                description="Recusa achados para arquivos ausentes nos dados buscados."
              />
            </>
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
            "candidate_validator",
            "publish",
            "filter",
            "group",
            "loop",
            "cache",
            "condition",
            "transform",
            "variable",
            "merge",
            "workflow",
            "validate",
            "response_filter",
            "error_control",
          ].includes(selected.type) && (
            <p>Este card não possui parâmetros obrigatórios nesta fase.</p>
          )}
        </section>
        {selected.outputs.length > 0 && (
          <section className={styles.config}>
            <strong>Interface publicada</strong>
            <label>
              Nome público da saída
              <input
                value={configText(selected.config.published_output_key)}
                onChange={(event) =>
                  updateConfig("published_output_key", event.target.value)
                }
                placeholder="result"
              />
            </label>
            <label>
              Porta publicada
              <select
                value={configText(selected.config.published_output_port)}
                onChange={(event) =>
                  updateConfig("published_output_port", event.target.value)
                }
              >
                <option value="">Não publicar</option>
                {selected.outputs.map((port) => (
                  <option key={port.key} value={port.key}>
                    {port.label} · {port.contract}
                  </option>
                ))}
              </select>
            </label>
            {configText(selected.config.published_output_port) && (
              <ToggleSwitch
                checked={Boolean(selected.config.published_output_required)}
                onChange={(checked) =>
                  updateConfig("published_output_required", checked)
                }
                label="Saída obrigatória"
                description="A chamada da subpipeline falha se esta saída não for produzida."
              />
            )}
          </section>
        )}
        {selected.type !== "error_control" && selected.errorOutput && (
          <section className={styles.errorPolicy}>
            <strong>Política de erro</strong>
            <label>
              Ao falhar
              <select
                value={configText(selected.config.on_error) || "fail"}
                onChange={(event) =>
                  updateConfig("on_error", event.target.value)
                }
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
    </section>
  );
}
