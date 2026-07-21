import type { Node } from "@xyflow/react";
import type { PipelineStage, PipelineTransition } from "./pipeline-types";

export type Stage = PipelineStage;
export type Transition = PipelineTransition;

export type Trigger = {
  source: "webhook" | "api" | "manual";
  enabled: boolean;
  target_stage_key: string;
  config: Record<string, unknown>;
};

export type Version = {
  id: number;
  version: number;
  status: string;
  profile_id?: number;
  scheduler_max_runs: number;
  stages: Stage[];
  transitions: Transition[];
  triggers?: Trigger[] | null;
};

export type Pipeline = {
  id: number;
  name: string;
  description: string;
  profile_id?: number;
  is_default: boolean;
  versions: Version[];
};

export type RuntimeEvent = {
  status?: string;
  percent?: number;
  message?: string;
  timestamp?: string;
};

export type StudioNodeData = {
  stage: Stage;
  update: (patch: Partial<Stage>) => void;
  remove: () => void;
  select: () => void;
  openLogs: () => void;
  editable: boolean;
  removable: boolean;
  runtime?: RuntimeEvent;
  active?: boolean;
};

export type EntrypointNodeData = {
  trigger: Trigger;
  editable: boolean;
  updateSource: (source: Trigger["source"]) => void;
  toggle: () => void;
};

export type StudioNode = Node<StudioNodeData>;
