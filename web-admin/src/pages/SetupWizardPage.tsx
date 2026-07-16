import { useMemo, useState } from "react";
import type { AdminRequest } from "../api/adminClient";
import { WizardProgress } from "../components/setup/WizardProgress";

type CatalogModel = {
  id: string;
  name: string;
  context_length: number;
  max_completion_tokens: number;
  supported_parameters: string[];
  prompt_price?: string;
  completion_price?: string;
  size?: number;
};
type Draft = {
  provider: "ollama" | "openrouter" | "";
  connection: {
    name: string;
    base_url: string;
    api_key_env_name: string;
    http_referer: string;
    app_title: string;
  };
  model: CatalogModel | null;
  parameters: {
    temperature: number;
    top_p: number;
    max_output_tokens: number;
    repeat_penalty: number;
    num_ctx: number;
    num_threads: number;
    num_predict: number;
    timeout_seconds: number;
    keep_alive: string;
    unload_model_after_review: boolean;
  };
  profile: { name: string; description: string };
  policy: {
    max_block_chars: number;
    max_files_per_block: number;
    review_concurrency: number;
    review_final_retries: number;
    review_wip_pull_requests: boolean;
    review_own_pull_requests: boolean;
    publish_manual_reviews: boolean;
    allow_autonomous_rejection: boolean;
    log_sensitive_data: boolean;
    unload_model_after_review: boolean;
  };
};

const initialDraft: Draft = {
  provider: "",
  connection: {
    name: "",
    base_url: "",
    api_key_env_name: "OPENROUTER_API_KEY",
    http_referer: "",
    app_title: "ForgeReview",
  },
  model: null,
  parameters: {
    temperature: 0.2,
    top_p: 0.9,
    max_output_tokens: 4096,
    repeat_penalty: 1.1,
    num_ctx: 0,
    num_threads: 0,
    num_predict: 0,
    timeout_seconds: 900,
    keep_alive: "5m",
    unload_model_after_review: false,
  },
  profile: {
    name: "Review padrão",
    description: "Configuração principal de revisão de código",
  },
  policy: {
    max_block_chars: 12000,
    max_files_per_block: 4,
    review_concurrency: 1,
    review_final_retries: 5,
    review_wip_pull_requests: false,
    review_own_pull_requests: false,
    publish_manual_reviews: false,
    allow_autonomous_rejection: false,
    log_sensitive_data: false,
    unload_model_after_review: false,
  },
};
const stepCopy = [
  [
    "Escolha a integração",
    "O fluxo será adaptado às exigências reais do provider.",
  ],
  [
    "Valide a conexão",
    "A API testa o endpoint e as credenciais antes de continuar.",
  ],
  [
    "Escolha um modelo real",
    "O catálogo é consultado diretamente no provider.",
  ],
  [
    "Ajuste a execução",
    "Somente parâmetros aplicáveis ao provider selecionado são exibidos.",
  ],
  ["Defina o comportamento", "Crie o profile padrão e os limites de revisão."],
  [
    "Revise e conclua",
    "Tudo será salvo de forma atômica como configuração padrão.",
  ],
] as const;

const moneyPerMillion = (value?: string) => {
  const number = Number(value);
  return Number.isFinite(number) && number > 0
    ? `US$ ${(number * 1_000_000).toFixed(2)}/M`
    : "—";
};
const bytes = (value?: number) =>
  value ? `${(value / 1024 / 1024 / 1024).toFixed(1)} GB` : "—";

