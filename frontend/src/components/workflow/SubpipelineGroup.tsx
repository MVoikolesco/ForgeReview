import {
  Handle,
  type Node,
  NodeResizer,
  type NodeProps,
  Position,
  useReactFlow,
} from "@xyflow/react";
import { Focus, Layers3, Pencil, Trash2 } from "lucide-react";
import type { CardData } from "../../lib/types";
import {
  subpipelineInstances,
  subpipelinePorts,
} from "../../lib/workflow";
import styles from "./SubpipelineGroup.module.scss";
import { useWorkflowCardActions } from "./WorkflowCardActionsContext";

export function SubpipelineGroup({ data, id, selected }: NodeProps) {
  const group = data as CardData;
  const flow = useReactFlow<Node<CardData>>();
  const actions = useWorkflowCardActions();
  const inputs = subpipelinePorts(group.config, "input_ports");
  const outputs = subpipelinePorts(group.config, "output_ports");
  const instances = subpipelineInstances(group.config).filter(
    (instance) => instance.enabled,
  );
  const members = flow.getNodes().filter((node) => node.parentId === id).length;
  const focusGroup = () => {
    const groupNode = flow.getNode(id);
    if (!groupNode) return;
    const width =
      groupNode.measured?.width ??
      groupNode.width ??
      groupNode.initialWidth ??
      900;
    const height =
      groupNode.measured?.height ??
      groupNode.height ??
      groupNode.initialHeight ??
      190;
    flow.setCenter(
      groupNode.position.x + width / 2,
      groupNode.position.y + height / 2,
      { zoom: 1.45, duration: 260 },
    );
  };

  return (
    <section
      className={`${styles.group} ${styles[group.status] ?? ""}`}
      aria-label={`Subpipeline ${group.name}`}
    >
      <NodeResizer
        isVisible={Boolean(selected) && !actions?.readOnly}
        minWidth={420}
        minHeight={180}
        maxWidth={5000}
        maxHeight={5000}
        color="#79a9f8"
        lineClassName={styles.resizeLine}
        handleClassName={styles.resizeHandle}
      />
      <header
        className="drag-handle"
        onDoubleClickCapture={(event) => {
          event.stopPropagation();
          focusGroup();
        }}
      >
        <span>
          <Layers3 size={13} aria-hidden="true" />
          {group.name}
        </span>
        <small>
          {members} card{members === 1 ? "" : "s"}
          {instances.length > 0 &&
            ` · ${instances.length} execuç${instances.length === 1 ? "ão" : "ões"}`}
        </small>
        <span className={`${styles.actions} nodrag nopan`}>
          <button
            type="button"
            title="Focar subpipeline"
            aria-label={`Focar ${group.name}`}
            onClick={(event) => {
              event.stopPropagation();
              focusGroup();
            }}
          >
            <Focus size={12} />
          </button>
          {!actions?.readOnly && (
            <>
              <button
                type="button"
                title="Editar subpipeline"
                aria-label={`Editar ${group.name}`}
                onClick={(event) => {
                  event.stopPropagation();
                  actions?.onEdit(id);
                }}
              >
                <Pencil size={12} />
              </button>
              <button
                type="button"
                title="Excluir subpipeline"
                aria-label={`Excluir ${group.name}`}
                onClick={(event) => {
                  event.stopPropagation();
                  actions?.onDelete(id);
                }}
              >
                <Trash2 size={12} />
              </button>
            </>
          )}
        </span>
      </header>

      <div className={styles.boundaryInputs}>
        {inputs.map((port, index) => {
          const top = 72 + index * 30;
          return (
            <div className={styles.boundaryPort} key={port.key} style={{ top }}>
              <Handle
                type="target"
                position={Position.Left}
                id={`in-entry:${port.key}`}
                title={`Entrada externa: ${port.label}`}
              />
              <Handle
                type="source"
                position={Position.Left}
                id={`out-entry:${port.key}`}
                className={styles.internalHandle}
                title={`Entrada interna: ${port.label}`}
              />
              <span>{port.label}</span>
              <small>{port.contract}</small>
            </div>
          );
        })}
      </div>

      <div className={styles.boundaryOutputs}>
        {outputs.map((port, index) => {
          const top = 72 + index * 30;
          return (
            <div
              className={`${styles.boundaryPort} ${styles.output}`}
              key={port.key}
              style={{ top }}
            >
              <small>{port.contract}</small>
              <span>{port.label}</span>
              <Handle
                type="target"
                position={Position.Right}
                id={`in-exit:${port.key}`}
                className={styles.internalHandle}
                title={`Saída interna: ${port.label}`}
              />
              <Handle
                type="source"
                position={Position.Right}
                id={`out-exit:${port.key}`}
                title={`Saída externa: ${port.label}`}
              />
            </div>
          );
        })}
      </div>
      <p className={styles.hint}>
        Arraste cards para dentro · duplo clique para focar · selecione para
        redimensionar
      </p>
    </section>
  );
}
