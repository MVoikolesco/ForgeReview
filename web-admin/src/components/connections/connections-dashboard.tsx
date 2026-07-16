"use client";

import { useEffect, useMemo, useState } from "react";
import { ArrowRight, Cable, Check, CircleCheck, Cpu, Plus, Search, Server, Sparkles } from "lucide-react";
import type { AdminRequest } from "@/lib/admin-client";
import type { Connection, ConnectionData, Model } from "@/lib/contracts";
import { ProviderMark } from "./provider-mark";
import { ConnectionDetail } from "./connection-detail";

type Props = {
  data: ConnectionData;
  request: AdminRequest;
  loading: boolean;
  onRefresh: () => Promise<void>;
  onNew: () => void;
};

export function ConnectionsDashboard({ data, request, loading, onRefresh, onNew }: Props) {
  const [selectedID, setSelectedID] = useState<number | null>(null);
  const [search, setSearch] = useState("");

  useEffect(() => {
    if (selectedID && !data.connections.some((connection) => connection.id === selectedID)) setSelectedID(null);
  }, [data.connections, selectedID]);

  const providers = useMemo(() => new Map(data.providers.map((provider) => [provider.id, provider])), [data.providers]);
  const modelCount = useMemo(() => {
    const count = new Map<number, number>();
    data.models.forEach((model) => count.set(model.connection_id, (count.get(model.connection_id) || 0) + 1));
    return count;
  }, [data.models]);
  const filtered = data.connections.filter((connection) => {
    const provider = providers.get(connection.provider_id);
    return `${connection.name} ${provider?.display_name} ${connection.base_url}`.toLowerCase().includes(search.toLowerCase());
  });
  const selected = data.connections.find((connection) => connection.id === selectedID);

  if (loading) return <DashboardSkeleton />;

  if (selected) {
    return <ConnectionDetail connection={selected} provider={providers.get(selected.provider_id)} models={data.models.filter((model) => model.connection_id === selected.id)} profiles={data.profiles} request={request} onBack={() => setSelectedID(null)} onRefresh={onRefresh} />;
  }

  const defaults = data.connections.filter((connection) => connection.is_default === 1).length;
  return (
    <>
      <section className="summary-strip">
        <Summary icon={<Cable />} label="Conexões" value={data.connections.length} detail={`${data.connections.filter((item) => item.is_enabled === 1).length} ativas`} />
        <Summary icon={<Cpu />} label="Modelos" value={data.models.length} detail="disponíveis" />
        <Summary icon={<CircleCheck />} label="Rotas padrão" value={defaults} detail="configuradas" />
      </section>

      <section className="connections-section">
        <div className="section-toolbar">
          <div><h2>Suas conexões</h2><p>Selecione uma conexão para ver modelos e configurações.</p></div>
          <label className="search-field"><Search size={17} /><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Buscar conexão..." /></label>
        </div>
        {!data.connections.length ? (
          <div className="empty-state">
            <span><Server size={28} /></span><h3>Crie sua primeira rota de IA</h3><p>Conecte Ollama ou OpenRouter e escolha o modelo que fará suas revisões.</p>
            <button className="primary-button" onClick={onNew}><Plus size={18} />Cadastrar conexão</button>
          </div>
        ) : !filtered.length ? (
          <div className="empty-state compact"><Search size={24} /><h3>Nenhuma conexão encontrada</h3><p>Tente buscar por outro nome ou provider.</p></div>
        ) : (
          <div className="connection-grid">
            {filtered.map((connection) => {
              const provider = providers.get(connection.provider_id);
              const defaultModel = data.models.find((model) => model.connection_id === connection.id && model.is_default === 1);
              return <ConnectionCard key={connection.id} connection={connection} providerName={provider?.name} providerLabel={provider?.display_name} models={modelCount.get(connection.id) || 0} defaultModel={defaultModel} onClick={() => setSelectedID(connection.id)} />;
            })}
            <button className="new-connection-card" onClick={onNew}><span><Plus size={22} /></span><strong>Adicionar conexão</strong><small>Configure uma nova rota</small></button>
          </div>
        )}
      </section>
    </>
  );
}

function Summary({ icon, label, value, detail }: { icon: React.ReactNode; label: string; value: number; detail: string }) {
  return <div className="summary-item"><span className="summary-icon">{icon}</span><div><small>{label}</small><strong>{value}</strong><span>{detail}</span></div></div>;
}

function ConnectionCard({ connection, providerName, providerLabel, models, defaultModel, onClick }: { connection: Connection; providerName?: string; providerLabel?: string; models: number; defaultModel?: Model; onClick: () => void }) {
  return (
    <button className="connection-card" onClick={onClick}>
      <div className="card-top"><ProviderMark provider={providerName} /><span className={`status-chip ${connection.is_enabled ? "online" : "offline"}`}><i />{connection.is_enabled ? "Ativa" : "Inativa"}</span></div>
      <div className="card-title"><h3>{connection.name}</h3>{connection.is_default === 1 && <span className="default-badge"><Sparkles size={12} />Padrão</span>}</div>
      <p>{providerLabel || "Provider"}</p>
      <div className="endpoint"><Server size={14} /><span>{connection.base_url}</span></div>
      <div className="card-meta"><span><Cpu size={15} />{models} {models === 1 ? "modelo" : "modelos"}</span>{defaultModel && <span className="model-default"><Check size={14} />{defaultModel.display_name}</span>}</div>
      <div className="card-link">Ver detalhes <ArrowRight size={16} /></div>
    </button>
  );
}

function DashboardSkeleton() {
  return <div className="dashboard-skeleton"><div /><div /><div /><span /><span /><span /></div>;
}
