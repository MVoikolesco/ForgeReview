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
    <section className={styles.canvas} aria-label="Canvas do workflow">
      <ReactFlow
        nodes={nodes}
        edges={visibleEdges}
        nodeTypes={nodeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onNodeClick={(_, node) => onSelect(node.data)}
        nodesDraggable={!readOnly}
        nodesConnectable={!readOnly}
        edgesFocusable={!readOnly}
        fitView
      >
        <Background gap={18} size={1} />
        <MiniMap />
        <Controls />
      </ReactFlow>
      <p className={styles.message} role="status">
        {message}
      </p>
    </section>
  );
}
