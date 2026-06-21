import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type { WorkflowDraftData, WorkflowDraftSavePayload } from '@/components/workflow/store/type';
import type {
  WorkflowRunList,
  WorkflowRunDetail,
  WorkflowNodeExecution,
  WorkflowRunsQuery,
  WorkflowChatMessagesList,
  WorkflowChatMessagesQuery,
  WorkflowLatestVersion,
  WorkflowPublishedVersionsResponse,
  PublishWorkflowResult,
  BuiltInWorkflowList,
  StopWorkflowTaskResponse,
  WorkflowExportVersion,
  WorkflowImportResult,
  WorkflowPrecheckResult,
  WorkflowNodeRunRequest,
  WorkflowNodeRunResponse,
  BuiltInWorkflowRuntimeSurfaceAuthorizationResponse,
  UpdatePublishedRuntimeSurfacesRequest,
} from './types/workflow';
import type { ChatAttachment } from '@/components/chat/types';
import { sanitizeModelOutputValue, wrapModelOutputSseCallbacks } from '@/utils/model-output-filter';

// Validation result type for workflow validation API
export interface WorkflowValidationResult {
  valid: boolean;
  errors: Array<{ nodeId?: string; message: string }>; // optional node-scoped errors
  warnings?: string[];
}

// Execution record type for workflow run history
export interface WorkflowExecutionRecord {
  id: string;
  status: 'queued' | 'running' | 'completed' | 'failed';
  startTime: string;
  endTime?: string;
  duration?: number;
  executedNodes?: Array<{ id: string; title?: string; status?: 'completed' | 'failed' }>; // optional node info
  errorMessage?: string;
}

// Request payload for starting a workflow execution
export interface WorkflowRunInputValues {
  [key: string]: unknown;
}

export interface WorkflowRunRequest {
  inputs?: WorkflowRunInputValues;
}

interface WorkflowDraftRunBody extends WorkflowRunRequest {
  response_mode: 'streaming';
}

interface WorkflowChatDraftRunRequest {
  query: string;
  conversation_id?: string;
  history_window_size?: number;
  files?: ChatAttachment[];
  inputs?: Record<string, unknown>;
}

interface WorkflowChatDraftRunBody extends WorkflowChatDraftRunRequest {
  response_mode: 'streaming';
}

export interface SuggestedQuestionCandidate {
  text: string;
  reason?: string;
}

export interface GenerateWorkflowSuggestedQuestionsPayload {
  locale?: string;
  count?: number;
  provider?: string;
  model?: string;
  graph?: WorkflowDraftSavePayload['graph'];
  features?: WorkflowDraftSavePayload['features'];
  existing_questions?: string[];
}

export interface GenerateWorkflowSuggestedQuestionsResult {
  questions: SuggestedQuestionCandidate[];
  warnings?: string[];
  provider?: string;
  model?: string;
}

function sanitizeWorkflowRunDetailResponse(
  response: ApiResponseData<WorkflowRunDetail>
): ApiResponseData<WorkflowRunDetail> {
  return {
    ...response,
    data: {
      ...response.data,
      outputs: sanitizeModelOutputValue(response.data.outputs),
      steps: response.data.steps?.map(step => ({
        ...step,
        outputs: sanitizeModelOutputValue(step.outputs),
      })),
    },
  };
}

function sanitizeWorkflowNodeExecutionsResponse(
  response: ApiResponseData<{ data: WorkflowNodeExecution[] }>
): ApiResponseData<{ data: WorkflowNodeExecution[] }> {
  return {
    ...response,
    data: {
      ...response.data,
      data: response.data.data.map(item => ({
        ...item,
        outputs: sanitizeModelOutputValue(item.outputs),
      })),
    },
  };
}

function sanitizeWorkflowChatMessagesResponse(
  response: ApiResponseData<WorkflowChatMessagesList>
): ApiResponseData<WorkflowChatMessagesList> {
  return {
    ...response,
    data: {
      ...response.data,
      data: response.data.data.map(item => ({
        ...item,
        answer: sanitizeModelOutputValue(item.answer) as string,
      })),
    },
  };
}

function buildWorkflowDraftRunBody(payload: WorkflowRunRequest): WorkflowDraftRunBody {
  return {
    inputs: payload.inputs,
    response_mode: 'streaming',
  };
}

