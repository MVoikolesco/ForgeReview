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
  CirclePlay,
  Copy,
  Download,
  MoreHorizontal,
  Save,
  Settings2,
  ShieldCheck,
  Trash2,
  Upload,
} from "lucide-react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  apiURL,
  executePublishedWorkflow,
  executeWorkflow,
  getCards,
  getExecution,
  getIntegrations,
  getModelProfiles,
  getPublishedWorkflow,
  getWebhookRegistrations,
  getWorkflows,
  getWorkflowVersion,
  publishWorkflow,
  saveWebhookRegistration,
  saveWorkflow,
} from "../../lib/api";
import { studioManagementActions } from "../../lib/navigation";
import {
  isEditableTarget,
  pushHistory,
  redoHistory,
  undoHistory,
  studioLoadTarget,
  type StudioHistory,
} from "../../lib/studio";
import type {
  CardData,
  CardType,
  ExecutionReport,
  Integration,
  ModelProfile,
  WebhookRegistration,
  WorkflowMetadata,
} from "../../lib/types";
import {
  applyExecutionReport,
  canConnect,
  cardOutputPorts,
  defaultWorkflowMetadata,
  hasErrorRoute,
  hydrateDefinition,
  isManualTrigger,
  localCards,
  removeSelectedElements,
  resetExecutionStatuses,
  reviewTemplate,
  selectionHasChanged,
  starterEdges,
  starterNodes,
  toDefinition,
  validateStudioWorkflow,
  type WorkflowValidationIssue,
} from "../../lib/workflow";
import { useCurrentUser } from "../auth/AuthGate";
import { ModalShell } from "../common/ModalShell";
import { ConnectionWizard } from "../integrations/ConnectionWizard";
import { AppShell } from "../shell/AppShell";
import { CardInspector } from "../workflow/CardInspector";
import { CardLibrary } from "../workflow/CardLibrary";
import { WorkflowCanvas } from "../workflow/WorkflowCanvas";
import styles from "./StudioWorkspace.module.scss";
import { WorkflowTransferModal } from "./WorkflowTransferModal";

