"use client";

import { Background, Controls, ReactFlow, type Connection, type Edge, type Node, type ReactFlowInstance } from "@xyflow/react";
import { CircleAlert, MousePointer2, PanelLeftOpen } from "lucide-react";
import type { ComponentType, MouseEvent } from "react";

export function PipelineStudioCanvas({
  nodes,
  edges,
  nodeTypes,
  editable,
  flowInstance,
  onInit,
  onDropStage,
  onNodesChange,
  onEdgesChange,
  onConnect,
  onEdgeClick,
  onOpenMenu,
  statusHasError,
  statusText,
  onOpenStatus,
}: {
  nodes: Node[];
  edges: Edge[];
  nodeTypes: Record<string, ComponentType<any>>;
  editable: boolean;
  flowInstance: ReactFlowInstance | null;
  onInit: (instance: ReactFlowInstance) => void;
  onDropStage: (type: string, position?: { x: number; y: number }) => void;
  onNodesChange: (changes: Parameters<NonNullable<React.ComponentProps<typeof ReactFlow>["onNodesChange"]>>[0]) => void;
  onEdgesChange: (changes: Parameters<NonNullable<React.ComponentProps<typeof ReactFlow>["onEdgesChange"]>>[0]) => void;
  onConnect: (connection: Connection) => void;
  onEdgeClick: (_: MouseEvent, edge: Edge) => void;
  onOpenMenu: () => void;
  statusHasError: boolean;
  statusText: string;
  onOpenStatus: () => void;
}) {
  return (
    <main
      className="studio-canvas"
      onDragOver={(event) => event.preventDefault()}
      onDrop={(event) => {
        const type = event.dataTransfer.getData("stage");
        if (!type || !editable) return;
        onDropStage(type, flowInstance?.screenToFlowPosition({ x: event.clientX, y: event.clientY }));
      }}
    >
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onInit={onInit}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        nodesDraggable={editable}
        nodesConnectable={editable}
        edgesReconnectable={editable}
        elementsSelectable
        deleteKeyCode={null}
        onEdgeClick={onEdgeClick}
        fitView
      >
        <Background gap={18} size={1} />
        <Controls />
      </ReactFlow>
      <button className="studio-menu-toggle" onClick={onOpenMenu} title="Abrir menu"><PanelLeftOpen size={17} /></button>
      <div className="studio-hint"><MousePointer2 size={14} /> Configure entradas, arraste etapas e conecte portas compatíveis.</div>
      <button className={`studio-status ${statusHasError ? "error" : "ok"}`} onClick={onOpenStatus} title="Status da pipeline">
        <CircleAlert size={16} /> {statusText}
      </button>
    </main>
  );
}