function buildWorkflowChatDraftRunBody(
  payload: WorkflowChatDraftRunRequest
): WorkflowChatDraftRunBody {
  return {
    query: payload.query,
    response_mode: 'streaming',
    conversation_id: payload.conversation_id,
    history_window_size: payload.history_window_size,
    files: payload.files,
    inputs: payload.inputs,
  };
}

// Callbacks for workflow streaming events (payload shapes will be refined later)
export interface WorkflowRunSseCallbacks {
  /** Fired when the workflow run is started */
  onWorkflowStarted?: (payload: unknown) => void;
  /** Fired when the workflow run pauses for human approval */
  onWorkflowPaused?: (payload: unknown) => void;
  /** Fired when an approval form should be rendered */
  onApprovalRequested?: (payload: unknown) => void;
  /** Fired when a reviewer has submitted an approval result */
  onApprovalResultFilled?: (payload: unknown) => void;
  /** Fired when an approval form expires */
  onApprovalExpired?: (payload: unknown) => void;
  /** Fired when a question-answer prompt should be rendered */
  onQuestionAnswerRequested?: (payload: unknown) => void;
  /** Fired when a question-answer response has been submitted */
  onQuestionAnswerSubmitted?: (payload: unknown) => void;
  /** Fired when the workflow run finishes successfully; caller should refresh variables and invalidate last-run status */
  onWorkflowFinished?: (payload: unknown) => void;
  /** Fired when the workflow run fails */
  onError?: (payload: unknown) => void;
  /** Fired when a node starts */
  onNodeStarted?: (payload: unknown) => void;
  /** Fired when a node finishes */
  onNodeFinished?: (payload: unknown) => void;
  /** Fired when a node is retried */
  onNodeRetry?: (payload: unknown) => void;
  /** Fired when agent logs are updated */
  onAgentLog?: (payload: unknown) => void;
  /** Fired when a text chunk arrives */
  onTextChunk?: (payload: unknown) => void;
  /** Fired when a text content should be replaced */
  onTextReplace?: (payload: unknown) => void;
  /** Fired on generic message event */
  onMessage?: (payload: unknown) => void;
  /** Fired when a message ends */
  onMessageEnd?: (payload: unknown) => void;
  /** Fired when an iteration container starts */
  onIterationStarted?: (payload: unknown) => void;
  /** Fired when entering next iteration round */
  onIterationNext?: (payload: unknown) => void;
  /** Fired when an iteration container completes */
  onIterationCompleted?: (payload: unknown) => void;
  /** Fired when a loop container starts */
  onLoopStarted?: (payload: unknown) => void;
  /** Fired when entering next loop round */
  onLoopNext?: (payload: unknown) => void;
  /** Fired when a loop container completes */
  onLoopCompleted?: (payload: unknown) => void;
}

function getWorkflowSseEventName(envelope: unknown, fallbackEvent?: string | null): string {
  const obj =
    typeof envelope === 'object' && envelope !== null ? (envelope as Record<string, unknown>) : {};
  const evt = obj.event;
  return (typeof evt === 'string' && evt) || fallbackEvent || '';
}

function withTerminalStatus(envelope: unknown, status: string): unknown {
  const record =
    envelope && typeof envelope === 'object' ? (envelope as Record<string, unknown>) : {};
  const nested = record.data;
  if (nested && typeof nested === 'object') {
    return {
      ...record,
      data: {
        status,
        ...(nested as Record<string, unknown>),
      },
    };
  }

  return { status, ...record };
}

