// Strictly typed structures used by Workflow Run Panel
// Keep small and focused to avoid circular deps

export interface WorkflowFinishedData {
  id: string;
  status: string;
  created_at?: number; // unix seconds or ms
  finished_at?: number | null; // unix seconds or ms
  elapsed_time?: number; // backend duration value; history APIs are commonly seconds, SSE may differ
  total_steps?: number;
  workflow_id?: string;
  outputs?: unknown;
  inputs?: Record<string, unknown> | undefined;
  total_tokens?: number; // optional token count
  error?: unknown; // optional error payload when run failed
  conversation_id?: string;
  message_id?: string;
}

export type HistoryResult =
  | { kind: 'empty' }
  | { kind: 'text'; content: string }
  | { kind: 'json'; value: unknown };
