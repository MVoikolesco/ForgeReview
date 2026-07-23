import { Handle, Position, type NodeProps } from "@xyflow/react";
import { Pencil, Trash2 } from "lucide-react";
import type { CardData } from "../../lib/types";
import { cardOutputPorts, categoryAccent } from "../../lib/workflow";
import { BaseCardShell } from "./BaseCardShell";
import { useWorkflowCardActions } from "./WorkflowCardActionsContext";
import styles from "./WorkflowCard.module.scss";

export function WorkflowCard({ id, data }: NodeProps) {
  const card = data as CardData;
  const actions = useWorkflowCardActions();
  return (
    <BaseCardShell
      accent={categoryAccent(card.category)}
      status={card.status}
      category={card.category}
      actions={actions && !actions.readOnly && (
        <span className={`${styles.actions} nodrag nopan`}>
          <button
            type="button"
            aria-label={`Editar ${card.name}`}
            title="Editar card"
            onClick={(event) => {
              event.stopPropagation();
              actions?.onEdit(id);
            }}
          >
            <Pencil size={12} aria-hidden="true" />
          </button>
          <button
            type="button"
            aria-label={`Excluir ${card.name}`}
            title="Excluir card"
            onClick={(event) => {
              event.stopPropagation();
              actions?.onDelete(id);
            }}
          >
            <Trash2 size={12} aria-hidden="true" />
          </button>
        </span>
      )}
    >
      <h2 className={styles.title}>{card.name}</h2>
      {card.inputs.map((port) => (
        <div className={styles.port} key={port.key}>
          <Handle
            type="target"
            position={Position.Left}
            id={`in-${port.key}`}
          />
          <span>{port.label}</span>
          <small>{port.contract}</small>
        </div>
      ))}
      {cardOutputPorts(card).map((port) => (
        <div className={`${styles.port} ${styles.output} ${port.key === "error" ? styles.error : ""}`} key={port.key}>
          <small>{port.contract}</small>
          <span>{port.label}</span>
          <Handle
            type="source"
            position={Position.Right}
            id={`out-${port.key}`}
          />
        </div>
      ))}
    </BaseCardShell>
  );
}
