"use client";

import { Download, Files, Upload } from "lucide-react";
import { useState } from "react";
import type { CardType, WorkflowDefinition } from "../../lib/types";
import {
  cloneWorkflowDefinition,
  parseWorkflowExport,
  validateWorkflowDefinition,
  workflowExportEnvelope,
} from "../../lib/workflow";
import { ModalShell } from "../common/ModalShell";
import styles from "./WorkflowTransferModal.module.scss";

type Mode = "clone" | "import" | "export";

type Props = {
  mode: Mode;
  definition: WorkflowDefinition;
  cards: CardType[];
  workflowKeys: string[];
  onClose: () => void;
  onApply: (definition: WorkflowDefinition, message: string) => void;
};

export function WorkflowTransferModal({ mode, definition, cards, workflowKeys, onClose, onApply }: Props) {
  const [candidate, setCandidate] = useState<WorkflowDefinition>();
  const [error, setError] = useState<string>();
  const title = mode === "clone" ? "Clonar workflow" : mode === "import" ? "Importar workflow" : "Exportar workflow";
  const validation = validateWorkflowDefinition(definition, cards);
  const preview = mode === "import" ? candidate : mode === "clone" ? cloneWorkflowDefinition(definition, workflowKeys) : definition;

  const exportDefinition = () => {
    if (validation) return setError(validation);
    const file = new Blob([JSON.stringify(workflowExportEnvelope(definition), null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(file);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${definition.key}.forgereview.json`;
    link.click();
    URL.revokeObjectURL(url);
    onClose();
  };

  const importFile = async (file?: File) => {
    if (!file) return;
    const result = parseWorkflowExport(await file.text(), cards);
    setError(result.error);
    setCandidate(result.envelope?.definition);
  };

  return (
    <ModalShell
      title={title}
      eyebrow="GESTÃO VISUAL"
      description={mode === "clone" ? "A cópia terá uma nova identidade e será salva como outro workflow." : mode === "import" ? "O arquivo é validado localmente antes de alterar o canvas. Segredos e ciphertext são recusados." : "Baixe apenas a definição segura e versionada; credenciais não fazem parte do arquivo."}
      onClose={onClose}
      className={styles.modal}
      footer={<>
        <button type="button" onClick={onClose}>Cancelar</button>
        {mode === "export" ? <button type="button" className={styles.primary} onClick={exportDefinition}><Download size={15} /> Baixar JSON</button> :
          <button type="button" className={styles.primary} disabled={!preview || Boolean(error)} onClick={() => { if (preview) onApply(preview, mode === "clone" ? "Cópia carregada no canvas. Salve para criar o novo rascunho." : "Workflow importado no canvas. Revise e salve como rascunho."); }}>{mode === "clone" ? <Files size={15} /> : <Upload size={15} />}{mode === "clone" ? "Confirmar cópia" : "Usar no canvas"}</button>}
      </>}
    >
      {mode === "import" && <label className={styles.file}><span>Arquivo de exportação</span><input type="file" accept="application/json,.json" onChange={(event) => void importFile(event.target.files?.[0])} /></label>}
      {error && <p className={styles.error} role="alert">{error}</p>}
      {mode === "export" && validation && <p className={styles.error} role="alert">A exportação foi bloqueada: {validation}</p>}
      {preview && <section className={styles.preview} aria-label="Prévia da definição">
        <span>PRÉVIA</span>
        <strong>{preview.name}</strong>
        <p><code>{preview.key}</code> · {preview.nodes.length} cards · {preview.edges.length} conexões</p>
      </section>}
    </ModalShell>
  );
}
