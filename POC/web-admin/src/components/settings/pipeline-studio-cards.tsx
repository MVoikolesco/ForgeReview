"use client";

import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import { AlertCircle, Braces, Hand, Trash2, Webhook } from "lucide-react";
import { useState } from "react";
import type { EntrypointNodeData, RuntimeEvent, StudioNodeData, Trigger } from "./pipeline-studio-types";
import {
  contractColor,
  handlePosition,
  portSide,
  stageProcessorLabel,
  triggerLabels,
} from "./pipeline-studio-utils";

function triggerIcon(source: Trigger["source"]) {
  if (source === "api") return <Braces size={15} />;
  if (source === "manual") return <Hand size={15} />;
  return <Webhook size={15} />;
}

function stagePercent(event?: RuntimeEvent) {
  return ["done", "concluido", "concluído", "completed"].includes(event?.status || "")
    ? 100
    : (event?.percent ?? 0);
}

function EntrypointNode({ data }: NodeProps<Node<EntrypointNodeData>>) {
  return (
    <article className={`studio-entrypoint ${data.trigger.enabled ? "enabled" : "disabled"}`}>
      <header>
        <span><i />Entrada</span>
        <small>{data.trigger.enabled ? "Ativa" : "Inativa"}</small>
      </header>
      <div className="studio-entrypoint-body">
        <span className="studio-entrypoint-icon">{triggerIcon(data.trigger.source)}</span>
        <label>
          Origem
          <select
            className="nodrag"
            disabled={!data.editable}
            value={data.trigger.source}
            onChange={(event) => data.updateSource(event.target.value as Trigger["source"])}
          >
            {Object.entries(triggerLabels).map(([value, label]) => (
              <option key={value} value={value}>{label}</option>
            ))}
          </select>
        </label>
      </div>
      <button className="nodrag studio-entrypoint-state" disabled={!data.editable} onClick={data.toggle}>
        {data.trigger.enabled ? "Desativar entrada" : "Ativar entrada"}
      </button>
      <Handle
        id="entry-output"
        type="source"
        position={Position.Right}
        className="studio-port-handle"
        style={{ background: "#e6a34e" }}
      />
    </article>
  );
}

function StudioNode({ data }: NodeProps<Node<StudioNodeData>>) {
  const stage = data.stage;
  const inputSide = portSide(stage, "input");
  const outputSide = portSide(stage, "output");
  const [renaming, setRenaming] = useState(false);
  return (
    <article className={`studio-node ${stage.executor_key === "error_log" ? "error" : ""}`} onClick={data.select}>
      <header>
        {data.editable && renaming ? (
          <input
            aria-label="Nome da etapa"
            autoFocus
            className="nodrag studio-node-name-input"
            value={stage.name}
            onBlur={() => setRenaming(false)}
            onChange={(event) => data.update({ name: event.target.value })}
            onClick={(event) => event.stopPropagation()}
            onKeyDown={(event) => {
              if (event.key === "Enter" || event.key === "Escape") {
                event.preventDefault();
                setRenaming(false);
              }
            }}
          />
        ) : (
          <button
            className="studio-node-name"
            disabled={!data.editable}
            onClick={(event) => {
              event.stopPropagation();
              if (data.editable) setRenaming(true);
            }}
            title={data.editable ? "Clique para renomear" : undefined}
            type="button"
          >
            <i />{stage.name}
          </button>
        )}
        {data.editable && data.removable && (
          <button aria-label="Remover etapa" onClick={(event) => { event.stopPropagation(); data.remove(); }}>
            <Trash2 size={13} />
          </button>
        )}
      </header>
      <div className={`studio-ports ${inputSide === "right" && outputSide === "left" ? "reversed" : ""}`}>
        <span className={`input-port ${inputSide}`}>
          <Handle id={`input-${inputSide}`} type="target" position={handlePosition(inputSide)} className="studio-port-handle" style={{ background: contractColor(stage.input_contract?.key) }} />
          <i style={{ background: contractColor(stage.input_contract?.key) }} />
          {stage.input_contract?.key || "Entrada inicial"}
        </span>
        <span className={`output-port ${outputSide}`}>
          {stage.output_contract?.key || "Terminal"}
          <i style={{ background: contractColor(stage.output_contract?.key) }} />
          <Handle id={`output-${outputSide}`} type="source" position={handlePosition(outputSide)} className="studio-port-handle" style={{ background: contractColor(stage.output_contract?.key) }} />
        </span>
      </div>
      {data.editable ? (
        <>
          <div className="studio-node-meta">
            <span>{stageProcessorLabel(stage)}</span>
            <span>{stage.required ? "Exigida ao publicar" : "Opcional"}</span>
          </div>
          <p className="studio-node-edit-hint">Clique no nome para renomear. Selecione o card para configurar.</p>
        </>
      ) : (
        <div className={`studio-run-log ${data.runtime?.status || "pending"}`}>
          <header>
            <span>{data.active ? "Em execução" : data.runtime?.status || "Aguardando"}</span>
            <span>
              <strong>{stagePercent(data.runtime)}%</strong>
              <button className="studio-log-button" onClick={(event) => { event.stopPropagation(); data.openLogs(); }} title="Abrir logs">
                <AlertCircle size={15} />
              </button>
            </span>
          </header>
          <p>{data.runtime?.message || "Aguardando uma execução desta etapa."}</p>
          <div><i style={{ width: `${stagePercent(data.runtime)}%` }} /></div>
        </div>
      )}
    </article>
  );
}

export const studioNodeTypes = { studio: StudioNode, entrypoint: EntrypointNode };
