// Unified status for rendering
export type NodeRunStatus = 'running' | 'succeeded' | 'failed' | 'stopped' | 'paused';

// New item shape (pure presentational data)
export interface WorkflowRunNodeListItem {
  title: string;
  nodeId: string;
  executionId?: string;
  createdAtMs?: number;
  receivedOrder?: number;
  nodeType: string; // prefer keyof typeof NODE_THEMES, but allow string fallback for forward compatibility
  status: NodeRunStatus;
  nodeInput?: unknown;
  nodeOutput?: unknown;
  modelInput?: unknown;
  processData?: unknown;
  executionMetadata?: unknown;
  elapsedTime?: number;
  error?: string | null;
  iterationInputs?: unknown;
  iterationOutputs?: unknown;
  iterationRounds?: Array<{
    index: number;
    nodes: WorkflowRunNodeListItem[];
    elapsedTime?: number;
  }>;
  loopInputs?: unknown;
  loopOutputs?: unknown;
  loopRounds?: Array<{
    index: number;
    nodes: WorkflowRunNodeListItem[];
    elapsedTime?: number;
    variables?: unknown;
  }>;
  steps?: number;
}

// Component props: accept new shape array
export interface WorkflowRunNodesListProps {
  items: WorkflowRunNodeListItem[];
  showDetail?: boolean;
  variant?: 'panel' | 'canvas';
  hideCanvasNodeChrome?: boolean;
}

export interface WorkflowRunNodeGroup {
  key: string;
  executions: WorkflowRunNodeListItem[];
}

export interface RuntimeLogSection {
  id: string;
  title: string;
  value: unknown;
  accent: string;
}

export interface RuntimeLogPreviewRow {
  label: string;
  value: unknown;
  tone?: 'input' | 'output' | 'meta' | 'warning';
  maxRecordEntries?: number;
}

export type RuntimeLabel = (key: string, params?: Record<string, string | number>) => string;

