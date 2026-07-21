import { Handle, Position, type NodeProps } from "@xyflow/react";
import type { CardData } from "../../lib/types";
import { categoryAccent } from "../../lib/workflow";
import { BaseCardShell } from "./BaseCardShell";
import styles from "./WorkflowCard.module.scss";

export function WorkflowCard({ data }: NodeProps) {
  const card = data as CardData;
  return (
    <BaseCardShell
      accent={categoryAccent(card.category)}
      status={card.status}
      category={card.category}
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
      {card.outputs.map((port) => (
        <div className={`${styles.port} ${styles.output}`} key={port.key}>
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
