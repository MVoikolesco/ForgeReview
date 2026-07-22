"use client";

import {
  addEdge,
  useEdgesState,
  useNodesState,
  type Connection,
  type Edge,
  type Node,
} from "@xyflow/react";
import {
  BookOpen,
  Braces,
  CirclePlay,
  Copy,
  Download,
  Save,
  Settings2,
  ShieldCheck,
  Trash2,
  Upload,
} from "lucide-react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  executeWorkflow,
  getCards,
  getExecution,
  getIntegrations,
  getModelProfiles,
  getWorkflows,
  getWorkflowVersion,
  publishWorkflow,
  saveWorkflow,
} from "../../lib/api";
import type {
  CardData,
  CardType,
  ExecutionReport,
  Integration,
  ModelProfile,
  WorkflowMetadata,
} from "../../lib/types";
import {
  canConnect,
  cardOutputPorts,
  defaultWorkflowMetadata,
  hasErrorRoute,
  hydrateDefinition,
  localCards,
  removeSelectedElements,
  reviewTemplate,
  selectionHasChanged,
  starterEdges,
  starterNodes,
  toDefinition,
  validateStudioWorkflow,
  type WorkflowValidationIssue,
} from "../../lib/workflow";
import { ModalShell } from "../common/ModalShell";
import { ConnectionWizard } from "../integrations/ConnectionWizard";
import { WorkflowTransferModal } from "./WorkflowTransferModal";
import { CardInspector } from "../workflow/CardInspector";
import { CardLibrary } from "../workflow/CardLibrary";
import { WorkflowCanvas } from "../workflow/WorkflowCanvas";
import styles from "./StudioWorkspace.module.scss";
import { useCurrentUser } from "../auth/AuthGate";