function dispatchWorkflowRunEvent(
  envelope: unknown,
  fallbackEvent: string | null | undefined,
  callbacks: WorkflowRunSseCallbacks
): void {
  const event = getWorkflowSseEventName(envelope, fallbackEvent);
  const terminalStatusByEvent: Record<string, string | undefined> = {
    workflow_stopped: 'stopped',
    workflow_failed: 'failed',
    workflow_succeeded: 'succeeded',
    workflow_completed: 'succeeded',
  };
  const terminalStatus = terminalStatusByEvent[event];
  const payload = terminalStatus ? withTerminalStatus(envelope, terminalStatus) : envelope;
  const handlers: Record<string, ((payload: unknown) => void) | undefined> = {
    workflow_started: callbacks.onWorkflowStarted,
    workflow_paused: callbacks.onWorkflowPaused,
    approval_requested: callbacks.onApprovalRequested,
    approval_result_filled: callbacks.onApprovalResultFilled,
    approval_expired: callbacks.onApprovalExpired,
    question_answer_requested: callbacks.onQuestionAnswerRequested,
    question_answer_submitted: callbacks.onQuestionAnswerSubmitted,
    workflow_finished: callbacks.onWorkflowFinished,
    workflow_stopped: callbacks.onWorkflowFinished,
    workflow_failed: callbacks.onWorkflowFinished,
    workflow_succeeded: callbacks.onWorkflowFinished,
    workflow_completed: callbacks.onWorkflowFinished,
    error: callbacks.onError,
    node_started: callbacks.onNodeStarted,
    node_finished: callbacks.onNodeFinished,
    node_retry: callbacks.onNodeRetry,
    agent_log: callbacks.onAgentLog,
    text_chunk: callbacks.onTextChunk,
    text_replace: callbacks.onTextReplace,
    message: callbacks.onMessage,
    data: callbacks.onMessage,
    message_end: callbacks.onMessageEnd,
    iteration_started: callbacks.onIterationStarted,
    iteration_next: callbacks.onIterationNext,
    iteration_completed: callbacks.onIterationCompleted,
    loop_started: callbacks.onLoopStarted,
    loop_next: callbacks.onLoopNext,
    loop_completed: callbacks.onLoopCompleted,
  };

  handlers[event]?.(payload);
}

/**
 * Workflow service for managing workflow-related API operations
 */
export class WorkflowService extends BaseService {
  constructor() {
    super({
      endpoint: 'main',
      basePath: '/console/api',
    });
  }

  /**
   * Get workflow draft data for a specific agent
   * @param agentId - The agent ID
   * @returns Promise<WorkflowDraftData>
   */
  async getWorkflowDraft(agentId: string): Promise<WorkflowDraftData> {
    const response = await this.request<ApiResponseData<WorkflowDraftData>>(
      'get',
      `/agents/${agentId}/workflows/draft`
    );
    return response.data;
  }

  /**
   * Get workflow run history list for specific agent
   * GET /console/api/agents/{agentId}/workflow-runs
   */
  async getWorkflowRuns(
    agentId: string,
    params?: WorkflowRunsQuery
  ): Promise<ApiResponseData<WorkflowRunList>> {
    return this.request<ApiResponseData<WorkflowRunList>>(
      'get',
      `/agents/${agentId}/workflow-runs`,
      undefined,
      { params }
    );
  }

  /**
   * Save workflow draft data for a specific agent
   * @param agentId - The agent ID
   * @param workflowData - The workflow data to save
   * @returns Promise with minimal server returned info
   */
  async saveWorkflowDraft(
    agentId: string,
    payload: WorkflowDraftSavePayload
  ): Promise<{
    hash: string;
    result: string;
    updated_at: string;
  }> {
    const response = await this.request<
      ApiResponseData<{
        hash: string;
        result: string;
        updated_at: string;
      }>
    >('post', `/agents/${agentId}/workflows/draft`, payload);
    return response.data;
  }

  async generateWorkflowSuggestedQuestions(
    agentId: string,
    payload: GenerateWorkflowSuggestedQuestionsPayload
  ): Promise<GenerateWorkflowSuggestedQuestionsResult> {
    const response = await this.request<ApiResponseData<GenerateWorkflowSuggestedQuestionsResult>>(
      'post',
      `/agents/${agentId}/workflows/draft/suggested-questions/generate`,
      payload
    );
    return response.data;
  }

  async runWorkflowDraft(
    agentId: string,
    payload: WorkflowRunRequest
  ): Promise<{ close: () => void }> {
    const url = this.buildUrl(`/agents/${agentId}/workflows/draft/run`);
    return this.client.sse<unknown, WorkflowDraftRunBody>(url, {
      method: 'POST',
      body: buildWorkflowDraftRunBody(payload),
      onMessage: () => {
        // Intentionally left blank
      },
    });
  }

