"use client";

import { createContext, useContext } from "react";

export type WorkflowCardActions = {
  readOnly: boolean;
  onEdit: (nodeID: string) => void;
  onDelete: (nodeID: string) => void;
};

const WorkflowCardActionsContext = createContext<WorkflowCardActions | null>(null);

export const WorkflowCardActionsProvider = WorkflowCardActionsContext.Provider;

export function useWorkflowCardActions() {
  return useContext(WorkflowCardActionsContext);
}
