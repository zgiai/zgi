import type { MessageEndEvent } from '@/hooks/workflow/use-run-workflow-chat-draft-stream';

export type RunStatus =
  | 'running'
  | 'pending_approval'
  | 'pending_question'
  | 'completed'
  | 'error'
  | 'stopped'
  | 'expired';

export type TerminalRunStatus = Extract<RunStatus, 'completed' | 'error' | 'stopped' | 'expired'>;

export type NodeRunStatus =
  | 'running'
  | 'failed'
  | 'success'
  | 'succeeded'
  | 'partial-succeeded'
  | 'stopped'
  | 'paused';

export interface NodeIOData {
  input?: unknown;
  output?: unknown;
  modelInput?: unknown;
}

export interface NodeInfo {
  status: NodeRunStatus;
  error?: string;
  elapsedTime?: number; // backend duration value; current workflow streams use milliseconds
  nodeId?: string;
  executionId?: string;
  createdAtMs?: number;
  receivedOrder?: number;
  nodeType?: string;
  title?: string;
  data?: NodeIOData;
  iterationInputs?: unknown;
  iterationOutputs?: unknown;
  iterationRounds?: Array<{ index: number; nodes: NodeInfo[]; elapsedTime?: number }>;
  loopInputs?: unknown;
  loopOutputs?: unknown;
  loopRounds?: Array<{
    index: number;
    nodes: NodeInfo[];
    elapsedTime?: number;
    variables?: unknown;
  }>;
  steps?: number;
}

export interface WorkflowRunInfo {
  id: string;
  status: RunStatus;
  error?: string;
  elapsedTime?: number; // backend duration value; current workflow streams use milliseconds
  runNodeInfo: NodeInfo[];
}

// Client-only message state for UI responsiveness independent of workflow
export type ClientPhase = 'idle' | 'requesting' | 'streaming' | 'completed';
export interface ClientMessageState {
  phase: ClientPhase;
  status?: Exclude<RunStatus, 'running'>;
  error?: string;
  startedAt?: number;
  finishedAt?: number;
}

export interface ModelInfo {
  modelName: string;
  // Additional arbitrary fields for model metadata
  [key: string]: unknown;
}

import type { WorkflowFileType } from '@/components/workflow/types/input-var';

// Attachment payload passed with chat input, parallel to `query`
export type AttachmentTransferMethod = 'local_file';
export type AttachmentType = WorkflowFileType;
export interface ChatAttachment {
  type: AttachmentType;
  transfer_method: AttachmentTransferMethod;
  url: string;
  upload_file_id: string;
}

export interface GeneratedImage {
  url: string;
  alt?: string;
  width?: number;
  height?: number;
  isLoading?: boolean;
}

export interface Message {
  // Backend message id (empty string for newly created message)
  messageId: string;
  // The user query content
  query: string;
  // The AI answer content, streamed into this field
  answer: string;
  // Parent message id for threading (optional for simple chat)
  parentId: string;
  // Optional workflow run details for this message
  WorkflowRunInfo?: WorkflowRunInfo;
  // Client-only state for loading/skeleton, independent of workflow usage
  clientState?: ClientMessageState;
  // User provided inputs for the workflow kickoff
  inputs?: Record<string, unknown>;
  // Optional model metadata that produced the message
  model?: ModelInfo | null;
  // Arbitrary custom data bag for extensions
  messageData: Record<string, unknown>;
  // Generated images
  generatedImages?: GeneratedImage[];
}

export interface Conversation {
  // Frontend-only uuid for this conversation (created on client)
  id: string;
  // Backend storage id (empty string for new conversations)
  conversationId: string;
  // Display title
  title: string;
  // Messages pair list; each item includes both query and answer
  messages: Message[];
  // Arbitrary custom data for conversation-level metadata
  conversationData: Record<string, unknown>;
}

export interface ChatRunStartedContext {
  query: string;
  tempKey?: string;
  workflowRunId?: string;
  workflowId?: string;
  createdAt?: number;
  inputs?: Record<string, unknown>;
}

export interface ChatRunFinishedContext {
  status: TerminalRunStatus;
  error?: string;
  elapsedTime?: number; // backend duration value; current workflow streams use milliseconds
  messageId?: string;
  workflowRunId?: string;
  model?: ModelInfo | null;
}

export interface ChatRunPausedContext {
  elapsedTime?: number;
  workflowRunId?: string;
  nodeIds?: string[];
  status?: 'pending_approval' | 'pending_question';
  nodeType?: string;
}

export interface ChatRunCallbacks {
  onWorkflowStarted: (ctx: ChatRunStartedContext) => void;
  onTextChunk: (text: string) => void;
  onTextReplace?: () => void;
  onNodeStarted: (node: NodeInfo) => void;
  onNodeFinished: (node: NodeInfo) => void;
  onMessage?: (meta?: Record<string, unknown>) => void;
  onMessageEnd: (messageInfo: MessageEndEvent) => void;
  onWorkflowPaused?: (ctx: ChatRunPausedContext) => void;
  onError: (error: string) => void;
  onWorkflowFinished: (ctx: ChatRunFinishedContext) => void;
}

// Utility to guarantee non-empty strings when mapping to null for props
export function normalizeBackendId(id: string): string | null {
  const t = typeof id === 'string' ? id.trim() : '';
  return t.length > 0 ? t : null;
}

export function normalizeMessageRunStatus(status: unknown): RunStatus | null {
  if (typeof status !== 'string') return null;
  const normalized = status.trim().toLowerCase();
  switch (normalized) {
    case 'running':
    case 'in_progress':
    case 'in-progress':
      return 'running';
    case 'pending_approval':
    case 'pending-approval':
    case 'paused':
      return 'pending_approval';
    case 'pending_question':
    case 'pending-question':
    case 'question_answer_required':
      return 'pending_question';
    case 'completed':
    case 'success':
    case 'succeeded':
    case 'partial-succeeded':
      return 'completed';
    case 'error':
    case 'failed':
      return 'error';
    case 'stopped':
      return 'stopped';
    case 'expired':
      return 'expired';
    default:
      return null;
  }
}

export function isMessageRunTerminalStatus(status: RunStatus | null | undefined): boolean {
  return (
    status === 'completed' || status === 'error' || status === 'stopped' || status === 'expired'
  );
}
