"use client";

import type { AdminRequest } from "@/lib/admin-client";
import type {
  ReviewSettingsData,
  ReviewSettingsPipeline,
  ReviewSettingsProfile,
  ReviewSettingsStage,
  ReviewSettingsStageType,
} from "@/lib/contracts";
import {
  AlertTriangle,
  Boxes,
  Check,
  ChevronRight,
  Clock3,
  Cpu,
  Database,
  FileJson,
  Layers3,
  RefreshCw,
  Settings2,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  UserRoundCog,
  Workflow,
  X,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

type Props = {
  request: AdminRequest;
  onAuthError: () => void;
};

type SettingsTab = "overview" | "profiles" | "pipeline" | "catalog";
type Detail =
  | { kind: "profile"; value: ReviewSettingsProfile }
  | { kind: "stage"; value: ReviewSettingsStage }
  | { kind: "type"; value: ReviewSettingsStageType };

const tabs: Array<{
  id: SettingsTab;
  label: string;
  description: string;
  icon: typeof Settings2;
}> = [
  {
    id: "overview",
    label: "Visão geral",
    description: "Mapa da configuração ativa",
    icon: SlidersHorizontal,
  },
  {
    id: "profiles",
    label: "Perfis",
    description: "Modelos, policies e prompts",
    icon: UserRoundCog,
  },
  {
    id: "pipeline",
    label: "Pipeline",
    description: "Versões e etapas publicadas",
    icon: Workflow,
  },
  {
    id: "catalog",
    label: "Tipos de etapa",
    description: "Executors e contratos",
    icon: Boxes,
  },
];

export function SettingsArea({ request, onAuthError }: Props) {
  const [data, setData] = useState<ReviewSettingsData | null>(null);
  const [tab, setTab] = useState<SettingsTab>("overview");
  const [selectedPipeline, setSelectedPipeline] = useState<number | null>(null);
  const [detail, setDetail] = useState<Detail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const closeDetail = useCallback(() => setDetail(null), []);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const response = await request<ReviewSettingsData>("review/settings");
      setData(response);
      setSelectedPipeline((current) => response.pipelines.some((pipeline) => pipeline.id === current) ? current : response.pipelines[0]?.id ?? null);
      setError("");
    } catch (failure) {
      if (failure instanceof Error && failure.message === "AUTH") {
        onAuthError();
        return;
      }
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setLoading(false);
    }
  }, [onAuthError, request]);

  useEffect(() => {
    void load();
  }, [load]);

  if (loading && !data) return <SettingsSkeleton />;

  if (error && !data) {
    return (
      <div className="settings-empty-state">
        <span><AlertTriangle size={25} /></span>
        <h2>Não foi possível carregar as configurações</h2>
        <p>{error}</p>
        <button className="primary-button" onClick={() => void load()}>
          <RefreshCw size={17} /> Tentar novamente
        </button>
      </div>
    );
  }

  if (!data) return null;

  const activeProfile =
    data.profiles.find((profile) => profile.is_default && profile.is_enabled);
  const activePipeline = effectivePipeline(data.pipelines, activeProfile);
  const browsedPipeline = data.pipelines.find((pipeline) => pipeline.id === selectedPipeline) ?? data.pipelines[0];
  const totalStages = data.pipelines.reduce(
    (total, pipeline) => total + pipeline.stages.length,
    0,
  );

  return (
    <div className={`settings-page ${tab === "pipeline" ? "studio-embedded" : ""}`}>
      {error && <div className="banner error">{error}</div>}
      <nav className="settings-tabs" aria-label="Áreas de configuração">
        {tabs.map((item) => {
          const Icon = item.icon;
          return (
            <button
              key={item.id}
              className={tab === item.id ? "active" : ""}
              onClick={() => setTab(item.id)}
              aria-current={tab === item.id ? "page" : undefined}
            >
              <span className="settings-tab-icon"><Icon size={18} /></span>
              <span><strong>{item.label}</strong><small>{item.description}</small></span>
            </button>
          );
        })}
      </nav>

      {tab === "overview" && (
        <OverviewTab
          data={data}
          activeProfile={activeProfile}
          activePipeline={activePipeline}
          totalStages={totalStages}
          onProfile={(profile) => setDetail({ kind: "profile", value: profile })}
          onStage={(stage) => setDetail({ kind: "stage", value: stage })}
          onNavigate={setTab}
        />
      )}
      {tab === "profiles" && (
        <ProfilesTab
          profiles={data.profiles}
          pipelines={data.pipelines}
          onOpen={(profile) => setDetail({ kind: "profile", value: profile })}
        />
      )}
      {tab === "pipeline" && (
        <PipelineTab pipelines={data.pipelines} selected={browsedPipeline} onSelect={setSelectedPipeline} onStage={(stage) => setDetail({ kind: "stage", value: stage })} onSetCurrent={async (pipeline) => { await request(`review/pipelines/${pipeline.id}/select`, { method: "POST" }); await load(); }} />
      )}
      {tab === "catalog" && (
        <CatalogTab
          types={data.stage_catalog}
          onOpen={(type) => setDetail({ kind: "type", value: type })}
        />
      )}

      {detail && <SettingsDetail detail={detail} onClose={closeDetail} request={request} onAuthError={onAuthError} onSaved={() => void load()} />}
    </div>
  );
}