export function StudioWorkspace() {
	const user = useCurrentUser();
	const canEdit = user?.role === "editor" || user?.role === "admin";
  const searchParams = useSearchParams();
  const versionParam = searchParams.get("version");
  const versionID = versionParam && /^\d+$/.test(versionParam) ? Number(versionParam) : undefined;
  const [nodes, setNodes, onNodesChange] =
    useNodesState<Node<CardData>>(starterNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>(starterEdges);
  const [cards, setCards] = useState<CardType[]>(localCards);
  const [selected, setSelected] = useState<CardData>(starterNodes[0].data);
  const [message, setMessage] = useState("Fluxo local pronto para validar.");
  const [busy, setBusy] = useState(false);
  const [integrations, setIntegrations] = useState<Integration[]>([]);
  const [modelProfiles, setModelProfiles] = useState<ModelProfile[]>([]);
  const [showConnections, setShowConnections] = useState(false);
  const [metadata, setMetadata] = useState<WorkflowMetadata>(defaultWorkflowMetadata);
  const [openedVersionID, setOpenedVersionID] = useState<number>();
  const [dirty, setDirty] = useState(false);
   const [transfer, setTransfer] = useState<"clone" | "import" | "export">();
   const [workflowKeys, setWorkflowKeys] = useState<string[]>([]);
   const [selectedNodeIDs, setSelectedNodeIDs] = useState<string[]>([]);
   const [selectedEdgeIDs, setSelectedEdgeIDs] = useState<string[]>([]);
   const [confirmRemoval, setConfirmRemoval] = useState(false);
   const definition = useMemo(() => toDefinition(nodes, edges, metadata), [nodes, edges, metadata]);
   const validationIssues = useMemo(
     () => validateStudioWorkflow(definition, cards),
     [definition, cards],
   );

  const loadIntegrations = async () => {
    try {
      const [items, profiles] = await Promise.all([getIntegrations(), getModelProfiles()]);
      setIntegrations(items);
      setModelProfiles(profiles);
    } catch {
      setMessage("Não foi possível carregar as integrações.");
    }
  };
  useEffect(() => {
    if (!versionID) {
      if (versionParam) setMessage("A versão solicitada é inválida.");
      void getCards()
        .then(setCards)
        .catch(() =>
          setMessage("Backend indisponível. Biblioteca local exibida."),
        );
      return;
    }
    setBusy(true);
    setMessage(`Abrindo versão salva ${versionID}...`);
    void Promise.all([getCards(), getWorkflowVersion(versionID)])
      .then(([catalog, definition]) => {
        const hydrated = hydrateDefinition(definition, catalog);
        setCards(catalog);
        setNodes(hydrated.nodes);
        setEdges(hydrated.edges);
        setSelected(hydrated.nodes[0]?.data ?? starterNodes[0].data);
        setMetadata({
          key: definition.key,
          name: definition.name,
          description: definition.description,
        });
        setOpenedVersionID(versionID);
        setDirty(false);
        setMessage(
          `Versão salva ${versionID} aberta. Alterações serão salvas como um novo rascunho; a versão original permanece imutável.`,
        );
      })
      .catch((error) =>
        setMessage(
          error instanceof Error
            ? error.message
            : "Não foi possível abrir a versão salva.",
        ),
      )
      .finally(() => setBusy(false));
  }, [versionID, versionParam, setEdges, setNodes]);
  useEffect(() => {
    void Promise.all([getIntegrations(), getModelProfiles()])
      .then(([items, profiles]) => {
        setIntegrations(items);
        setModelProfiles(profiles);
      })
      .catch(() => undefined);
  }, []);

  const addCard = (card: CardType) => {
    const key = `${card.key}-${Date.now().toString(36)}`;
    const data: CardData = {
      key,
      type: card.key,
      name: card.name,
      category: card.category,
      inputs: card.inputs,
      outputs: card.outputs,
      errorOutput: card.error_output,
      config: card.key === "template" ? { template: "Defina o prompt" } : {},
      status: "idle",
    };
    setNodes((all) => [
      ...all,
      {
        id: key,
        type: "card",
        position: {
          x: 210 + (all.length % 4) * 280,
          y: 90 + Math.floor(all.length / 4) * 250,
        },
        data,
      },
    ]);
    setSelected(data);
    setDirty(true);
  };
  const loadReviewTemplate = () => {
    const template = reviewTemplate(cards);
    if (!template) {
      setMessage(
        "O catálogo ainda está sendo carregado. Tente novamente em instantes.",
      );
      return;
    }
    setNodes(template.nodes);
    setEdges(template.edges);
    setSelected(template.nodes[0].data);
    setDirty(true);
    setMessage(
      "Template de review carregado. Configure Gitea, modelo e dados do PR nos cards selecionados.",
    );
  };
  const connect = useCallback((connection: Connection) => {
    const source = nodes.find((node) => node.id === connection.source);
    const target = nodes.find((node) => node.id === connection.target);
    const sourcePort = source && cardOutputPorts(source.data).find(
      (port) => `out-${port.key}` === connection.sourceHandle,
    );
    const targetPort = target?.data.inputs.find(
      (port) => `in-${port.key}` === connection.targetHandle,
    );
    if (!canConnect(sourcePort, targetPort)) {
      setMessage(
        "Conexão bloqueada: as portas precisam aceitar o mesmo contrato.",
      );
      return;
    }
    setEdges((all) => addEdge({ ...connection, animated: true }, all));
    setDirty(true);
  }, [nodes, setEdges]);
  const patchSelected = (patch: Partial<CardData>) => {
    setSelected((current) => ({ ...current, ...patch }));
    setNodes((all) =>
      all.map((node) =>
        node.id === selected.key
          ? { ...node, data: { ...node.data, ...patch } }
          : node,
      ),
    );
    setDirty(true);
  };
  const applyReport = (report: ExecutionReport) =>
    setNodes((all) =>
      all.map((node) => ({
        ...node,
        data: {
          ...node.data,
          status:
            report.runs.find((run) => run.node_key === node.id)?.status ||
            "idle",
        },
      })),
    );
  const saveDraft = async () => {
    if (validationIssues.length) {
      setMessage("Corrija os ajustes indicados antes de salvar o rascunho.");
      return;
    }
    setBusy(true);
    setMessage("Salvando rascunho do workflow...");
    try {
      const saved = await saveWorkflow(definition);
      setDirty(false);
      setMessage(
        `Novo rascunho salvo como versão ${saved.version_id}${openedVersionID ? ` a partir da versão ${openedVersionID}` : ""}.`,
      );
    } catch (error) {
      setMessage(
        error instanceof Error ? error.message : "Não foi possível salvar o rascunho.",
      );
    } finally {
      setBusy(false);
    }
  };
  const publishCurrent = async () => {
    if (validationIssues.length) {
      setMessage("Corrija os ajustes indicados antes de publicar.");
      return;
    }
    setBusy(true);
    setMessage("Salvando rascunho para publicação...");
    try {
      const saved = await saveWorkflow(definition);
      setDirty(false);
      setMessage("Validando e publicando a versão salva...");
      const published = await publishWorkflow(saved.version_id);
      setMessage(`Versão ${published.version} publicada.`);
    } catch (error) {
      setMessage(
        error instanceof Error
          ? error.message
          : "Não foi possível publicar o workflow.",
      );
    } finally {
      setBusy(false);
    }
  };
  const saveAndRun = async () => {
    if (validationIssues.length) {
      setMessage("Corrija os ajustes indicados antes de executar.");
      return;
    }
    setBusy(true);
    setMessage("Salvando versão do workflow...");
    try {
      const saved = await saveWorkflow(definition);
      setDirty(false);
      setMessage("Executando versão salva...");
      setNodes((all) =>
        all.map((node) => ({
          ...node,
          data: { ...node.data, status: "running" },
        })),
      );
      const started = await executeWorkflow(saved.version_id);
      let report = started.report;
      if (started.status === "queued" && started.execution_id)
        for (let attempt = 0; attempt < 20; attempt += 1) {
          await new Promise((resolve) => setTimeout(resolve, 500));
          try {
            const pending = await getExecution(started.execution_id);
            if (pending.status !== "queued" && pending.status !== "running") {
              report = pending;
              break;
            }
          } catch {
            /* continue polling */
          }
        }
      if (report) applyReport(report);
      setMessage(
        report
          ? `Execução ${started.execution_id} ${report.status === "completed" ? "concluída" : report.status}.`
          : `Execução ${started.execution_id} enviada à fila.`,
      );
    } catch (error) {
      setNodes((all) =>
        all.map((node) =>
          node.data.status === "running"
            ? { ...node, data: { ...node.data, status: "failed" } }
            : node,
        ),
      );
      setMessage(error instanceof Error ? error.message : "Erro inesperado.");
    } finally {
      setBusy(false);
    }
  };
  const openConnections = () => {
    setShowConnections(true);
    void loadIntegrations();
  };
  const openTransfer = (mode: "clone" | "import" | "export") => {
    if (mode !== "clone") return setTransfer(mode);
    void getWorkflows()
      .then((workflows) => {
        setWorkflowKeys(workflows.map((workflow) => workflow.key));
        setTransfer(mode);
      })
      .catch(() => setMessage("Não foi possível verificar os nomes existentes para criar a cópia."));
  };
  const applyTransferredDefinition = (definition: import("../../lib/types").WorkflowDefinition, nextMessage: string) => {
    const hydrated = hydrateDefinition(definition, cards);
    setNodes(hydrated.nodes);
    setEdges(hydrated.edges);
    setSelected(hydrated.nodes[0]?.data ?? starterNodes[0].data);
    setMetadata({ key: definition.key, name: definition.name, description: definition.description });
    setOpenedVersionID(undefined);
    setDirty(true);
    setMessage(nextMessage);
    setTransfer(undefined);
  };
  const trackNodeChanges = useCallback((...args: Parameters<typeof onNodesChange>) => {
    if (!canEdit) return;
    if (
      args[0].some((change) =>
        ["add", "remove", "replace", "position"].includes(change.type),
      )
    )
      setDirty(true);
    onNodesChange(...args);
  }, [canEdit, onNodesChange]);
  const trackEdgeChanges = useCallback((...args: Parameters<typeof onEdgesChange>) => {
    if (!canEdit) return;
    if (
      args[0].some((change) =>
        ["add", "remove", "replace"].includes(change.type),
      )
    )
      setDirty(true);
    onEdgesChange(...args);
  }, [canEdit, onEdgesChange]);
  const requestRemoval = useCallback(() => {
    if (!canEdit) return;
    if (!selectedNodeIDs.length && !selectedEdgeIDs.length) {
      setMessage("Selecione um ou mais cards ou conexões para remover.");
      return;
    }
    setConfirmRemoval(true);
  }, [canEdit, selectedEdgeIDs.length, selectedNodeIDs.length]);
  const handleSelectionChange = useCallback((selectedNodes: Node<CardData>[], selectedEdges: Edge[]) => {
    const nextNodeIDs = selectedNodes.map((node) => node.id);
    const nextEdgeIDs = selectedEdges.map((edge) => edge.id);
    setSelectedNodeIDs((current) =>
      selectionHasChanged(current, nextNodeIDs) ? nextNodeIDs : current,
    );
    setSelectedEdgeIDs((current) =>
      selectionHasChanged(current, nextEdgeIDs) ? nextEdgeIDs : current,
    );
    if (selectedNodes[0]) {
      setSelected((current) =>
        current.key === selectedNodes[0].data.key ? current : selectedNodes[0].data,
      );
    }
  }, []);
  const removeSelection = () => {
    const removed = removeSelectedElements(nodes, edges, selectedNodeIDs, selectedEdgeIDs);
    setNodes(removed.nodes);
    setEdges(removed.edges);
    setSelected(removed.nodes[0]?.data ?? starterNodes[0].data);
    setSelectedNodeIDs([]);
    setSelectedEdgeIDs([]);
    setConfirmRemoval(false);
    setDirty(true);
    setMessage(`${selectedNodeIDs.length} card(s) e ${removed.removedEdges} conexão(ões) removidos.`);
  };
  const selectValidationIssue = (issue: WorkflowValidationIssue) => {
    const node = issue.nodeKey && nodes.find((item) => item.id === issue.nodeKey);
    if (node) {
      setSelected(node.data);
      setSelectedNodeIDs([node.id]);
      setSelectedEdgeIDs([]);
      setMessage(`Ajuste destacado: ${issue.message}`);
    } else {
      setMessage(issue.message);
    }
  };
  useEffect(() => {
    if (!dirty) return;
    const warnBeforeExit = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warnBeforeExit);
    return () => window.removeEventListener("beforeunload", warnBeforeExit);
  }, [dirty]);
  return (
    <main className={styles.studio}>
      <header className={styles.topbar}>
        <strong>
          <Braces size={20} /> ForgeReview <small>WORKFLOW STUDIO</small>
        </strong>
         <span>{openedVersionID ? `Versão salva ${openedVersionID}` : "Fluxo de verificação"}</span>
         {dirty && <em className={styles.unsaved}>Alterações não salvas</em>}
         <em className={validationIssues.length ? styles.invalid : styles.valid}>
           {validationIssues.length ? `${validationIssues.length} ajuste(s) pendente(s)` : "Validação local pronta"}
         </em>
        <div>
          <button disabled={!canEdit} onClick={loadReviewTemplate}>
            <BookOpen size={14} /> Template review
          </button>
          <button disabled={user?.role !== "admin"} onClick={openConnections}>
            <Settings2 size={14} /> Integrações
          </button>
          <Link href="/pipelines">Pipelines</Link>
          <Link href="/integrations">Gerenciar integrações</Link>
          <button disabled={!canEdit || busy} onClick={() => openTransfer("clone")}><Copy size={14} /> Clonar</button>
          <button disabled={!canEdit || busy} onClick={() => openTransfer("import")}><Upload size={14} /> Importar</button>
          <button disabled={!canEdit || busy} onClick={() => openTransfer("export")}><Download size={14} /> Exportar</button>
           <button
             disabled={busy || !canEdit}
             onClick={() => setMessage(validationIssues.length ? "Há ajustes locais pendentes no workflow." : "Validação local concluída. O backend confirmará ao salvar.")}
           >
             <ShieldCheck size={14} /> Validar{validationIssues.length ? ` (${validationIssues.length})` : ""}
           </button>
           <button disabled={busy || !canEdit} onClick={requestRemoval}>
             <Trash2 size={14} /> Remover seleção
           </button>
          <button
            disabled={busy || !canEdit}
            onClick={() => void saveDraft()}
          >
            <Save size={14} /> Salvar novo rascunho
          </button>
          <button
            className={styles.publish}
            disabled={busy || !canEdit}
            onClick={() => void publishCurrent()}
          >
            <Upload size={14} /> Publicar
          </button>
          <button
            className={styles.primary}
            disabled={busy || !canEdit}
            onClick={() => void saveAndRun()}
          >
            <CirclePlay size={14} /> {busy ? "Executando" : "Salvar e executar"}
          </button>
        </div>
      </header>
      <CardLibrary cards={cards} onAdd={addCard} readOnly={!canEdit} />
      <WorkflowCanvas
        nodes={nodes}
        edges={edges}
        message={message}
        onNodesChange={trackNodeChanges}
        onEdgesChange={trackEdgeChanges}
        onConnect={connect}
        onSelect={setSelected}
        onSelectionChange={handleSelectionChange}
        onRequestDelete={requestRemoval}
        validationIssues={validationIssues}
        onSelectValidationIssue={selectValidationIssue}
        readOnly={!canEdit}
      />
      <CardInspector
        selected={selected}
        integrations={integrations}
        modelProfiles={modelProfiles}
        hasErrorRoute={hasErrorRoute(edges, selected.key)}
        onChange={(patch) => canEdit && patchSelected(patch)}
        readOnly={!canEdit}
      />
      {showConnections && (
        <ConnectionWizard
          items={integrations}
          onClose={() => setShowConnections(false)}
          onCreated={(item) => {
            setIntegrations((all) => [...all, item]);
            void loadIntegrations();
            setMessage(`Integração ${item.name} cadastrada.`);
          }}
        />
      )}
      {transfer && <WorkflowTransferModal mode={transfer} definition={definition} cards={cards} workflowKeys={workflowKeys} onClose={() => setTransfer(undefined)} onApply={applyTransferredDefinition} />}
      {confirmRemoval && (
        <ModalShell
          title="Remover seleção?"
          eyebrow="AÇÃO DESTRUTIVA"
          description={`${selectedNodeIDs.length} card(s) e ${selectedEdgeIDs.length} conexão(ões) estão selecionados. Cards removidos também removem todas as conexões ligadas.`}
          onClose={() => setConfirmRemoval(false)}
          footer={<><button type="button" onClick={() => setConfirmRemoval(false)}>Cancelar</button><button type="button" className={styles.removeConfirm} onClick={removeSelection}>Remover</button></>}
        >
          <p>Revise a seleção no canvas. Esta ação só acontece depois desta confirmação e marca o workflow como não salvo.</p>
        </ModalShell>
      )}
    </main>
  );
}
