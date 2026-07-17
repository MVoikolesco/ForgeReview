"use client";

import { StepperModal } from "@/components/stepper/stepper-modal";
import type { AdminRequest } from "@/lib/admin-client";
import type { CatalogModel, SetupDraft } from "@/lib/contracts";
import {
  ArrowLeft,
  ArrowRight,
  Box,
  Check,
  CheckCircle2,
  Cpu,
  KeyRound,
  Loader2,
  Route,
  Search,
  Server,
  Sparkles,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";

type Props = {
  request: AdminRequest;
  existingProfiles: number;
  existingConnection?: {
    id: number;
    provider: SetupDraft["provider"];
    name: string;
    base_url: string;
    api_key_configured: boolean;
    http_referer?: string;
    app_title?: string;
  };
  onClose: () => void;
  onComplete: () => Promise<void>;
};
type CatalogResponse = { models?: CatalogModel[] };

const steps = [
  ["Provider", "Escolha a origem"],
  ["Conexão", "Valide o acesso"],
  ["Modelo", "Defina o motor"],
  ["Revisão", "Confirme a rota"],
] as const;

const defaults: SetupDraft = {
  provider: "ollama",
  connection: {
    name: "Ollama local",
    base_url: "http://localhost:11434",
    api_key: "",
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
    unload_model_after_review: false,
  },
};

export function ConnectionWizard({
  request,
  existingProfiles,
  existingConnection,
  onClose,
  onComplete,
}: Props) {
  const addMode = Boolean(existingConnection);
  const [step, setStep] = useState(addMode ? 2 : 0);
  const [draft, setDraft] = useState<SetupDraft>(() =>
    existingConnection
      ? {
          ...defaults,
          provider:
            existingConnection.provider === "ollama" &&
            existingConnection.api_key_configured
              ? "ollama-cloud"
              : existingConnection.provider,
          connection: { ...defaults.connection, ...existingConnection },
        }
      : defaults,
  );
  const [catalog, setCatalog] = useState<CatalogModel[]>([]);
  const [search, setSearch] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [makeDefault, setMakeDefault] = useState(false);

  useEffect(() => {
    if (!existingConnection) return;
    setBusy(true);
    request<CatalogResponse>("setup/catalog", {
      method: "POST",
      body: JSON.stringify({
        provider: existingConnection.provider,
        connection: existingConnection,
      }),
    })
      .then((result) => setCatalog(result.models || []))
      .catch((failure) =>
        setError(failure instanceof Error ? failure.message : String(failure)),
      )
      .finally(() => setBusy(false));
  }, [existingConnection, request]);

  const visibleModels = useMemo(() => {
    const term = search.trim().toLowerCase();
    return (
      term
        ? catalog.filter((model) =>
            `${model.name} ${model.id}`.toLowerCase().includes(term),
          )
        : catalog
    ).slice(0, 80);
  }, [catalog, search]);

  function chooseProvider(provider: SetupDraft["provider"]) {
    setDraft((value) => ({
      ...value,
      provider,
      model: null,
      connection:
        provider === "openrouter"
          ? {
              name: "OpenRouter principal",
              base_url: "https://openrouter.ai/api/v1",
              api_key: "",
              http_referer: "",
              app_title: "ForgeReview",
            }
          : provider === "ollama-cloud"
            ? {
                name: "Ollama Cloud",
                base_url: "https://ollama.com",
                api_key: "",
                http_referer: "",
                app_title: "ForgeReview",
              }
            : {
                name: "Ollama local",
                base_url: "http://localhost:11434",
                api_key: "",
                http_referer: "",
                app_title: "ForgeReview",
              },
    }));
  }

  function updateConnection(
    field: keyof SetupDraft["connection"],
    value: string,
  ) {
    setDraft((current) => ({
      ...current,
      connection: { ...current.connection, [field]: value },
    }));
  }

  async function validateConnection() {
    setBusy(true);
    setError("");
    try {
      const result = await request<CatalogResponse>("setup/catalog", {
        method: "POST",
        body: JSON.stringify({
          provider: draft.provider,
          connection: draft.connection,
        }),
      });
      if (!result.models?.length)
        throw new Error(
          "A conexão respondeu, mas nenhum modelo foi encontrado.",
        );
      setCatalog(result.models);
      setStep(2);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  }

  async function finish() {
    if (!draft.model) return;
    setBusy(true);
    setError("");
    try {
      await request(addMode ? "setup/add-model" : "setup/complete", {
        method: "POST",
        body: JSON.stringify(
          addMode
            ? {
                connection_id: existingConnection?.id,
                model: draft.model,
                parameters: draft.parameters,
                make_default: makeDefault,
              }
            : { ...draft, provider: draft.provider },
        ),
      });
      await onComplete();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
      setBusy(false);
    }
  }

  function next() {
    setError("");
    if (step === 0) return setStep(1);
    if (step === 1) return void validateConnection();
    if (step === 2 && draft.model) return setStep(3);
    if (step === 3) return void finish();
  }

  const canContinue = step !== 2 || Boolean(draft.model);
  return (
    <StepperModal
      steps={steps}
      step={step}
      eyebrow={`Etapa ${step + 1} de ${steps.length}`}
      title={wizardTitle(step)}
      description={wizardSubtitle(step)}
      brand={<Sparkles size={18} />}
      heading={addMode ? "Adicionar modelo" : "Nova conexão"}
      railDescription={
        addMode
          ? `Amplie ${existingConnection?.name} com outro modelo.`
          : "Configure uma rota completa em poucos passos."
      }
      footnote="As configurações são validadas antes de serem salvas."
      error={error}
      onClose={onClose}
      footer={
        <WizardFooter
          step={step}
          steps={steps.length}
          busy={busy}
          canContinue={canContinue}
          onBack={() => (step ? setStep((value) => value - 1) : onClose())}
          onNext={next}
          addMode={addMode}
        />
      }
    >
      <>
        {step === 0 && (
          <ProviderStep selected={draft.provider} onSelect={chooseProvider} />
        )}
        {step === 1 && (
          <ConnectionStep
            draft={draft}
            update={updateConnection}
            addMode={addMode}
            existingConnection={existingConnection}
          />
        )}
        {step === 2 && (
          <ModelStep
            models={visibleModels}
            selected={draft.model}
            search={search}
            setSearch={setSearch}
            onSelect={(model) => setDraft((current) => ({ ...current, model }))}
          />
        )}
        {step === 3 && (
          <ReviewStep
            draft={draft}
            existingProfiles={existingProfiles}
            addMode={addMode}
            makeDefault={makeDefault}
            setMakeDefault={setMakeDefault}
          />
        )}
      </>
    </StepperModal>
  );
}

function WizardFooter({
  step,
  steps,
  busy,
  canContinue,
  onBack,
  onNext,
  addMode,
}: {
  step: number;
  steps: number;
  busy: boolean;
  canContinue: boolean;
  onBack: () => void;
  onNext: () => void;
  addMode: boolean;
}) {
  return (
    <footer>
      <button className="secondary-button" onClick={onBack} disabled={busy}>
        <ArrowLeft size={17} />
        {step ? "Voltar" : "Cancelar"}
      </button>
      <span>
        Etapa {step + 1} de {steps}
      </span>
      <button
        className="primary-button"
        onClick={onNext}
        disabled={!canContinue || busy}
      >
        {busy ? (
          <Loader2 className="spin" size={17} />
        ) : step === 3 ? (
          <CheckCircle2 size={17} />
        ) : null}
        {busy
          ? step === 1
            ? "Validando..."
            : "Salvando..."
          : step === 3
            ? addMode
              ? "Adicionar modelo"
              : "Criar e usar conexão"
            : step === 1
              ? "Validar e buscar modelos"
              : "Continuar"}
        {!busy && step < 3 && <ArrowRight size={17} />}
      </button>
    </footer>
  );
}

function ProviderStep({
  selected,
  onSelect,
}: {
  selected: SetupDraft["provider"];
  onSelect: (provider: SetupDraft["provider"]) => void;
}) {
  return (
    <div className="provider-options">
      <button
        className={selected === "ollama" ? "selected" : ""}
        onClick={() => onSelect("ollama")}
      >
        <span className="provider-option-icon ollama">
          <Box />
        </span>
        <span className="provider-option-copy">
          <strong>Ollama</strong>
          <small>Modelos locais, privados e sem custo por token.</small>
          <em>Local</em>
        </span>
        <i>{selected === "ollama" && <Check size={14} />}</i>
      </button>
      <button
        className={selected === "openrouter" ? "selected" : ""}
        onClick={() => onSelect("openrouter")}
      >
        <span className="provider-option-icon openrouter">
          <Route />
        </span>
        <span className="provider-option-copy">
          <strong>OpenRouter</strong>
          <small>Acesso unificado aos principais modelos de IA.</small>
          <em>Cloud</em>
        </span>
        <i>{selected === "openrouter" && <Check size={14} />}</i>
      </button>
      <button
        className={selected === "ollama-cloud" ? "selected" : ""}
        onClick={() => onSelect("ollama-cloud")}
      >
        <span className="provider-option-icon ollama">
          <Box />
        </span>
        <span className="provider-option-copy">
          <strong>Ollama Cloud</strong>
          <small>Modelos hospedados com autenticação por chave.</small>
          <em>Cloud</em>
        </span>
        <i>{selected === "ollama-cloud" && <Check size={14} />}</i>
      </button>
    </div>
  );
}

function ConnectionStep({
  draft,
  update,
  addMode,
  existingConnection,
}: {
  draft: SetupDraft;
  update: (field: keyof SetupDraft["connection"], value: string) => void;
  addMode: boolean;
  existingConnection?: Props["existingConnection"];
}) {
  return (
    <div className="connection-form">
      <div className="selected-provider-line">
        <span>
          {draft.provider === "ollama" || draft.provider === "ollama-cloud" ? (
            <Box size={18} />
          ) : (
            <Route size={18} />
          )}
        </span>
        <div>
          <small>Provider selecionado</small>
          <strong>
            {draft.provider === "ollama"
              ? "Ollama local"
              : draft.provider === "ollama-cloud"
                ? "Ollama Cloud"
                : "OpenRouter"}
          </strong>
        </div>
      </div>
      <div className="form-grid">
        <label className="full">
          Nome da conexão
          <span className="input-with-icon">
            <Cpu size={16} />
            <input
              value={draft.connection.name}
              onChange={(event) => update("name", event.target.value)}
              placeholder="Ex.: OpenRouter Produção"
            />
          </span>
        </label>
        <label className="full">
          URL do endpoint
          <span className="input-with-icon">
            <Server size={16} />
            <input
              value={draft.connection.base_url}
              onChange={(event) => update("base_url", event.target.value)}
              placeholder="https://..."
            />
          </span>
          <small>O servidor deve conseguir alcançar este endereço.</small>
        </label>
        {(draft.provider === "openrouter" ||
          draft.provider === "ollama-cloud") && (
          <>
            <label className="full">
              API key{" "}
              {addMode && existingConnection?.api_key_configured
                ? "(deixe em branco para manter a chave configurada)"
                : ""}
              <span className="input-with-icon">
                <KeyRound size={16} />
                <input
                  type="password"
                  autoComplete="new-password"
                  value={draft.connection.api_key}
                  onChange={(event) => update("api_key", event.target.value)}
                  placeholder="Cole a chave para validar e salvar"
                />
              </span>
              <small>
                A chave é usada somente nesta operação e nunca é exibida
                novamente.
              </small>
            </label>
            {draft.provider === "openrouter" && (
              <label>
                Identificação da aplicação
                <input
                  value={draft.connection.app_title}
                  onChange={(event) => update("app_title", event.target.value)}
                />
              </label>
            )}
            <label>
              HTTP Referer <em>opcional</em>
              <input
                value={draft.connection.http_referer}
                onChange={(event) => update("http_referer", event.target.value)}
                placeholder="https://sua-aplicacao.com"
              />
            </label>
          </>
        )}
      </div>
      <div className="validation-note">
        <CheckCircle2 size={17} />
        <div>
          <strong>Validação em tempo real</strong>
          <p>
            No próximo passo consultaremos o catálogo diretamente no provider.
          </p>
        </div>
      </div>
    </div>
  );
}

function ModelStep({
  models,
  selected,
  search,
  setSearch,
  onSelect,
}: {
  models: CatalogModel[];
  selected: CatalogModel | null;
  search: string;
  setSearch: (value: string) => void;
  onSelect: (model: CatalogModel) => void;
}) {
  return (
    <div className="catalog-step">
      <label className="catalog-search">
        <Search size={18} />
        <input
          autoFocus
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="Buscar por nome ou identificador..."
        />
      </label>
      <div className="catalog-count">
        <span>{models.length} modelos encontrados</span>
        <small>Catálogo validado agora</small>
      </div>
      <div className="catalog-list">
        {models.map((model) => (
          <button
            key={model.id}
            className={selected?.id === model.id ? "selected" : ""}
            onClick={() => onSelect(model)}
          >
            <span className="catalog-radio">
              {selected?.id === model.id ? <CheckCircle2 /> : <span />}
            </span>
            <div>
              <strong>{model.name || model.id}</strong>
              <code>{model.id}</code>
            </div>
            <span className="catalog-spec">
              <small>Contexto</small>
              <strong>{formatContext(model.context_length)}</strong>
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}

function ReviewStep({
  draft,
  existingProfiles,
  addMode,
  makeDefault,
  setMakeDefault,
}: {
  draft: SetupDraft;
  existingProfiles: number;
  addMode: boolean;
  makeDefault: boolean;
  setMakeDefault: (value: boolean) => void;
}) {
  return (
    <div className="review-step">
      <div className="route-preview">
        <span className="route-node">
          <Server size={20} />
        </span>
        <div>
          <small>Conexão</small>
          <strong>{draft.connection.name}</strong>
          <code>{draft.connection.base_url}</code>
        </div>
        <ArrowRight />
        <span className="route-node accent">
          <Cpu size={20} />
        </span>
        <div>
          <small>Modelo padrão</small>
          <strong>{draft.model?.name}</strong>
          <code>{draft.model?.id}</code>
        </div>
      </div>
      <h3>O que será configurado</h3>
      <ul>
        {!addMode && (
          <li>
            <Check size={15} />
            <span>
              <strong>Nova conexão configurada</strong>
              <small>
                A rota será criada sem alterar o provider padrão global.
              </small>
            </span>
          </li>
        )}
        <li>
          <Check size={15} />
          <span>
            <strong>
              {addMode
                ? "Modelo adicionado à conexão"
                : "Modelo padrão da nova conexão"}
            </strong>
            <small>
              Você poderá definir padrões explicitamente nos detalhes.
            </small>
          </span>
        </li>
        {!addMode && (
          <li>
            <Check size={15} />
            <span>
              <strong>
                {existingProfiles
                  ? "Perfil existente atualizado"
                  : "Perfil de revisão criado"}
              </strong>
              <small>Parâmetros seguros e limites recomendados.</small>
            </span>
          </li>
        )}
      </ul>
      <div className="automatic-note">
        <Sparkles size={18} />
        <p>
          <strong>
            {addMode ? "Sem alterar a rota global." : "Tudo pronto."}
          </strong>{" "}
          Padrões globais só mudam quando você solicita explicitamente.
        </p>
      </div>
      {addMode && (
        <label className="default-choice">
          <input
            type="checkbox"
            checked={makeDefault}
            onChange={(event) => setMakeDefault(event.target.checked)}
          />
          Tornar este o modelo padrão desta conexão
        </label>
      )}
    </div>
  );
}

function wizardTitle(step: number) {
  return [
    "Escolha o provider",
    "Configure a conexão",
    "Selecione um modelo",
    "Revise e ative",
  ][step];
}
function wizardSubtitle(step: number) {
  return [
    "Qual infraestrutura fornecerá os modelos para suas revisões?",
    "Informe como o ForgeReview deve acessar este provider.",
    "Este será o modelo inicial e padrão da conexão.",
    "Confira a rota antes de salvar. Tudo será ativado automaticamente.",
  ][step];
}
function formatContext(value: number) {
  return value ? `${Math.round(value / 1000)}k tokens` : "Não informado";
}
