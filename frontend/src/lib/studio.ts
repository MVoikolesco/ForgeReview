import type { WorkflowDefinition } from "./types";

export type StudioHistory = { past: WorkflowDefinition[]; future: WorkflowDefinition[] };

/** Keeps only serializable workflow definitions; selection and execution status never enter history. */
export const pushHistory = (history: StudioHistory, current: WorkflowDefinition): StudioHistory => ({
  past: [...history.past, structuredClone(current)],
  future: [],
});

export const undoHistory = (history: StudioHistory, current: WorkflowDefinition) => {
  const previous = history.past.at(-1);
  return previous
    ? { definition: structuredClone(previous), history: { past: history.past.slice(0, -1), future: [structuredClone(current), ...history.future] } }
    : undefined;
};

export const redoHistory = (history: StudioHistory, current: WorkflowDefinition) => {
  const next = history.future[0];
  return next
    ? { definition: structuredClone(next), history: { past: [...history.past, structuredClone(current)], future: history.future.slice(1) } }
    : undefined;
};

export const searchCards = <T extends { name: string; key: string; category?: string }>(cards: T[], query: string) => {
  const terms = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean);
  return !terms.length ? cards : cards.filter((card) => {
    const text = `${card.name} ${card.key} ${card.category ?? ""}`.toLocaleLowerCase();
    return terms.every((term) => text.includes(term));
  });
};

export const isEditableTarget = (target: EventTarget | null) => {
  const element = target instanceof Element ? target : null;
  return Boolean(element?.closest("input, textarea, select, [contenteditable='true'], [role='dialog']"));
};
