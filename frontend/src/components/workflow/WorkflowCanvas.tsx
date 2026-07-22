"use client";

import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type Connection,
  type Edge,
  type Node,
  type OnEdgesChange,
  type OnNodesChange,
} from "@xyflow/react";
import type { WorkflowValidationIssue } from "../../lib/workflow";
import "@xyflow/react/dist/style.css";
import type { CardData } from "../../lib/types";
import styles from "./WorkflowCanvas.module.scss";
import { WorkflowCard } from "./WorkflowCard";

const nodeTypes = { card: WorkflowCard };

type WorkflowCanvasProps = {
  nodes: Node<CardData>[];
  edges: Edge[];
  message: string;
  onNodesChange: OnNodesChange<Node<CardData>>;
  onEdgesChange: OnEdgesChange;
  onConnect: (connection: Connection) => void;
  onSelect: (data: CardData) => void;
  onSelectionChange: (nodes: Node<CardData>[], edges: Edge[]) => void;
  onRequestDelete: () => void;
  validationIssues: WorkflowValidationIssue[];
  onSelectValidationIssue: (issue: WorkflowValidationIssue) => void;
  readOnly?: boolean;
};

export function WorkflowCanvas({
  nodes,
  edges,
  message,
  onNodesChange,
  onEdgesChange,
  onConnect,
  onSelect,
  onSelectionChange,
  onRequestDelete,
  validationIssues,
  onSelectValidationIssue,
  readOnly = false,
}: WorkflowCanvasProps) {
  const visibleEdges = edges.map((edge) =>
    edge.sourceHandle === "out-error"
      ? {
          ...edge,
          animated: true,
          label: "erro",
          style: { stroke: "#e87b91", strokeDasharray: "5 4" },
          labelStyle: { fill: "#e87b91", fontSize: 10 },
        }
      : edge,
  );
  return (
    <section
      className={styles.canvas}
      aria-label="Canvas do workflow"
      onKeyDownCapture={(event) => {
        if (!readOnly && (event.key === "Delete" || event.key === "Backspace")) {
          event.preventDefault();
          event.stopPropagation();
          onRequestDelete();
        }
      }}
    >
      <ReactFlow
        nodes={nodes}
        edges={visibleEdges}
        nodeTypes={nodeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onNodeClick={(_, node) => onSelect(node.data)}
        onSelectionChange={({ nodes: selectedNodes, edges: selectedEdges }) =>
          onSelectionChange(selectedNodes as Node<CardData>[], selectedEdges)
        }
        nodesDraggable={!readOnly}
        nodesConnectable={!readOnly}
        deleteKeyCode={null}
        fitView
      >
        <Background gap={18} size={1} />
        <MiniMap />
        <Controls />
      </ReactFlow>
      <p className={styles.message} role="status">
        {message}
      </p>
      {validationIssues.length > 0 && (
        <section className={styles.validation} aria-label="Erros de validação" aria-live="polite">
          <strong>{validationIssues.length} ajuste{validationIssues.length === 1 ? "" : "s"} necessário{validationIssues.length === 1 ? "" : "s"}</strong>
          <ul>
            {validationIssues.map((issue, index) => (
              <li key={`${issue.nodeKey ?? "graph"}-${index}`}>
                <button type="button" onClick={() => onSelectValidationIssue(issue)}>
                  {issue.message}
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </section>
  );
}