function OverviewTab({
  data,
  activeProfile,
  activePipeline,
  totalStages,
  onProfile,
  onStage,
  onNavigate,
}: {
  data: ReviewSettingsData;
  activeProfile?: ReviewSettingsProfile;
  activePipeline?: ReviewSettingsPipeline;
  totalStages: number;
  onProfile: (profile: ReviewSettingsProfile) => void;
  onStage: (stage: ReviewSettingsStage) => void;
  onNavigate: (tab: SettingsTab) => void;
}) {
  return (
    <div className="settings-overview">
      <div className="settings-stat-grid">
        <SettingsStat icon={UserRoundCog} value={data.profiles.length} label="Perfis configurados" />
        <SettingsStat icon={Workflow} value={data.pipelines.length} label="Pipelines publicados" />
        <SettingsStat icon={Layers3} value={totalStages} label="Etapas em execução" />
        <SettingsStat icon={Boxes} value={data.stage_catalog.length} label="Tipos disponíveis" />
      </div>

      <div className="settings-overview-grid">
        <section className="settings-feature-card">
          <div className="settings-feature-copy">
            <span className="settings-kicker"><Sparkles size={14} /> Configuração efetiva</span>
            <h2>{activeProfile?.name ?? "Nenhum perfil ativo"}</h2>
            <p>
              {activeProfile?.description ||
                "Este é o perfil selecionado para as próximas revisões do workspace."}
            </p>
            <div className="settings-feature-facts">
              <span><small>Provider</small><strong>{activeProfile?.model?.provider || "Não definido"}</strong></span>
              <span><small>Modelo</small><strong>{activeProfile?.model?.name || "Não definido"}</strong></span>
              <span><small>Confiança mínima</small><strong>{percent(activeProfile?.policy?.minimum_confidence)}</strong></span>
            </div>
          </div>
          {activeProfile && (
            <button onClick={() => onProfile(activeProfile)}>
              Ver perfil completo <ChevronRight size={16} />
            </button>
          )}
        </section>

        <section className="settings-readiness-card">
          <div className="settings-card-heading">
            <span className="settings-card-icon"><ShieldCheck size={20} /></span>
            <div><small>Integridade</small><h2>Pronto para revisar</h2></div>
          </div>
          <div className="settings-check-list">
            <StatusLine ok={Boolean(activeProfile?.model?.is_ready)} label="Rota de modelo habilitada" />
            <StatusLine ok={Boolean(activeProfile?.policy)} label="Policy de execução carregada" />
            <StatusLine ok={Boolean(activePipeline?.stages.length && activePipeline.stages.every((stage) => stage.is_type_enabled && stage.is_model_ready))} label="Pipeline publicado disponível" />
            <StatusLine ok={Boolean(activeProfile?.prompt)} label="Prompt base versionado" optional />
          </div>
        </section>
      </div>

      {activePipeline && (
        <section className="settings-pipeline-preview">
          <div className="settings-section-head">
            <div><span className="eyebrow">Pipeline ativo</span><h2>{activePipeline.name}</h2></div>
            <button className="secondary-button" onClick={() => onNavigate("pipeline")}>Explorar pipeline</button>
          </div>
          <div className="settings-stage-ribbon">
            {activePipeline.stages.map((stage) => (
              <button key={stage.id} onClick={() => onStage(stage)}>
                <span>{stage.position}</span>
                <strong>{stage.name}</strong>
                <small>{stage.executor_key}</small>
              </button>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}

function ProfilesTab({ profiles, pipelines, onOpen }: {
  profiles: ReviewSettingsProfile[];
  pipelines: ReviewSettingsPipeline[];
  onOpen: (profile: ReviewSettingsProfile) => void;
}) {
  return (
    <section className="settings-section-card">
      <div className="settings-section-head">
        <div><span className="eyebrow">Rotas de review</span><h2>Perfis configurados</h2><p>Veja como modelo, policy e prompt se combinam em cada rota.</p></div>
        <span className="settings-count">{profiles.length} {profiles.length === 1 ? "perfil" : "perfis"}</span>
      </div>
      <div className="settings-profile-grid">
        {profiles.map((profile) => {
          const pipeline = effectivePipeline(pipelines, profile);
          return (
            <button key={profile.id} className="settings-profile-card" onClick={() => onOpen(profile)}>
              <div className="settings-profile-top">
                <span className="settings-avatar">{initials(profile.name)}</span>
                <span className={`settings-state ${profile.is_enabled ? "enabled" : "disabled"}`}>
                  {profile.is_enabled ? "Ativo" : "Inativo"}
                </span>
              </div>
              <div><h3>{profile.name}</h3><p>{profile.description || "Perfil de revisão sem descrição."}</p></div>
              <dl>
                <div><dt>Modelo</dt><dd>{profile.model?.name || "Não definido"}</dd></div>
                <div><dt>Pipeline</dt><dd>{pipeline?.name || "Fallback global"}</dd></div>
                <div><dt>Prompt</dt><dd>{profile.prompt ? `v${profile.prompt.version}` : "Padrão da etapa"}</dd></div>
              </dl>
              <span className="settings-card-action">Abrir configuração <ChevronRight size={15} /></span>
            </button>
          );
        })}
      </div>
    </section>
  );
}

function PipelineTab({ pipelines, selected, onSelect, onStage, onSetCurrent }: {
  pipelines: ReviewSettingsPipeline[];
  selected?: ReviewSettingsPipeline;
  onSelect: (id: number) => void;
  onStage: (stage: ReviewSettingsStage) => void;
  onSetCurrent: (pipeline: ReviewSettingsPipeline) => Promise<void>;
}) {
  return (
    <div className="settings-pipeline-layout">
      <aside className="settings-pipeline-list">
        <div><span className="eyebrow">Versões publicadas</span><h2>Pipelines</h2></div>
        {pipelines.map((pipeline) => (
          <button key={pipeline.id} className={selected?.id === pipeline.id ? "active" : ""} onClick={() => onSelect(pipeline.id)}>
            <span className="settings-pipeline-symbol"><Workflow size={17} /></span>
            <span><strong>{pipeline.name}</strong><small>{pipeline.profile_name || "Fallback global"}</small></span>
            <ChevronRight size={15} />
          </button>
        ))}
      </aside>
      {selected ? (
        <section className="settings-pipeline-detail">
          <div className="settings-pipeline-hero">
            <div>
              <span className="settings-kicker"><Database size={14} /> {selected.profile_name || "Sistema"}</span>
              <h2>{selected.name}</h2>
              <p>{selected.description}</p>
            </div>
            <div className="settings-version-stamp"><small>Versão publicada</small><strong>v{selected.version}</strong><span>{dateLabel(selected.published_at)}</span>{!selected.is_default && <button className="secondary-button" onClick={() => void onSetCurrent(selected)}>Definir atual</button>}</div>
          </div>
          <div className="settings-stage-list">
            {selected.stages.map((stage, index) => (
              <button key={stage.id} className="settings-stage-row" onClick={() => onStage(stage)}>
                <span className="settings-stage-index">{String(stage.position).padStart(2, "0")}</span>
                <span className="settings-stage-glyph">{stage.use_llm ? <Sparkles size={18} /> : <Cpu size={18} />}</span>
                <span className="settings-stage-copy"><strong>{stage.name}</strong><small>{stage.stage_type_key} · {stage.executor_key}</small></span>
                <span className="settings-stage-badges">
                  {stage.is_required && <em>Obrigatória</em>}
                  <em>{stage.use_llm ? "IA" : "Sistema"}</em>
                </span>
                <ChevronRight size={16} />
                {index < selected.stages.length - 1 && <i aria-hidden />}
              </button>
            ))}
          </div>
        </section>
      ) : <div className="settings-empty-inline">Nenhum pipeline publicado.</div>}
    </div>
  );
}

function CatalogTab({ types, onOpen }: {
  types: ReviewSettingsStageType[];
  onOpen: (type: ReviewSettingsStageType) => void;
}) {
  return (
    <section className="settings-section-card">
      <div className="settings-section-head">
        <div><span className="eyebrow">Capacidades do runtime</span><h2>Catálogo de tipos</h2><p>Cada tipo referencia um executor nativo e contratos controlados.</p></div>
        <span className="settings-count">{types.length} tipos</span>
      </div>
      <div className="settings-catalog-grid">
        {types.map((type) => (
          <button key={type.id} className="settings-type-card" onClick={() => onOpen(type)}>
            <div className="settings-type-head">
              <span><FileJson size={20} /></span>
              <em>{type.is_system ? "Sistema" : "Custom"}</em>
            </div>
            <h3>{type.name}</h3>
            <code>{type.executor_key}</code>
            <div className="settings-contract-flow">
              <span><small>Entrada</small><strong>{type.input_contract?.key || "início"}</strong></span>
              <ChevronRight size={14} />
              <span><small>Saída</small><strong>{type.output_contract?.key || "terminal"}</strong></span>
            </div>
            <span className="settings-card-action">Inspecionar contratos <ChevronRight size={15} /></span>
          </button>
        ))}
      </div>
    </section>
  );
}

function SettingsDetail({ detail, onClose, request, onAuthError, onSaved }: { detail: Detail; onClose: () => void; request: AdminRequest; onAuthError: () => void; onSaved: () => void; }) {
  const dialogRef = useRef<HTMLElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const previousFocus = document.activeElement as HTMLElement | null;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
        return;
      }
      if (event.key !== "Tab" || !dialogRef.current) return;
      const focusable = Array.from(
        dialogRef.current.querySelectorAll<HTMLElement>(
          'button, a[href], input, select, textarea, summary, [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((element) => !element.hasAttribute("disabled"));
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      document.body.style.overflow = previousOverflow;
      previousFocus?.focus();
    };
  }, [onClose]);

  return (
    <div className="settings-modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section ref={dialogRef} className="settings-modal" role="dialog" aria-modal="true" aria-label="Detalhes da configuração">
        <header>
          <div><span className="eyebrow">{detail.kind === "profile" ? "Configuração editável" : "Configuração somente leitura"}</span><h2>{detail.value.name}</h2></div>
          <button ref={closeRef} onClick={onClose} aria-label="Fechar"><X size={19} /></button>
        </header>
        <div className="settings-modal-content">
          {detail.kind === "profile" && <ProfileDetail profile={detail.value} request={request} onAuthError={onAuthError} onSaved={onSaved} />}
          {detail.kind === "stage" && <StageDetail stage={detail.value} />}
          {detail.kind === "type" && <TypeDetail type={detail.value} />}
        </div>
      </section>
    </div>
  );
}

function ProfileDetail({ profile, request, onAuthError, onSaved }: { profile: ReviewSettingsProfile; request: AdminRequest; onAuthError: () => void; onSaved: () => void; }) {
  const policy = profile.policy;
  const [form, setForm] = useState({
    name: profile.name,
    description: profile.description || "",
    is_enabled: profile.is_enabled,
    is_default: profile.is_default,
    max_block_chars: policy?.max_block_chars ?? 12000,
    max_files_per_block: policy?.max_files_per_block ?? 4,
    minimum_confidence: policy?.minimum_confidence ?? 0.75,
    max_parallel_groups: policy?.max_parallel_groups ?? 1,
    allow_autonomous_rejection: policy?.allow_autonomous_rejection ?? false,
    publish_manual_reviews: policy?.publish_manual_reviews ?? false,
    enable_detailed_stage_logs: policy?.enable_detailed_stage_logs ?? false,
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  async function save() {
    setSaving(true);
    setError("");
    try {
      const profilePatch: Record<string, unknown> = {};
      if (form.name !== profile.name) profilePatch.name = form.name;
      if (form.description !== (profile.description || "")) profilePatch.description = form.description;
      if (form.is_enabled !== profile.is_enabled) profilePatch.is_enabled = form.is_enabled ? 1 : 0;
      if (form.is_default !== profile.is_default) profilePatch.is_default = form.is_default ? 1 : 0;
      if (Object.keys(profilePatch).length > 0) {
        await request(`review/profiles/${profile.id}`, {
          method: "PATCH",
          body: JSON.stringify(profilePatch),
        });
      }
      if (policy?.id) {
        const policyPatch: Record<string, unknown> = {};
        if (Number(form.max_block_chars) !== policy.max_block_chars) policyPatch.max_block_chars = Number(form.max_block_chars);
        if (Number(form.max_files_per_block) !== policy.max_files_per_block) policyPatch.max_files_per_block = Number(form.max_files_per_block);
        if (Number(form.minimum_confidence) !== policy.minimum_confidence) policyPatch.review_min_publish_confidence = Number(form.minimum_confidence);
        if (Number(form.max_parallel_groups) !== policy.max_parallel_groups) policyPatch.review_max_parallel_groups = Number(form.max_parallel_groups);
        if (form.allow_autonomous_rejection !== policy.allow_autonomous_rejection) policyPatch.allow_autonomous_rejection = form.allow_autonomous_rejection ? 1 : 0;
        if (form.publish_manual_reviews !== policy.publish_manual_reviews) policyPatch.publish_manual_reviews = form.publish_manual_reviews ? 1 : 0;
        if (form.enable_detailed_stage_logs !== policy.enable_detailed_stage_logs) policyPatch.enable_detailed_stage_logs = form.enable_detailed_stage_logs ? 1 : 0;
        if (Object.keys(policyPatch).length > 0) {
          await request(`review/policies/${policy.id}`, {
            method: "PATCH",
            body: JSON.stringify(policyPatch),
          });
        }
      }
      onSaved();
    } catch (failure) {
      if (failure instanceof Error && failure.message === "AUTH") return onAuthError();
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      {error && <div className="banner error">{error}</div>}
      <div className="settings-detail-summary">
        <span className="settings-avatar large">{initials(profile.name)}</span>
        <div><strong>{profile.model?.name || "Sem modelo"}</strong><small>{profile.model ? `${profile.model.provider} · ${profile.model.connection}` : "Nenhuma rota de IA vinculada"}</small></div>
        <span className={`settings-state ${profile.is_enabled ? "enabled" : "disabled"}`}>{profile.is_enabled ? "Ativo" : "Inativo"}</span>
      </div>
      <section className="settings-edit-section">
        <div className="settings-edit-heading"><div><h3>Identidade e estado</h3><p>Defina como este perfil aparece e se pode ser usado pelo runtime.</p></div></div>
        <div className="settings-form-grid">
        <label><span>Nome</span><input value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} /></label>
        <label><span>Descrição</span><input value={form.description} onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))} /></label>
        <SwitchField checked={form.is_enabled} label="Perfil habilitado" onChange={(checked) => setForm((current) => ({ ...current, is_enabled: checked }))} />
        <SwitchField checked={form.is_default} label="Perfil padrão" onChange={(checked) => setForm((current) => ({ ...current, is_default: checked }))} />
        </div>
      </section>
      <section className="settings-edit-section">
        <div className="settings-edit-heading"><div><h3>Regras de execução</h3><p>Limites aplicados às revisões deste perfil.</p></div>{policy && <span className="settings-edit-badge">Policy #{policy.id}</span>}</div>
      {policy ? (
        <>
        <div className="settings-form-grid policy">
          <label><span>Bloco máximo</span><input type="number" value={form.max_block_chars} onChange={(event) => setForm((current) => ({ ...current, max_block_chars: Number(event.target.value) }))} /></label>
          <label><span>Arquivos por grupo</span><input type="number" value={form.max_files_per_block} onChange={(event) => setForm((current) => ({ ...current, max_files_per_block: Number(event.target.value) }))} /></label>
          <label><span>Confiança mínima</span><input type="number" min="0" max="1" step="0.01" value={form.minimum_confidence} onChange={(event) => setForm((current) => ({ ...current, minimum_confidence: Number(event.target.value) }))} /></label>
          <label><span>Grupos paralelos</span><input type="number" min="1" step="1" value={form.max_parallel_groups} onChange={(event) => setForm((current) => ({ ...current, max_parallel_groups: Number(event.target.value) }))} /></label>
          <SwitchField checked={form.allow_autonomous_rejection} label="Permitir rejeição autônoma" onChange={(checked) => setForm((current) => ({ ...current, allow_autonomous_rejection: checked }))} />
          <SwitchField checked={form.publish_manual_reviews} label="Exigir aprovação antes de publicar" onChange={(checked) => setForm((current) => ({ ...current, publish_manual_reviews: checked }))} />
          <SwitchField checked={form.enable_detailed_stage_logs} label="Logs detalhados por etapa" onChange={(checked) => setForm((current) => ({ ...current, enable_detailed_stage_logs: checked }))} />
        </div>
        </>
      ) : <p className="settings-muted">Este perfil não possui policy vinculada.</p>}
      </section>
      <div className="settings-detail-actions">
        <button className="primary-button" onClick={() => void save()} disabled={saving}>
          {saving ? <RefreshCw className="spin" size={16} /> : <Check size={16} />} Salvar alterações
        </button>
      </div>
      <h3 className="settings-detail-title">Prompt base</h3>
      {profile.prompt ? (
        <div className="settings-prompt-block">
          <div><span>{profile.prompt.name}</span><em>v{profile.prompt.version}</em></div>
          <pre>{profile.prompt.content}</pre>
        </div>
      ) : <p className="settings-muted">As instruções específicas de cada etapa serão utilizadas.</p>}
    </>
  );
}

function SwitchField({ checked, label, onChange }: { checked: boolean; label: string; onChange: (checked: boolean) => void }) {
  return (
    <label className="settings-switch">
      <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      <span className="settings-switch-track" aria-hidden="true"><i /></span>
      <span>{label}</span>
    </label>
  );
}

function StageDetail({ stage }: { stage: ReviewSettingsStage }) {
  return (
    <>
      <div className="settings-detail-grid">
        <DetailFact label="Executor" value={stage.executor_key} />
        <DetailFact label="Tipo" value={stage.stage_type_key} />
        <DetailFact label="Tokens de saída" value={stage.max_output_tokens ? stage.max_output_tokens.toLocaleString("pt-BR") : "Não aplicável"} />
        <DetailFact label="Tentativas" value={String(stage.retry_limit)} />
        <DetailFact label="Timeout" value={`${stage.timeout_seconds}s`} />
        <DetailFact label="Modelo" value={stage.model_name || "Modelo do perfil"} />
        <DetailFact label="Rota da etapa" value={stage.is_model_ready ? "Disponível" : "Indisponível"} />
        <DetailFact label="Contrato de entrada" value={stage.input_contract || "Início do fluxo"} />
        <DetailFact label="Contrato de saída" value={stage.output_contract || "Etapa terminal"} />
      </div>
      <h3 className="settings-detail-title">Prompt da etapa</h3>
      <div className="settings-prompt-block"><pre>{stage.prompt_template || "Esta etapa é determinística e não utiliza prompt."}</pre></div>
    </>
  );
}

function TypeDetail({ type }: { type: ReviewSettingsStageType }) {
  return (
    <>
      <div className="settings-detail-grid compact">
        <DetailFact label="Chave" value={type.key} />
        <DetailFact label="Executor registrado" value={type.executor_key} />
        <DetailFact label="Origem" value={type.is_system ? "Sistema" : "Custom"} />
        <DetailFact label="Estado" value={type.is_enabled ? "Disponível" : "Desabilitado"} />
      </div>
      {[type.input_contract, type.output_contract].filter(Boolean).map((contract, index) => contract && (
        <div className="settings-contract-detail" key={contract.id}>
          <div><span>{index === 0 && type.input_contract ? "Contrato de entrada" : "Contrato de saída"}</span><strong>{contract.key} · v{contract.version}</strong></div>
          <p>{contract.response_instruction || "Contrato consumido internamente pelo executor."}</p>
          <small>Validador semântico</small><code>{contract.semantic_validator_key || "não aplicável"}</code>
          <details><summary>Visualizar JSON Schema</summary><pre>{JSON.stringify(contract.schema, null, 2)}</pre></details>
        </div>
      ))}
    </>
  );
}

function SettingsStat({ icon: Icon, value, label }: { icon: typeof Settings2; value: number; label: string }) {
  return <div className="settings-stat"><span><Icon size={19} /></span><div><strong>{value}</strong><small>{label}</small></div></div>;
}

function StatusLine({ ok, label, optional }: { ok: boolean; label: string; optional?: boolean }) {
  return <div className={ok ? "ok" : optional ? "optional" : "warning"}><span>{ok ? <Check size={14} /> : optional ? <Sparkles size={14} /> : <AlertTriangle size={14} />}</span><strong>{label}</strong><small>{ok ? "Configurado" : optional ? "Opcional" : "Atenção"}</small></div>;
}

function DetailFact({ label, value }: { label: string; value: string }) {
  return <div className="settings-detail-fact"><small>{label}</small><strong>{value}</strong></div>;
}

function SettingsSkeleton() {
  return <div className="settings-skeleton"><div /><div /><section><div /><div /><div /></section></div>;
}

function initials(name: string) {
  return name.split(/\s+/).slice(0, 2).map((part) => part[0]).join("").toUpperCase();
}

function percent(value?: number) {
  return value === undefined ? "Não definido" : `${Math.round(value * 100)}%`;
}

function dateLabel(value: string) {
  if (!value) return "Data não registrada";
  const date = new Date(value.includes("T") ? value : `${value.replace(" ", "T")}Z`);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("pt-BR", { dateStyle: "medium" }).format(date);
}

function effectivePipeline(
  pipelines: ReviewSettingsPipeline[],
  profile?: ReviewSettingsProfile,
) {
  return (
    pipelines.find(
      (pipeline) =>
        profile &&
        profile.is_enabled &&
        pipeline.profile_id === profile.id &&
        pipeline.is_default &&
        pipeline.is_enabled,
    ) ??
    pipelines.find(
      (pipeline) =>
        pipeline.profile_id === undefined &&
        pipeline.is_default &&
        pipeline.is_enabled,
    )
  );
}