type Props = { request: AdminRequest; onComplete: () => void };
export function SetupWizardPage({ request, onComplete }: Props) {
  const [step, setStep] = useState(0),
    [draft, setDraft] = useState<Draft>(initialDraft),
    [catalog, setCatalog] = useState<CatalogModel[]>([]),
    [account, setAccount] = useState<Record<string, any> | null>(null),
    [search, setSearch] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const copy = stepCopy[step];
  const updateConnection = (key: keyof Draft["connection"], value: string) =>
    setDraft((valueDraft) => ({
      ...valueDraft,
      connection: { ...valueDraft.connection, [key]: value },
    }));
  const updateParameters = (key: keyof Draft["parameters"], value: any) =>
    setDraft((valueDraft) => ({
      ...valueDraft,
      parameters: { ...valueDraft.parameters, [key]: value },
    }));
  const updateProfile = (key: keyof Draft["profile"], value: string) =>
    setDraft((valueDraft) => ({
      ...valueDraft,
      profile: { ...valueDraft.profile, [key]: value },
    }));
  const updatePolicy = (key: keyof Draft["policy"], value: any) =>
    setDraft((valueDraft) => ({
      ...valueDraft,
      policy: { ...valueDraft.policy, [key]: value },
    }));
  const chooseProvider = (provider: "ollama" | "openrouter") => {
    setDraft((value) => ({
      ...value,
      provider,
      model: null,
      connection: {
        name:
          provider === "openrouter" ? "OpenRouter principal" : "Ollama local",
        base_url:
          provider === "openrouter"
            ? "https://openrouter.ai/api/v1"
            : "http://host.docker.internal:11434",
        api_key_env_name: provider === "openrouter" ? "OPENROUTER_API_KEY" : "",
        http_referer: "",
        app_title: "ForgeReview",
      },
    }));
    setCatalog([]);
    setStep(1);
  };
  const validateConnection = async () => {
    setBusy(true);
    setError("");
    try {
      const result = await request("setup/catalog", {
        method: "POST",
        body: JSON.stringify({
          provider: draft.provider,
          connection: draft.connection,
        }),
      });
      setCatalog(result.models || []);
      setAccount(result.account || null);
      if (!result.models?.length)
        throw new Error(
          "A conexão funcionou, mas nenhum modelo foi encontrado.",
        );
      setStep(2);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };
  const filtered = useMemo(() => {
    const term = search.toLowerCase().trim();
    return term
      ? catalog.filter((model) =>
          (model.name + " " + model.id).toLowerCase().includes(term),
        )
      : catalog;
  }, [catalog, search]);
  const submit = async () => {
    setBusy(true);
    setError("");
    try {
      await request("setup/complete", {
        method: "POST",
        body: JSON.stringify(draft),
      });
      onComplete();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };
  const canNext =
    step === 2
      ? Boolean(draft.model)
      : step === 4
        ? Boolean(draft.profile.name)
        : true;
  return (
    <section className="wizard-page">
      <WizardProgress current={step} />
      <div className="wizard-card">
        <header>
          <span className="section-kicker">
            Configuração guiada · etapa {step + 1} de 6
          </span>
          <h2>{copy[0]}</h2>
          <p>{copy[1]}</p>
        </header>
        {error && <div className="wizard-error">{error}</div>}
        <div className="wizard-body">
          {step === 0 && (
            <div className="provider-choice">
              <button onClick={() => chooseProvider("ollama")}>
                <span className="provider-logo">OL</span>
                <strong>Ollama</strong>
                <p>Modelos executados na sua infraestrutura local.</p>
                <small>Sem chave · catálogo em /api/tags</small>
              </button>
              <button onClick={() => chooseProvider("openrouter")}>
                <span className="provider-logo openrouter">OR</span>
                <strong>OpenRouter</strong>
                <p>
                  Catálogo unificado de modelos via API compatível com OpenAI.
                </p>
                <small>Bearer token · catálogo oficial</small>
              </button>
            </div>
          )}
          {step === 1 && (
            <div className="wizard-form">
              <label>
                <span>Nome da conexão</span>
                <input
                  value={draft.connection.name}
                  onChange={(e) => updateConnection("name", e.target.value)}
                  required
                />
              </label>
              <label className="wide">
                <span>URL base</span>
                <input
                  value={draft.connection.base_url}
                  onChange={(e) => updateConnection("base_url", e.target.value)}
                  required
                />
                <small>
                  {draft.provider === "ollama"
                    ? "Dentro do Docker, use host.docker.internal para acessar o Ollama do host."
                    : "Endpoint oficial recomendado pelo OpenRouter."}
                </small>
              </label>
              {draft.provider === "openrouter" && (
                <>
                  <label>
                    <span>Variável da API key</span>
                    <input
                      value={draft.connection.api_key_env_name}
                      onChange={(e) =>
                        updateConnection("api_key_env_name", e.target.value)
                      }
                      required
                    />
                    <small>
                      A chave permanece no .env e nunca é gravada no banco.
                    </small>
                  </label>
                  <label>
                    <span>Nome da aplicação</span>
                    <input
                      value={draft.connection.app_title}
                      onChange={(e) =>
                        updateConnection("app_title", e.target.value)
                      }
                    />
                    <small>Enviado como X-OpenRouter-Title.</small>
                  </label>
                  <label className="wide">
                    <span>URL da aplicação (opcional)</span>
                    <input
                      value={draft.connection.http_referer}
                      onChange={(e) =>
                        updateConnection("http_referer", e.target.value)
                      }
                      placeholder="https://seu-dominio.example"
                    />
                    <small>
                      Enviado como HTTP-Referer para atribuição no OpenRouter.
                    </small>
                  </label>
                </>
              )}
            </div>
          )}
          {step === 2 && (
            <>
              <div className="catalog-summary">
                <div>
                  <strong>{catalog.length}</strong>
                  <span>modelos disponíveis</span>
                </div>
                {draft.provider === "openrouter" && account && (
                  <>
                    <div>
                      <strong>{String(account.label || "Chave válida")}</strong>
                      <span>credencial OpenRouter</span>
                    </div>
                    <div>
                      <strong>
                        {account.limit_remaining == null
                          ? "Sem limite"
                          : `US$ ${Number(account.limit_remaining).toFixed(2)}`}
                      </strong>
                      <span>limite restante</span>
                    </div>
                  </>
                )}
              </div>
              <div className="model-search">
                <input
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Buscar por nome ou slug..."
                />
              </div>
              <div className="model-catalog">
                {filtered.slice(0, 80).map((model) => (
                  <button
                    key={model.id}
                    className={draft.model?.id === model.id ? "selected" : ""}
                    onClick={() => setDraft((value) => ({ ...value, model }))}
                  >
                    <span>
                      <strong>{model.name || model.id}</strong>
                      <code>{model.id}</code>
                    </span>
                    <span className="model-facts">
                      <small>
                        {model.context_length
                          ? `${model.context_length.toLocaleString("pt-BR")} tokens`
                          : bytes(model.size)}
                      </small>
                      {draft.provider === "openrouter" && (
                        <small>
                          {moneyPerMillion(model.prompt_price)} entrada
                        </small>
                      )}
                    </span>
                  </button>
                ))}
              </div>
            </>
          )}
          {step === 3 && (
            <div className="wizard-form">
              <label>
                <span>Temperature</span>
                <input
                  type="number"
                  min="0"
                  max="2"
                  step="0.1"
                  value={draft.parameters.temperature}
                  onChange={(e) =>
                    updateParameters("temperature", Number(e.target.value))
                  }
                />
                <small>OpenRouter aceita valores de 0 a 2.</small>
              </label>
              <label>
                <span>Top P</span>
                <input
                  type="number"
                  min="0"
                  max="1"
                  step="0.05"
                  value={draft.parameters.top_p}
                  onChange={(e) =>
                    updateParameters("top_p", Number(e.target.value))
                  }
                />
              </label>
              <label>
                <span>Timeout</span>
                <input
                  type="number"
                  min="1"
                  value={draft.parameters.timeout_seconds}
                  onChange={(e) =>
                    updateParameters("timeout_seconds", Number(e.target.value))
                  }
                />
                <small>Segundos por chamada ao modelo.</small>
              </label>
              {draft.provider === "openrouter" && (
                <label>
                  <span>Máx. tokens de saída</span>
                  <input
                    type="number"
                    min="256"
                    max="32768"
                    step="256"
                    value={draft.parameters.max_output_tokens}
                    onChange={(e) =>
                      updateParameters(
                        "max_output_tokens",
                        Number(e.target.value),
                      )
                    }
                  />
                  <small>
                    Limite por resposta. Não é a janela máxima anunciada pelo
                    modelo.
                  </small>
                </label>
              )}
              {draft.provider === "ollama" && (
                <>
                  <label>
                    <span>Repeat penalty</span>
                    <input
                      type="number"
                      step="0.1"
                      value={draft.parameters.repeat_penalty}
                      onChange={(e) =>
                        updateParameters(
                          "repeat_penalty",
                          Number(e.target.value),
                        )
                      }
                    />
                  </label>
                  <label>
                    <span>Contexto (num_ctx)</span>
                    <input
                      type="number"
                      min="0"
                      value={draft.parameters.num_ctx}
                      onChange={(e) =>
                        updateParameters("num_ctx", Number(e.target.value))
                      }
                    />
                  </label>
                  <label>
                    <span>Tokens previstos</span>
                    <input
                      type="number"
                      min="0"
                      value={draft.parameters.num_predict}
                      onChange={(e) =>
                        updateParameters("num_predict", Number(e.target.value))
                      }
                    />
                  </label>
                  <label>
                    <span>Threads</span>
                    <input
                      type="number"
                      min="0"
                      value={draft.parameters.num_threads}
                      onChange={(e) =>
                        updateParameters("num_threads", Number(e.target.value))
                      }
                    />
                  </label>
                  <label>
                    <span>Keep alive</span>
                    <input
                      value={draft.parameters.keep_alive}
                      onChange={(e) =>
                        updateParameters("keep_alive", e.target.value)
                      }
                    />
                  </label>
                  <label className="wizard-toggle">
                    <span>Descarregar após review</span>
                    <input
                      type="checkbox"
                      checked={draft.parameters.unload_model_after_review}
                      onChange={(e) =>
                        updateParameters(
                          "unload_model_after_review",
                          e.target.checked,
                        )
                      }
                    />
                  </label>
                </>
              )}
            </div>
          )}
          {step === 4 && (
            <div className="wizard-form">
              <label>
                <span>Nome do profile</span>
                <input
                  value={draft.profile.name}
                  onChange={(e) => updateProfile("name", e.target.value)}
                  required
                />
              </label>
              <label>
                <span>Descrição</span>
                <input
                  value={draft.profile.description}
                  onChange={(e) => updateProfile("description", e.target.value)}
                />
              </label>
              <label>
                <span>Caracteres por bloco</span>
                <input
                  type="number"
                  min="1"
                  value={draft.policy.max_block_chars}
                  onChange={(e) =>
                    updatePolicy("max_block_chars", Number(e.target.value))
                  }
                />
              </label>
              <label>
                <span>Arquivos por bloco</span>
                <input
                  type="number"
                  min="1"
                  value={draft.policy.max_files_per_block}
                  onChange={(e) =>
                    updatePolicy("max_files_per_block", Number(e.target.value))
                  }
                />
              </label>
              <label>
                <span>Tentativas da resposta final</span>
                <input
                  type="number"
                  min="1"
                  value={draft.policy.review_final_retries}
                  onChange={(e) =>
                    updatePolicy("review_final_retries", Number(e.target.value))
                  }
                />
              </label>
              <label className="wizard-toggle">
                <span>Revisar PRs WIP</span>
                <input
                  type="checkbox"
                  checked={draft.policy.review_wip_pull_requests}
                  onChange={(e) =>
                    updatePolicy("review_wip_pull_requests", e.target.checked)
                  }
                />
              </label>
              <label className="wizard-toggle">
                <span>Revisar PRs próprios</span>
                <input
                  type="checkbox"
                  checked={draft.policy.review_own_pull_requests}
                  onChange={(e) =>
                    updatePolicy("review_own_pull_requests", e.target.checked)
                  }
                />
              </label>
              <label className="wizard-toggle">
                <span>Permitir rejeição autônoma</span>
                <input
                  type="checkbox"
                  checked={draft.policy.allow_autonomous_rejection}
                  onChange={(e) =>
                    updatePolicy("allow_autonomous_rejection", e.target.checked)
                  }
                />
              </label>
            </div>
          )}
          {step === 5 && (
            <div className="setup-review">
              <article>
                <span>Provider</span>
                <strong>
                  {draft.provider === "openrouter" ? "OpenRouter" : "Ollama"}
                </strong>
                <small>{draft.connection.base_url}</small>
              </article>
              <article>
                <span>Modelo</span>
                <strong>{draft.model?.name}</strong>
                <small>{draft.model?.id}</small>
              </article>
              <article>
                <span>Execução</span>
                <strong>Temperature {draft.parameters.temperature}</strong>
                <small>
                  {draft.provider === "openrouter"
                    ? `${draft.parameters.max_output_tokens.toLocaleString("pt-BR")} tokens de saída`
                    : `Timeout de ${draft.parameters.timeout_seconds}s`}
                </small>
              </article>
              <article>
                <span>Profile padrão</span>
                <strong>{draft.profile.name}</strong>
                <small>
                  {draft.policy.max_block_chars} caracteres por bloco
                </small>
              </article>
              <div className="setup-notice">
                A conexão, o modelo, os parâmetros, o profile e a policy serão
                gravados juntos. Os prompts internos versionados pelo
                ForgeReview não serão alterados.
              </div>
            </div>
          )}
        </div>
        {step > 0 && (
          <footer className="wizard-actions">
            <button
              className="button button-secondary"
              onClick={() => {
                setError("");
                setStep((value) => value - 1);
              }}
              disabled={busy}
            >
              Voltar
            </button>
            {step === 1 ? (
              <button
                className="button button-primary"
                onClick={validateConnection}
                disabled={busy}
              >
                {busy ? "Validando..." : "Validar e buscar modelos"}
              </button>
            ) : step === 5 ? (
              <button
                className="button button-primary"
                onClick={submit}
                disabled={busy}
              >
                {busy ? "Salvando..." : "Concluir configuração"}
              </button>
            ) : (
              <button
                className="button button-primary"
                onClick={() => setStep((value) => value + 1)}
                disabled={!canNext}
              >
                Continuar
              </button>
            )}
          </footer>
        )}
      </div>
    </section>
  );
}