export function StudioWorkspace() {
  const user = useCurrentUser();
  const canEdit = user?.role === "editor" || user?.role === "admin";
  const searchParams = useSearchParams();
  const versionParam = searchParams.get("version");
  const workflowParam = searchParams.get("workflow");
  const loadTarget = studioLoadTarget(versionParam, workflowParam);
  const [nodes, setNodes, onNodesChange] =
    useNodesState<Node<CardData>>(starterNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>(starterEdges);
  const [cards, setCards] = useState<CardType[]>(localCards);
  const [inspectedNodeID, setInspectedNodeID] = useState<string>();
  const [message, setMessage] = useState("Fluxo local pronto para validar.");
  const [busy, setBusy] = useState(false);
  const [integrations, setIntegrations] = useState<Integration[]>([]);
  const [modelProfiles, setModelProfiles] = useState<ModelProfile[]>([]);
  const [showConnections, setShowConnections] = useState(false);
  const [metadata, setMetadata] = useState<WorkflowMetadata>(
    defaultWorkflowMetadata,
  );
  const [openedVersionID, setOpenedVersionID] = useState<number>();
  const [dirty, setDirty] = useState(false);
  const [transfer, setTransfer] = useState<"clone" | "import" | "export">();
  const [workflowKeys, setWorkflowKeys] = useState<string[]>([]);
  const [selectedNodeIDs, setSelectedNodeIDs] = useState<string[]>([]);
  const [selectedEdgeIDs, setSelectedEdgeIDs] = useState<string[]>([]);
  const [history, setHistory] = useState<StudioHistory>({
    past: [],
    future: [],
  });
  const [canvasSearch, setCanvasSearch] = useState("");
  const [focusedNodeID, setFocusedNodeID] = useState<string>();
  const [showManualRun, setShowManualRun] = useState(false);
  const [manualTriggerID, setManualTriggerID] = useState("");
  const [manualPayload, setManualPayload] = useState("");
  const [runTarget, setRunTarget] = useState<"draft" | "published">("draft");
  const [openedVersionPublished, setOpenedVersionPublished] = useState(false);
  const [webhookRegistrations, setWebhookRegistrations] = useState<
    WebhookRegistration[]
  >([]);
  const definition = useMemo(
    () => toDefinition(nodes, edges, metadata),
    [nodes, edges, metadata],
  );
  const currentDefinition = useRef(definition);
  useEffect(() => {
    currentDefinition.current = definition;
  }, [definition]);
  const remember = useCallback(
    () =>
      setHistory((current) => pushHistory(current, currentDefinition.current)),
    [],
  );
  const validationIssues = useMemo(
    () => validateStudioWorkflow(definition, cards),
    [definition, cards],
  );
  const inspectedCard = useMemo(
    () => nodes.find((node) => node.id === inspectedNodeID)?.data,
    [inspectedNodeID, nodes],
  );
  const manualTriggers = useMemo(
    () => nodes.filter((node) => isManualTrigger(node.data)),
    [nodes],
  );

  const loadIntegrations = async () => {
    try {
      const [items, profiles] = await Promise.all([
        getIntegrations(),
        getModelProfiles(),
      ]);
      setIntegrations(items);
      setModelProfiles(profiles);
    } catch {
      setMessage("Não foi possível carregar as integrações.");
    }
  };
  useEffect(() => {
    setBusy(true);
    setMessage(
      loadTarget.kind === "version"
        ? `Abrindo versão salva ${loadTarget.versionID}...`
        : "Abrindo pipeline publicado...",
    );
    const load =
      loadTarget.kind === "version"
        ? Promise.all([
            getCards(),
            getWorkflowVersion(loadTarget.versionID),
            getWorkflows(),
          ]).then(([catalog, definition, workflows]) => ({
            catalog,
            definition,
            versionID: loadTarget.versionID,
            published: workflows.some((workflow) =>
              workflow.versions.some(
                (version) =>
                  version.version_id === loadTarget.versionID &&
                  version.status === "published",
              ),
            ),
          }))
        : Promise.all([getCards(), getPublishedWorkflow(loadTarget.workflowKey)]).then(
            ([catalog, published]) => ({
              catalog,
              definition: published.definition,
              versionID: published.version_id,
              published: true,
            }),
          );
    void load
      .then(({ catalog, definition, versionID: loadedVersionID, published }) => {
        const hydrated = hydrateDefinition(definition, catalog);
        setCards(catalog);
        setNodes(hydrated.nodes);
        setEdges(hydrated.edges);
        setInspectedNodeID(undefined);
        setMetadata({
          key: definition.key,
          name: definition.name,
          description: definition.description,
        });
        setOpenedVersionID(loadedVersionID);
        setOpenedVersionPublished(published);
        setDirty(false);
        setMessage(
          `Versão salva ${loadedVersionID} aberta. Alterações serão salvas como um novo rascunho; a versão original permanece imutável.`,
        );
      })
      .catch((error) => {
        setOpenedVersionID(undefined);
        setOpenedVersionPublished(false);
        if (loadTarget.kind === "published" && "status" in (error as object) && (error as { status?: number }).status === 404) {
          setNodes([]);
          setEdges([]);
          setInspectedNodeID(undefined);
          setMetadata(defaultWorkflowMetadata);
          setDirty(false);
          setMessage("Nenhuma pipeline publicada está disponível. Crie ou abra uma pipeline para iniciar o canvas.");
          return;
        }
        setMessage(error instanceof Error ? error.message : "Não foi possível abrir o workflow.");
      })
      .finally(() => setBusy(false));
  }, [loadTarget.kind, loadTarget.kind === "version" ? loadTarget.versionID : loadTarget.workflowKey, setEdges, setNodes]);
  useEffect(() => {
    void Promise.all([getIntegrations(), getModelProfiles()])
      .then(([items, profiles]) => {
        setIntegrations(items);
        setModelProfiles(profiles);
      })
      .catch(() => undefined);
  }, []);
  useEffect(() => {
    if (user?.role === "admin")
      void getWebhookRegistrations()
        .then(setWebhookRegistrations)
        .catch(() => undefined);
  }, [user?.role]);

  const addCard = (card: CardType, position?: { x: number; y: number }) => {
    remember();
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
        position: position ?? {
          x: 210 + (all.length % 4) * 280,
          y: 90 + Math.floor(all.length / 4) * 250,
        },
        data,
      },
    ]);
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
    setInspectedNodeID(undefined);
    setDirty(true);
    setMessage(
      "Template de review carregado. Configure Gitea, modelo e dados do PR nos cards selecionados.",
    );
  };
  const connect = useCallback(
    (connection: Connection) => {
      const source = nodes.find((node) => node.id === connection.source);
      const target = nodes.find((node) => node.id === connection.target);
      const sourcePort =
        source &&
        cardOutputPorts(source.data).find(
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
      remember();
      setEdges((all) => addEdge({ ...connection, animated: false }, all));
      setDirty(true);
    },
    [nodes, remember, setEdges],
  );
  const patchInspected = (patch: Partial<CardData>) => {
    if (!inspectedNodeID) return;
    remember();
    setNodes((all) =>
      all.map((node) =>
        node.id === inspectedNodeID
          ? { ...node, data: { ...node.data, ...patch } }
          : node,
      ),
    );
    setDirty(true);
  };
  const applyReport = (report: ExecutionReport) =>
    setNodes((all) => applyExecutionReport(all, report));
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
        error instanceof Error
          ? error.message
          : "Não foi possível salvar o rascunho.",
      );
    } finally {
      setBusy(false);
    }
  };
  const restoreHistory = useCallback(
    (next: import("../../lib/types").WorkflowDefinition) => {
      const hydrated = hydrateDefinition(next, cards);
      setNodes(hydrated.nodes);
      setEdges(hydrated.edges);
      setMetadata({
        key: next.key,
        name: next.name,
        description: next.description,
      });
      setDirty(true);
    },
    [cards, setEdges, setNodes],
  );
  useEffect(() => {
    const shortcut = (event: KeyboardEvent) => {
      if (isEditableTarget(event.target)) return;
      const mod = event.ctrlKey || event.metaKey;
      if (mod && event.key.toLowerCase() === "s") {
        event.preventDefault();
        if (canEdit) void saveDraft();
        return;
      }
      if (mod && ["z", "y"].includes(event.key.toLowerCase())) {
        event.preventDefault();
        const redo = event.key.toLowerCase() === "y" || event.shiftKey;
        const result = redo
          ? redoHistory(history, currentDefinition.current)
          : undoHistory(history, currentDefinition.current);
        if (result) {
          setHistory(result.history);
          restoreHistory(result.definition);
        }
        return;
      }
      if ((mod && event.key.toLowerCase() === "k") || event.key === "/") {
        event.preventDefault();
        document
          .querySelector<HTMLInputElement>("[data-canvas-search]")
          ?.focus();
        return;
      }
      if (event.key === "?") {
        setMessage(
          "Atalhos: ⌘/Ctrl+Z desfaz, ⌘/Ctrl+Y refaz, ⌘/Ctrl+S salva, ⌘/Ctrl+K ou / busca, F foca seleção e Esc fecha.",
        );
        return;
      }
      if (event.key === "Escape") {
        setInspectedNodeID(undefined);
        setSelectedNodeIDs([]);
        setSelectedEdgeIDs([]);
        document
          .querySelectorAll("details[open]")
          .forEach((item) => item.removeAttribute("open"));
        return;
      }
      if (event.key.toLowerCase() === "f" && selectedNodeIDs[0])
        setFocusedNodeID(selectedNodeIDs[0]);
    };
    window.addEventListener("keydown", shortcut);
    return () => window.removeEventListener("keydown", shortcut);
  }, [canEdit, history, restoreHistory, selectedNodeIDs]);
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
  const saveAndRun = async (triggerNodeID: string) => {
    if (validationIssues.length) {
      setMessage("Corrija os ajustes indicados antes de executar.");
      return;
    }
    const trigger = nodes.find((node) => node.id === triggerNodeID)?.data;
    if (!trigger || !isManualTrigger(trigger)) {
      setMessage("Selecione um trigger manual para executar pelo Studio.");
      return;
    }
    let testPayload: Record<string, unknown> = {};
    if (manualPayload.trim()) {
      try {
        const parsed = JSON.parse(manualPayload) as unknown;
        if (!parsed || Array.isArray(parsed) || typeof parsed !== "object")
          throw new Error();
        testPayload = parsed as Record<string, unknown>;
      } catch {
        setMessage("O payload de teste precisa ser um objeto JSON válido.");
        return;
      }
    }
    setShowManualRun(false);
    setBusy(true);
    setMessage(
      runTarget === "published"
        ? "Executando versão publicada..."
        : "Salvando versão do workflow...",
    );
    try {
      const targetVersionID =
        runTarget === "published"
          ? openedVersionID
          : (await saveWorkflow(definition)).version_id;
      if (!targetVersionID)
        throw new Error("Nenhuma versão publicada está aberta para executar.");
      if (runTarget === "draft") setDirty(false);
      setNodes(resetExecutionStatuses);
      const started =
        runTarget === "published"
          ? await executePublishedWorkflow(
              targetVersionID,
              trigger.key,
              testPayload,
            )
          : await executeWorkflow(targetVersionID, trigger.key, testPayload);
      let report = started.report;
      let contractIssue = report?.contractIssue;
      let eventSource: EventSource | undefined;
      if (started.status === "queued" && started.execution_id) {
        eventSource = new EventSource(
          `${apiURL}/api/executions/${started.execution_id}/events`,
          { withCredentials: true },
        );
        eventSource.addEventListener("execution", (event) => {
          try {
            const update = JSON.parse((event as MessageEvent).data) as {
              node?: { node_key: string; status: CardData["status"] };
            };
            if (update.node)
              setNodes((all) =>
                all.map((node) =>
                  node.data.key === update.node?.node_key
                    ? {
                        ...node,
                        data: { ...node.data, status: update.node.status },
                      }
                    : node,
                ),
              );
          } catch {
            /* malformed safe event is ignored; polling remains a fallback */
          }
        });
        for (let attempt = 0; attempt < 120; attempt += 1) {
          await new Promise((resolve) => setTimeout(resolve, 300));
          try {
            const pending = await getExecution(started.execution_id);
            applyReport(pending);
            contractIssue ||= pending.contractIssue;
            if (pending.status !== "queued" && pending.status !== "running") {
              report = pending;
              break;
            }
          } catch (error) {
            if (
              error instanceof Error &&
              "status" in error &&
              error.status === 401
            )
              throw error;
          }
        }
        eventSource?.close();
        if (!report) {
          setNodes((all) =>
            all.map((node) =>
              node.data.status === "running"
                ? { ...node, data: { ...node.data, status: "idle" } }
                : node,
            ),
          );
          setMessage(
            `Execução ${started.execution_id} continua na fila; o acompanhamento ao vivo foi encerrado.`,
          );
        }
      }
      if (report) applyReport(report);
      setMessage(
        contractIssue
          ? contractIssue
          : report
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
  const openManualRun = (target: "draft" | "published") => {
    const preferred = manualTriggers.some((node) => node.id === inspectedNodeID)
      ? inspectedNodeID
      : manualTriggers[0]?.id;
    setManualTriggerID(preferred ?? "");
    setRunTarget(target);
    setShowManualRun(true);
  };
  const openTransfer = (mode: "clone" | "import" | "export") => {
    if (mode !== "clone") return setTransfer(mode);
    void getWorkflows()
      .then((workflows) => {
        setWorkflowKeys(workflows.map((workflow) => workflow.key));
        setTransfer(mode);
      })
      .catch(() =>
        setMessage(
          "Não foi possível verificar os nomes existentes para criar a cópia.",
        ),
      );
  };
  const applyTransferredDefinition = (
    definition: import("../../lib/types").WorkflowDefinition,
    nextMessage: string,
  ) => {
    const hydrated = hydrateDefinition(definition, cards);
    setNodes(hydrated.nodes);
    setEdges(hydrated.edges);
    setInspectedNodeID(undefined);
    setMetadata({
      key: definition.key,
      name: definition.name,
      description: definition.description,
    });
    setOpenedVersionID(undefined);
    setDirty(true);
    setMessage(nextMessage);
    setTransfer(undefined);
  };
  const trackNodeChanges = useCallback(
    (...args: Parameters<typeof onNodesChange>) => {
      if (!canEdit) return;
      if (
        args[0].some((change) =>
          ["add", "remove", "replace", "position"].includes(change.type),
        )
      )
        setDirty(true);
      onNodesChange(...args);
    },
    [canEdit, onNodesChange],
  );
  const trackEdgeChanges = useCallback(
    (...args: Parameters<typeof onEdgesChange>) => {
      if (!canEdit) return;
      if (
        args[0].some((change) =>
          ["add", "remove", "replace"].includes(change.type),
        )
      )
        setDirty(true);
      onEdgesChange(...args);
    },
    [canEdit, onEdgesChange],
  );
  const removeSelection = useCallback(() => {
    if (!canEdit) return;
    if (!selectedNodeIDs.length && !selectedEdgeIDs.length) {
      setMessage("Selecione um ou mais cards ou conexões para remover.");
      return;
    }
    const removed = removeSelectedElements(
      nodes,
      edges,
      selectedNodeIDs,
      selectedEdgeIDs,
    );
    setNodes(removed.nodes);
    setEdges(removed.edges);
    setInspectedNodeID((current) =>
      current && selectedNodeIDs.includes(current) ? undefined : current,
    );
    setSelectedNodeIDs([]);
    setSelectedEdgeIDs([]);
    setDirty(true);
    setMessage(
      `${selectedNodeIDs.length} card(s) e ${removed.removedEdges} conexão(ões) removidos.`,
    );
  }, [
    canEdit,
    edges,
    nodes,
    selectedEdgeIDs,
    selectedNodeIDs,
    setEdges,
    setNodes,
  ]);
  const deleteNode = useCallback(
    (nodeID: string) => {
      if (!canEdit) return;
      setNodes((all) => all.filter((node) => node.id !== nodeID));
      setEdges((all) =>
        all.filter((edge) => edge.source !== nodeID && edge.target !== nodeID),
      );
      setInspectedNodeID((current) =>
        current === nodeID ? undefined : current,
      );
      setSelectedNodeIDs((all) => all.filter((id) => id !== nodeID));
      setDirty(true);
      setMessage("Card e conexões vinculadas removidos.");
    },
    [canEdit, setEdges, setNodes],
  );
  const editNode = useCallback((nodeID: string) => {
    setInspectedNodeID(nodeID);
  }, []);
  const closeInspector = useCallback(() => {
    setInspectedNodeID(undefined);
  }, []);
  const clearCanvasEditing = useCallback(() => {
    setInspectedNodeID(undefined);
    setSelectedNodeIDs([]);
    setSelectedEdgeIDs([]);
  }, []);
  const handleSelectionChange = useCallback(
    (selectedNodes: Node<CardData>[], selectedEdges: Edge[]) => {
      const nextNodeIDs = selectedNodes.map((node) => node.id);
      const nextEdgeIDs = selectedEdges.map((edge) => edge.id);
      setSelectedNodeIDs((current) =>
        selectionHasChanged(current, nextNodeIDs) ? nextNodeIDs : current,
      );
      setSelectedEdgeIDs((current) =>
        selectionHasChanged(current, nextEdgeIDs) ? nextEdgeIDs : current,
      );
    },
    [],
  );
  const selectValidationIssue = (issue: WorkflowValidationIssue) => {
    const node =
      issue.nodeKey && nodes.find((item) => item.id === issue.nodeKey);
    if (node) {
      setInspectedNodeID(node.id);
      setSelectedNodeIDs([node.id]);
      setSelectedEdgeIDs([]);
      setMessage(`Ajuste destacado: ${issue.message}`);
    } else {
      setMessage(issue.message);
    }
  };
  const registerWebhook = useCallback(
    async (registration: {
      key: string;
      name: string;
      workflow_key: string;
      trigger_node_key: string;
      secret: string;
      active: boolean;
    }) => {
      try {
        const saved = await saveWebhookRegistration(registration);
        setWebhookRegistrations((all) => [
          ...all.filter((item) => item.key !== saved.key),
          saved,
        ]);
        setMessage(`Webhook ${saved.name} registrado sem expor o segredo.`);
      } catch (error) {
        setMessage(
          error instanceof Error
            ? error.message
            : "Não foi possível registrar o webhook.",
        );
        throw error;
      }
    },
    [],
  );
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
    <AppShell
      compact
      title="Studio"
      headerActions={
        <div className={styles.actionArea}>
          <div className={styles.identity}>
            <span>
              {openedVersionID ? `Versão ${openedVersionID}` : metadata.name}
            </span>
          </div>
          <div className={styles.status} aria-label="Status do workflow">
            {dirty && <em className={styles.unsaved}>Não salvo</em>}
            <em
              className={
                validationIssues.length ? styles.invalid : styles.valid
              }
            >
              {validationIssues.length
                ? `${validationIssues.length} ajuste(s)`
                : "Válido"}
            </em>
          </div>
          <div className={styles.actions}>
            <details className={styles.overflow}>
              <summary aria-label="Abrir ações do Studio" title="Mais ações">
                <MoreHorizontal size={18} aria-hidden="true" />
                <span>Ações</span>
              </summary>
              <div className={styles.menu} aria-label="Ações do Studio">
                <button disabled={!canEdit} onClick={loadReviewTemplate}>
                  <BookOpen size={14} /> Template review
                </button>
                {user?.role === "admin" && (
                  <button onClick={openConnections}>
                    <Settings2 size={14} /> Nova integração
                  </button>
                )}
                {studioManagementActions(user?.role).map((item) => (
                  <Link key={item.href} href={item.href}>
                    {item.label}
                  </Link>
                ))}
                <hr />
                <button
                  disabled={!canEdit || busy}
                  onClick={() => openTransfer("clone")}
                >
                  <Copy size={14} /> Clonar workflow
                </button>
                <button
                  disabled={!canEdit || busy}
                  onClick={() => openTransfer("import")}
                >
                  <Upload size={14} /> Importar JSON
                </button>
                <button
                  disabled={!canEdit || busy}
                  onClick={() => openTransfer("export")}
                >
                  <Download size={14} /> Exportar JSON
                </button>
                <button
                  disabled={busy || !canEdit}
                  onClick={() =>
                    setMessage(
                      validationIssues.length
                        ? "Há ajustes locais pendentes no workflow."
                        : "Validação local concluída. O backend confirmará ao salvar.",
                    )
                  }
                >
                  <ShieldCheck size={14} /> Validar workflow
                </button>
                <button
                  disabled={busy || !canEdit}
                  onClick={() => void saveDraft()}
                >
                  <Save size={14} /> Salvar rascunho
                </button>
                {canEdit && (
                  <button disabled={busy} onClick={removeSelection}>
                    <Trash2 size={14} /> Excluir seleção
                  </button>
                )}
              </div>
            </details>
            <button
              className={styles.publish}
              disabled={busy || !canEdit}
              onClick={() => void publishCurrent()}
            >
              <Upload size={14} />{" "}
              <span className={styles.actionLabel}>Publicar</span>
            </button>
            <button
              className={styles.primary}
              disabled={busy || !canEdit}
              onClick={() => openManualRun("draft")}
            >
              <CirclePlay size={14} />{" "}
              <span className={styles.actionLabel}>
                {busy ? "Executando" : "Salvar e executar"}
              </span>
            </button>
            {openedVersionPublished && !dirty && (
              <button
                className={styles.publish}
                disabled={busy || !canEdit}
                onClick={() => openManualRun("published")}
              >
                <CirclePlay size={14} /> Executar publicada
              </button>
            )}
          </div>
        </div>
      }
    >
      <div
        className={`${styles.studio} ${inspectedCard ? styles.inspectorOpen : ""}`}
      >
        <CardLibrary cards={cards} onAdd={addCard} readOnly={!canEdit} />
        <WorkflowCanvas
          nodes={nodes}
          edges={edges}
          message={message}
          onNodesChange={trackNodeChanges}
          onEdgesChange={trackEdgeChanges}
          onConnect={connect}
          onSelectionChange={handleSelectionChange}
          onRequestDelete={removeSelection}
          onEditNode={editNode}
          onDeleteNode={deleteNode}
          onPaneClick={clearCanvasEditing}
          validationIssues={validationIssues}
          onSelectValidationIssue={selectValidationIssue}
          onAddCardAt={(key, position) => {
            const card = cards.find((item) => item.key === key);
            if (card) addCard(card, position);
          }}
          onDuplicateNode={(nodeID) => {
            if (!canEdit) return;
            const source = nodes.find((node) => node.id === nodeID);
            if (!source) return;
            remember();
            const id = `${source.data.type}-${Date.now().toString(36)}`;
            setNodes((all) => [
              ...all,
              {
                ...source,
                id,
                position: {
                  x: source.position.x + 44,
                  y: source.position.y + 44,
                },
                data: {
                  ...source.data,
                  key: id,
                  name: `${source.data.name} (cópia)`,
                  config: structuredClone(source.data.config),
                  status: "idle",
                },
              },
            ]);
            setSelectedNodeIDs([id]);
            setDirty(true);
          }}
          onDeleteEdge={(edgeID) => {
            if (!canEdit) return;
            setEdges((all) => all.filter((edge) => edge.id !== edgeID));
            setSelectedEdgeIDs((all) => all.filter((id) => id !== edgeID));
            setDirty(true);
            setMessage("Conexão removida.");
          }}
          searchQuery={canvasSearch}
          onSearchQueryChange={(query) => {
            setCanvasSearch(query);
            const match = nodes.find((node) =>
              node.data.name.toLowerCase().includes(query.toLowerCase()),
            );
            if (query && match) {
              setFocusedNodeID(match.id);
              setSelectedNodeIDs([match.id]);
            }
          }}
          focusNodeID={focusedNodeID}
          readOnly={!canEdit}
        />
        {inspectedCard && (
          <CardInspector
            nodeID={inspectedNodeID!}
            selected={inspectedCard}
            integrations={integrations}
            modelProfiles={modelProfiles}
            hasErrorRoute={hasErrorRoute(edges, inspectedCard.key)}
            onChange={(patch) => canEdit && patchInspected(patch)}
            onClose={closeInspector}
            readOnly={!canEdit}
            webhookRegistrations={webhookRegistrations}
            canAdministerWebhooks={user?.role === "admin"}
            workflowKey={metadata.key}
            onRegisterWebhook={registerWebhook}
          />
        )}
      </div>
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
      {transfer && (
        <WorkflowTransferModal
          mode={transfer}
          definition={definition}
          cards={cards}
          workflowKeys={workflowKeys}
          onClose={() => setTransfer(undefined)}
          onApply={applyTransferredDefinition}
        />
      )}
      {showManualRun && (
        <ModalShell
          title="Executar workflow"
          eyebrow="TRIGGER MANUAL"
          description={
            runTarget === "published"
              ? "Executa a versão publicada aberta, sem salvar um rascunho. O payload não é persistido no navegador."
              : "Salva um novo rascunho e inicia a execução pelo trigger selecionado. O payload não é persistido no navegador."
          }
          onClose={() => setShowManualRun(false)}
          className={styles.runModal}
          footer={
            <>
              <button type="button" onClick={() => setShowManualRun(false)}>
                Cancelar
              </button>
              <button
                type="button"
                className={styles.primary}
                disabled={!manualTriggerID || busy}
                onClick={() => void saveAndRun(manualTriggerID)}
              >
                <CirclePlay size={14} />{" "}
                {runTarget === "published"
                  ? "Executar publicada"
                  : "Salvar e executar"}
              </button>
            </>
          }
        >
          <div className={styles.runForm}>
            <label>
              Trigger manual
              <select
                value={manualTriggerID}
                onChange={(event) => setManualTriggerID(event.target.value)}
              >
                <option value="">Selecione um trigger</option>
                {manualTriggers.map((node) => (
                  <option key={node.id} value={node.id}>
                    {node.data.name}
                  </option>
                ))}
              </select>
            </label>
            {!manualTriggers.length && (
              <p role="alert">
                Este workflow não possui um trigger em modo manual.
              </p>
            )}
            <label>
              Payload JSON opcional
              <textarea
                aria-label="Payload JSON opcional do trigger manual"
                value={manualPayload}
                onChange={(event) => setManualPayload(event.target.value)}
                placeholder='{"pull_request":{"owner":"acme","repo":"api","number":42}}'
                spellCheck={false}
              />
            </label>
          </div>
        </ModalShell>
      )}
    </AppShell>
  );
}
