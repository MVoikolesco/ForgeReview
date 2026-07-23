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
  type ReactFlowInstance,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Crosshair, Expand, Map, Search } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CardData } from "../../lib/types";
import {
  edgeIsActivelyPropagating,
  type WorkflowValidationIssue,
} from "../../lib/workflow";
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
  onAddCardAt?: (key: string, position: { x: number; y: number }) => void;
  onDuplicateNode?: (nodeID: string) => void;
  onDeleteEdge?: (edgeID: string) => void;
  onResetView?: () => void;
  searchQuery?: string;
  onSearchQueryChange?: (query: string) => void;
  focusNodeID?: string;
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
  onAddCardAt,
  onDuplicateNode,
  onDeleteEdge,
  onResetView,
  searchQuery = "",
  onSearchQueryChange,
  focusNodeID,
  readOnly = false,
}: WorkflowCanvasProps) {
  const [flow, setFlow] = useState<ReactFlowInstance<Node<CardData>, Edge>>();
  const [showMap, setShowMap] = useState(true);
  const [menu, setMenu] = useState<{
    x: number;
    y: number;
    nodeID?: string;
    edgeID?: string;
  }>();
  const menuRef = useRef<HTMLDivElement>(null);
  const visibleEdges = useMemo(
    () =>
      edges.map((edge) => {
        const animated = edgeIsActivelyPropagating(edge, nodes);
        return edge.sourceHandle === "out-error"
          ? {
              ...edge,
              animated,
              label: "erro",
              style: { stroke: "#e87b91", strokeDasharray: "5 4" },
              labelStyle: { fill: "#e87b91", fontSize: 10 },
            }
          : { ...edge, animated };
      }),
    [edges, nodes],
  );
  const handleSelectionChange = useCallback(
    ({
      nodes: selectedNodes,
      edges: selectedEdges,
    }: {
      nodes: Node[];
      edges: Edge[];
    }) => onSelectionChange(selectedNodes as Node<CardData>[], selectedEdges),
    [onSelectionChange],
  );
  const cardActions = useMemo(
    () => ({ readOnly, onEdit: onEditNode, onDelete: onDeleteNode }),
    [onDeleteNode, onEditNode, readOnly],
  );
  const focusNode = useCallback(
    (nodeID = focusNodeID) => {
      const node = nodes.find((item) => item.id === nodeID);
      if (node && flow)
        flow.setCenter(node.position.x + 115, node.position.y + 80, {
          zoom: 1.15,
          duration: 220,
        });
    },
    [flow, focusNodeID, nodes],
  );
  const closeMenu = useCallback(() => setMenu(undefined), []);
  const openMenu = useCallback(
    (
      event: MouseEvent | React.MouseEvent,
      target: { nodeID?: string; edgeID?: string } = {},
    ) => {
      event.preventDefault();
      event.stopPropagation();
      const width = 216;
      const height = target.nodeID ? 190 : target.edgeID ? 126 : 90;
      setMenu({
        ...target,
        x: Math.max(
          12,
          Math.min(event.clientX, window.innerWidth - width - 12),
        ),
        y: Math.max(
          12,
          Math.min(event.clientY, window.innerHeight - height - 12),
        ),
      });
    },
    [],
  );
  useEffect(() => {
    if (!menu) return;
    const closeOutside = (event: PointerEvent) => {
      if (!menuRef.current?.contains(event.target as globalThis.Node))
        closeMenu();
    };
    const closeEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        closeMenu();
      }
    };
    window.addEventListener("pointerdown", closeOutside);
    window.addEventListener("keydown", closeEscape);
    return () => {
      window.removeEventListener("pointerdown", closeOutside);
      window.removeEventListener("keydown", closeEscape);
    };
  }, [closeMenu, menu]);
  return (
    <section
      className={styles.canvas}
      aria-label="Canvas do workflow"
      onKeyDownCapture={(event) => {
        if (
          !readOnly &&
          (event.key === "Delete" || event.key === "Backspace")
        ) {
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
          onPaneClick={() => {
            closeMenu();
            onPaneClick();
          }}
          onInit={setFlow}
          onNodeContextMenu={(event, node) =>
            openMenu(event, { nodeID: node.id })
          }
          onEdgeContextMenu={(event, edge) =>
            openMenu(event, { edgeID: edge.id })
          }
          onPaneContextMenu={(event) => openMenu(event)}
          onDrop={(event) => {
            event.preventDefault();
            const key = event.dataTransfer.getData(
              "application/forgereview-card",
            );
            if (key && flow)
              onAddCardAt?.(
                key,
                flow.screenToFlowPosition({
                  x: event.clientX,
                  y: event.clientY,
                }),
              );
          }}
          onDragOver={(event) => event.preventDefault()}
          onSelectionChange={handleSelectionChange}
          nodesDraggable={!readOnly}
          nodesConnectable={!readOnly}
          deleteKeyCode={null}
          fitView
        >
          <Background gap={18} size={1} />
          {showMap && (
            <MiniMap
              nodeColor={(node) =>
                node.data.status === "failed"
                  ? "#df6575"
                  : node.data.status === "completed"
                    ? "#4e9f78"
                    : "#4b8cff"
              }
              pannable
              zoomable
            />
          )}
          <Controls />
        </ReactFlow>
      </WorkflowCardActionsProvider>
      <div className={styles.toolbar} aria-label="Ferramentas do canvas">
        <label>
          <Search size={14} />
          <span className="sr-only">Buscar card</span>
          <input
            data-canvas-search
            value={searchQuery}
            onChange={(event) => onSearchQueryChange?.(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Escape") {
                event.preventDefault();
                onSearchQueryChange?.("");
                event.currentTarget.blur();
              }
              if (event.key === "Enter" && focusNodeID) {
                event.preventDefault();
                focusNode(focusNodeID);
                onSearchQueryChange?.("");
                event.currentTarget.blur();
              }
            }}
            placeholder="Buscar no canvas"
          />
        </label>
        <button type="button" onClick={() => focusNode()} title="Focar seleção">
          <Crosshair size={15} />
        </button>
        <button
          type="button"
          onClick={() => flow?.fitView({ duration: 220, padding: 0.2 })}
          title="Enquadrar canvas"
        >
          <Expand size={15} />
        </button>
        <button
          type="button"
          onClick={() => setShowMap((visible) => !visible)}
          aria-pressed={showMap}
          title="Alternar minimapa"
        >
          <Map size={15} />
        </button>
      </div>
      {menu && (
        <div
          ref={menuRef}
          className={styles.contextMenu}
          style={{ left: menu.x, top: menu.y }}
          role="menu"
          aria-label="Ações do canvas"
        >
          {menu.nodeID && (
            <>
              <button
                type="button"
                role="menuitem"
                disabled={readOnly}
                onClick={() => {
                  onEditNode(menu.nodeID!);
                  closeMenu();
                }}
              >
                Editar card
              </button>
              <button
                type="button"
                role="menuitem"
                disabled={readOnly}
                onClick={() => {
                  onDuplicateNode?.(menu.nodeID!);
                  closeMenu();
                }}
              >
                Duplicar card
              </button>
              <button
                type="button"
                role="menuitem"
                disabled={readOnly}
                onClick={() => {
                  onDeleteNode(menu.nodeID!);
                  closeMenu();
                }}
              >
                Excluir card
              </button>
              <hr role="separator" />
            </>
          )}
          {menu.edgeID && (
            <>
              <button
                type="button"
                role="menuitem"
                disabled={readOnly}
                onClick={() => {
                  onDeleteEdge?.(menu.edgeID!);
                  closeMenu();
                }}
              >
                Excluir conexão
              </button>
              <hr role="separator" />
            </>
          )}
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              flow?.fitView({ duration: 220, padding: 0.2 });
              closeMenu();
            }}
          >
            Enquadrar tudo
          </button>
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              onResetView?.();
              flow?.setViewport({ x: 0, y: 0, zoom: 1 }, { duration: 220 });
              closeMenu();
            }}
          >
            Redefinir visualização
          </button>
        </div>
      )}
      <p style={{ display: "none" }} className={styles.message} role="status">
        {message}
      </p>
      {validationIssues.length > 0 && (
        <section
          className={styles.validation}
          aria-label="Erros de validação"
          aria-live="polite"
        >
          <strong>
            {validationIssues.length} ajuste
            {validationIssues.length === 1 ? "" : "s"} necessário
            {validationIssues.length === 1 ? "" : "s"}
          </strong>
          <ul>
            {validationIssues.map((issue, index) => (
              <li key={`${issue.nodeKey ?? "graph"}-${index}`}>
                <button
                  type="button"
                  onClick={() => onSelectValidationIssue(issue)}
                >
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
