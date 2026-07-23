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
import { useCallback, useMemo } from "react";
import styles from "./WorkflowCanvas.module.scss";
import { WorkflowCard } from "./WorkflowCard";
import { WorkflowCardActionsProvider } from "./WorkflowCardActionsContext";

const nodeTypes = { card: WorkflowCard };

type WorkflowCanvasProps = {
  nodes: Node<CardData>[];
  edges: Edge[];
  message: string;
  onNodesChange: OnNodesChange<Node<CardData>>;
  onEdgesChange: OnEdgesChange;
  onConnect: (connection: Connection) => void;
  onSelectionChange: (nodes: Node<CardData>[], edges: Edge[]) => void;
  onRequestDelete: () => void;
  onEditNode: (nodeID: string) => void;
  onDeleteNode: (nodeID: string) => void;
  onPaneClick: () => void;
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
  onSelectionChange,
  onRequestDelete,
  onEditNode,
  onDeleteNode,
  onPaneClick,
  validationIssues,
  onSelectValidationIssue,
  readOnly = false,
}: WorkflowCanvasProps) {
  const visibleEdges = useMemo(
    () => edges.map((edge) =>
      edge.sourceHandle === "out-error"
        ? {
            ...edge,
            animated: true,
            label: "erro",
            style: { stroke: "#e87b91", strokeDasharray: "5 4" },
            labelStyle: { fill: "#e87b91", fontSize: 10 },
          }
        : edge,
    ),
    [edges],
  );
  const handleSelectionChange = useCallback(
    ({ nodes: selectedNodes, edges: selectedEdges }: { nodes: Node[]; edges: Edge[] }) =>
      onSelectionChange(selectedNodes as Node<CardData>[], selectedEdges),
    [onSelectionChange],
  );
  const cardActions = useMemo(
    () => ({ readOnly, onEdit: onEditNode, onDelete: onDeleteNode }),
    [onDeleteNode, onEditNode, readOnly],
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
      <WorkflowCardActionsProvider value={cardActions}>
        <ReactFlow
          nodes={nodes}
          edges={visibleEdges}
          nodeTypes={nodeTypes}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onPaneClick={onPaneClick}
          onSelectionChange={handleSelectionChange}
          nodesDraggable={!readOnly}
          nodesConnectable={!readOnly}
          deleteKeyCode={null}
          fitView
        >
          <Background gap={18} size={1} />
          <MiniMap />
          <Controls />
        </ReactFlow>
      </WorkflowCardActionsProvider>
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