  async precheckWorkflowDraft(
    agentId: string,
    payload: WorkflowRunRequest
  ): Promise<WorkflowPrecheckResult> {
    const response = await this.request<ApiResponseData<WorkflowPrecheckResult>>(
      'post',
      `/agents/${agentId}/workflows/draft/precheck`,
      buildWorkflowDraftRunBody(payload)
    );

    return response.data;
  }

  async runDraftWorkflowNode(
    agentId: string,
    nodeId: string,
    payload: WorkflowNodeRunRequest
  ): Promise<WorkflowNodeRunResponse> {
    const response = await this.request<ApiResponseData<WorkflowNodeRunResponse>>(
      'post',
      `/agents/${agentId}/workflows/draft/nodes/${nodeId}/run`,
      payload
    );
    return response.data;
  }

  /**
   * Start a workflow draft run with SSE using POST and handle event callbacks internally.
   * Returns a handle with close() for canceling the stream.
   */
  ssePostRunWorkflowDraft(
    agentId: string,
    payload: WorkflowRunRequest,
    callbacks: WorkflowRunSseCallbacks,
    opts?: { abortSignal?: AbortSignal; onClose?: () => void }
  ): Promise<{ close: () => void }> {
    const url = this.buildUrl(`/agents/${agentId}/workflows/draft/run`);
    return this.client.ssePost<WorkflowDraftRunBody>(url, {
      body: buildWorkflowDraftRunBody(payload),
      callbacks: wrapModelOutputSseCallbacks(callbacks),
      abortSignal: opts?.abortSignal,
      onClose: opts?.onClose,
    });
  }

  sseWorkflowRunEvents(
    workflowRunId: string,
    callbacks: WorkflowRunSseCallbacks,
    opts?: {
      abortSignal?: AbortSignal;
      onClose?: () => void;
      params?: {
        after?: number;
        include_snapshot?: boolean;
        continue_on_pause?: boolean;
      };
    }
  ): Promise<{ close: () => void }> {
    const url = this.buildUrl(`/workflow-runs/${workflowRunId}/events`);
    const wrappedCallbacks = wrapModelOutputSseCallbacks(callbacks);
    return this.client.sse<unknown>(url, {
      query: opts?.params,
      abortSignal: opts?.abortSignal,
      onClose: opts?.onClose,
      onMessage: message => {
        dispatchWorkflowRunEvent(message.data, message.event, wrappedCallbacks);
      },
      onError: error => {
        wrappedCallbacks.onError?.({
          message: error.message,
          originalError: error,
        });
      },
    });
  }

  async getWorkflowRunDetail(
    agentId: string,
    runId: string
  ): Promise<ApiResponseData<WorkflowRunDetail>> {
    const response = await this.request<ApiResponseData<WorkflowRunDetail>>(
      'get',
      `/agents/${agentId}/workflow-runs/${runId}`
    );
    return sanitizeWorkflowRunDetailResponse(response);
  }
  /**
   * Get node execution records for a specific workflow run
   * GET /console/api/agents/{agentId}/workflow-runs/{runId}/node-executions
   */
  async getWorkflowRunNodeExecutions(
    agentId: string,
    runId: string
  ): Promise<ApiResponseData<{ data: WorkflowNodeExecution[] }>> {
    const response = await this.request<ApiResponseData<{ data: WorkflowNodeExecution[] }>>(
      'get',
      `/agents/${agentId}/workflow-runs/${runId}/node-executions`
    );
    return sanitizeWorkflowNodeExecutionsResponse(response);
  }

  /**
   * Get chat message history for a workflow conversation.
   * GET /console/api/agents/{agentId}/chat-messages
   */
  async getWorkflowChatMessages(
    agentId: string,
    params: WorkflowChatMessagesQuery
  ): Promise<ApiResponseData<WorkflowChatMessagesList>> {
    const response = await this.request<ApiResponseData<WorkflowChatMessagesList>>(
      'get',
      `/agents/${agentId}/chat-messages`,
      undefined,
      { params }
    );
    return sanitizeWorkflowChatMessagesResponse(response);
  }

  /**
   * Publish workflow for a specific agent
   * POST /console/api/agents/{agentId}/workflows/publish
   * No request body, returns ApiResponseData with latest version info
   */
  async publishWorkflow(agentId: string): Promise<ApiResponseData<PublishWorkflowResult>> {
    return this.request('post', `/agents/${agentId}/workflows/publish`);
  }

  /**
   * Get latest workflow version info by agent ID
   * GET /console/api/agents/{agentId}/workflows/latest-version
   */
  async getLatestWorkflowVersion(agentId: string): Promise<ApiResponseData<WorkflowLatestVersion>> {
    return this.request('get', `/agents/${agentId}/workflows/latest-version`);
  }

  async getPublishedWorkflowVersions(
    agentId: string
  ): Promise<ApiResponseData<WorkflowPublishedVersionsResponse>> {
    return this.request('get', `/agents/${agentId}/workflows/published-versions`);
  }

  async exportWorkflow(agentId: string, version: WorkflowExportVersion = 'draft'): Promise<Blob> {
    return this.request('get', `/agents/${agentId}/workflows/export`, undefined, {
      params: { version },
      responseType: 'blob',
    });
  }

  async importWorkflow(
    file: File,
    workspaceId?: string
  ): Promise<ApiResponseData<WorkflowImportResult>> {
    const formData = new FormData();
    formData.append('file', file);
    if (workspaceId) {
      formData.append('workspace_id', workspaceId);
    }
    return this.request('post', '/agents/workflows/import', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
  }

  /**
   * Start a chat draft run with SSE using POST and handle event callbacks.
   * Endpoint: /console/api/agents/{agentId}/advanced-chat/workflows/draft/run
   * Text chunks are delivered via onMessage events from server.
   */
  ssePostRunWorkflowChatDraft(
    agentId: string,
    payload: WorkflowChatDraftRunRequest,
    callbacks: WorkflowRunSseCallbacks,
    opts?: { abortSignal?: AbortSignal; onClose?: () => void }
  ): Promise<{ close: () => void }> {
    const url = this.buildUrl(`/agents/${agentId}/advanced-chat/workflows/draft/run`);
    return this.client.ssePost<WorkflowChatDraftRunBody>(url, {
      body: buildWorkflowChatDraftRunBody(payload),
      callbacks: wrapModelOutputSseCallbacks(callbacks),
      abortSignal: opts?.abortSignal,
      onClose: opts?.onClose,
    });
  }

  async precheckWorkflowChatDraft(
    agentId: string,
    payload: WorkflowChatDraftRunRequest
  ): Promise<WorkflowPrecheckResult> {
    const response = await this.request<ApiResponseData<WorkflowPrecheckResult>>(
      'post',
      `/agents/${agentId}/advanced-chat/workflows/draft/precheck`,
      buildWorkflowChatDraftRunBody(payload)
    );

    return response.data;
  }

  /**
   * Get built-in system workflows
   * GET /console/api/built-in-workflows
   */
  async getBuiltInWorkflows(): Promise<ApiResponseData<BuiltInWorkflowList>> {
    return this.request('get', '/built-in-workflows');
  }

  async getBuiltInWorkflowRuntimeSurfaces(
    scenario: string
  ): Promise<ApiResponseData<BuiltInWorkflowRuntimeSurfaceAuthorizationResponse>> {
    return this.request(
      'get',
      `/built-in-workflows/${encodeURIComponent(scenario)}/runtime-surfaces`
    );
  }

  async updateBuiltInWorkflowRuntimeSurfaces(
    scenario: string,
    payload: UpdatePublishedRuntimeSurfacesRequest
  ): Promise<ApiResponseData<BuiltInWorkflowRuntimeSurfaceAuthorizationResponse>> {
    return this.request(
      'patch',
      `/built-in-workflows/${encodeURIComponent(scenario)}/runtime-surfaces`,
      payload
    );
  }

  /**
   * Stop a running workflow task
   * POST /console/api/agents/{agent_id}/workflow-runs/tasks/{workflow_run_id}/stop
   */
  async stopWorkflowTask(
    agentId: string,
    workflowRunId: string
  ): Promise<ApiResponseData<StopWorkflowTaskResponse>> {
    return this.request('post', `/agents/${agentId}/workflow-runs/tasks/${workflowRunId}/stop`);
  }
}

export const workflowService = new WorkflowService();
export default workflowService;
