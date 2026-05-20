import type {
  ConversationTransport,
  ConversationSummary,
  ConversationDetail,
  Pagination,
  SendMessagePayload,
  ChatRunCallbacks,
} from '@/components/chat/controllers/types';
import { WebAppService } from '@/services/webapp.service';
import type {
  WebAppConversation,
  WebAppConversationDetail,
  WebAppConversationMessageItem,
} from '@/services/types/webapp';
import type { Message, NodeInfo, TerminalRunStatus } from '@/components/chat/types';
import { normalizeMessageRunStatus } from '@/components/chat/types';
import {
  extractLlmGatewayRequest,
  extractWorkflowRunContainerContext,
  getWorkflowRunCreatedAtMs,
  getWorkflowRunExecutionId,
  getWorkflowRunItemKey,
  getWorkflowRunRoundElapsedTime,
  sortWorkflowRunItems,
  sortWorkflowRunRounds,
} from '@/utils/workflow/run-events';

// Map WebAppConversation to ConversationSummary
function mapWebAppConversationToSummary(item: WebAppConversation): ConversationSummary {
  return {
    id: item.id,
    conversationId: item.id,
    title: item.name,
    dialogueCount: item.dialogue_count,
    updatedAt: item.updated_at * 1000, // Convert to ms
    status: item.status,
    metadata: {
      workflow_version_uuid: item.workflow_version_uuid,
      invoke_from: item.invoke_from,
      created_at: item.created_at,
    },
  };
}

// Map WebAppConversationMessageItem to Message
function mapWebAppMessageToMessage(item: WebAppConversationMessageItem): Message {
  const runStatus = normalizeMessageRunStatus(item.status);

  return {
    messageId: item.id,
    query: item.query,
    answer: item.answer,
    parentId: '',
    model: null,
    clientState: {
      phase: runStatus === 'running' ? 'streaming' : 'completed',
      status: runStatus && runStatus !== 'running' ? runStatus : undefined,
      finishedAt: item.created_at * 1000,
    },
    WorkflowRunInfo:
      item.workflow_run_id && runStatus
        ? {
            id: item.workflow_run_id,
            status: runStatus,
            runNodeInfo: [],
          }
        : undefined,
    messageData: {
      ...(item.workflow_run_id ? { tempKey: `restore:${item.workflow_run_id}` } : {}),
      workflow_run_id: item.workflow_run_id,
      message_id: item.id,
      created_at: item.created_at,
      status: item.status,
      inputs: item.inputs,
    },
  };
}

function normalizeFinalRunStatus(status: unknown): TerminalRunStatus {
  const normalized = normalizeMessageRunStatus(status);
  if (normalized === 'completed' || normalized === 'stopped' || normalized === 'expired') {
    return normalized;
  }
  return 'error';
}

// Map WebAppConversationDetail to ConversationDetail
function mapWebAppConversationDetailToDetail(data: WebAppConversationDetail): ConversationDetail {
  return {
    summary: {
      id: data.id,
      conversationId: data.id,
      title: data.name,
      dialogueCount: data.dialogue_count,
      updatedAt: data.updated_at * 1000,
      status: data.status,
      metadata: {
        agent_id: data.agent_id,
        mode: data.mode,
        workflow_version_uuid: data.workflow_version_uuid,
        invoke_from: data.invoke_from,
        created_at: data.created_at,
        inputs: data.inputs,
      },
    },
    messages: data.messages.map(mapWebAppMessageToMessage),
    loaded: true,
    loading: false,
  };
}

export class WebappConversationTransport implements ConversationTransport {
  private onTaskIdCallback?: (taskId: string) => void;

  constructor(private versionUuid: string) {}

  /** Set callback to receive task_id when workflow starts */
  setOnTaskId(callback: (taskId: string) => void): void {
    this.onTaskIdCallback = callback;
  }

  async list(params: {
    page: number;
    limit: number;
  }): Promise<{ items: ConversationSummary[]; pagination: Pagination }> {
    try {
      const response = await WebAppService.getConversations(this.versionUuid, params);
      const { data, has_more, limit, page, total } = response.data;

      return {
        items: data.map(mapWebAppConversationToSummary),
        pagination: {
          page,
          limit,
          total,
          hasMore: has_more,
        },
      };
    } catch (err) {
      console.error('[WebappTransport] Failed to list conversations:', err);
      // Return empty list on error; toast handled by hook if this were called from a hook
      return {
        items: [],
        pagination: { page: params.page, limit: params.limit, total: 0, hasMore: false },
      };
    }
  }

  async get(conversationId: string): Promise<ConversationDetail> {
    try {
      const response = await WebAppService.getConversation(this.versionUuid, conversationId);
      return mapWebAppConversationDetailToDetail(response.data);
    } catch (err) {
      console.error('[WebappTransport] Failed to get conversation:', err);
      throw err;
    }
  }

  async create(payload?: { title?: string }): Promise<ConversationSummary> {
    // Webapp does not have explicit create endpoint
    // Return a client-side draft with empty conversationId
    const draft: ConversationSummary = {
      id: `draft-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
      conversationId: '',
      title: payload?.title ?? `New conversation ${new Date().toLocaleString()}`,
      dialogueCount: 0,
      updatedAt: Date.now(),
      status: 'draft',
    };
    return draft;
  }

  async remove(_conversationId: string): Promise<void> {
    const conversationId = _conversationId?.trim();
    if (!conversationId) {
      // Draft conversation: nothing to delete on server
      return Promise.resolve();
    }
    try {
      await WebAppService.deleteConversation(this.versionUuid, conversationId);
    } catch (err) {
      console.error('[WebappTransport] Failed to delete conversation:', err);
      throw err;
    }
  }

  send(payload: SendMessagePayload, callbacks: ChatRunCallbacks, abortSignal?: AbortSignal): void {
    let receiveOrder = 0;
    const nextReceiveOrder = (): number => {
      receiveOrder += 1;
      return receiveOrder;
    };

    const unwrap = (payload: unknown): Record<string, unknown> => {
      const obj =
        typeof payload === 'object' && payload ? (payload as Record<string, unknown>) : {};
      const data = obj && 'data' in obj ? (obj['data'] as unknown) : undefined;
      return typeof data === 'object' && data ? (data as Record<string, unknown>) : obj;
    };

    const mapNode = (node: unknown, finished: boolean): NodeInfo => {
      const p = unwrap(node);
      const statusRaw = typeof p['status'] === 'string' ? (p['status'] as string) : undefined;
      let status: NodeInfo['status'];
      switch (statusRaw) {
        case 'failed':
          status = 'failed';
          break;
        case 'stopped':
          status = 'stopped';
          break;
        case 'success':
        case 'succeeded':
        case 'completed':
          status = 'success';
          break;
        default:
          status = finished ? 'success' : 'running';
      }

      const rawErr = p['error'];
      let error: string | undefined;
      if (typeof rawErr === 'string') {
        error = rawErr as string;
      } else if (rawErr && typeof rawErr === 'object') {
        const m = (rawErr as { message?: unknown }).message;
        error = typeof m === 'string' ? (m as string) : undefined;
      } else {
        error = undefined;
      }

      let nodeId: string | undefined;
      if (typeof p['node_id'] === 'string') nodeId = p['node_id'] as string;
      else if (typeof p['node_id'] === 'number') nodeId = String(p['node_id'] as number);
      else if (typeof p['execution_id'] === 'string') nodeId = p['execution_id'] as string;
      else if (typeof p['execution_id'] === 'number') nodeId = String(p['execution_id'] as number);
      else if (typeof p['id'] === 'string') nodeId = p['id'] as string;
      else if (typeof p['id'] === 'number') nodeId = String(p['id'] as number);
      else nodeId = undefined;

      const rawType =
        typeof p['node_type'] === 'string'
          ? (p['node_type'] as string)
          : typeof p['type'] === 'string'
            ? (p['type'] as string)
            : undefined;
      const hyphen = rawType ? rawType.replace(/_/g, '-').toLowerCase() : undefined;
      let nodeType: string | undefined;
      if (!hyphen) {
        nodeType = undefined;
      } else {
        switch (hyphen) {
          case 'database':
            nodeType = 'call-database';
            break;
          case 'if-else':
            nodeType = 'if-else';
            break;
          case 'http':
          case 'http-request':
            nodeType = 'http-request';
            break;
          case 'assign':
          case 'assigner':
            nodeType = 'assigner';
            break;
          case 'iterationstart':
          case 'iteration-start':
            nodeType = 'iteration-start';
            break;
          default:
            nodeType = hyphen;
        }
      }

      let title: string;
      if (typeof p['title'] === 'string') title = p['title'] as string;
      else if (typeof p['node_title'] === 'string') title = p['node_title'] as string;
      else if (typeof p['name'] === 'string') title = p['name'] as string;
      else if (typeof p['label'] === 'string') title = p['label'] as string;
      else title = (nodeType ?? nodeId ?? '') as string;

      const elapsedTime =
        typeof p['elapsed_time'] === 'number' ? (p['elapsed_time'] as number) : undefined;
      const inputs = p['inputs'];
      const outputs = p['outputs'];
      const modelInput = extractLlmGatewayRequest(p);
      return {
        status,
        error,
        elapsedTime,
        nodeId,
        executionId: getWorkflowRunExecutionId(p),
        createdAtMs: getWorkflowRunCreatedAtMs(p),
        nodeType,
        title,
        data: {
          input: inputs,
          output: finished ? outputs : undefined,
          modelInput,
        },
      };
    };

    // Iteration session state scoped to this send/run
    const iterationSessions = new Map<
      string,
      {
        nodeId?: string;
        nodeType?: string;
        title?: string;
        inputs?: unknown;
        outputs?: unknown;
        elapsedTime?: number;
        error?: string;
        rounds: Array<{ index: number; nodes: NodeInfo[]; elapsedTime?: number }>;
        activeIndex?: number | null;
      }
    >();
    let activeIteration: { nodeId: string | null; index: number | null } = {
      nodeId: null,
      index: null,
    };
    const loopSessions = new Map<
      string,
      {
        nodeId?: string;
        nodeType?: string;
        title?: string;
        inputs?: unknown;
        outputs?: unknown;
        elapsedTime?: number;
        error?: string;
        rounds: Array<{
          index: number;
          nodes: NodeInfo[];
          elapsedTime?: number;
          variables?: unknown;
        }>;
        activeIndex?: number | null;
      }
    >();
    let activeLoop: { nodeId: string | null; index: number | null } = {
      nodeId: null,
      index: null,
    };

    WebAppService.ssePostRun(
      this.versionUuid,
      {
        query: payload.query,
        conversation_id: payload.conversationId,
        history_window_size: payload.historyWindowSize,
        files: payload.files,
        inputs: payload.inputs,
      },
      {
        onWorkflowStarted: (ctx: unknown) => {
          const data = unwrap(ctx) as {
            conversation_id?: string;
            message_id?: string;
            tempKey?: string;
            task_id?: string;
            id?: string;
            workflow_run_id?: string;
          };
          // Extract task_id for stop functionality
          const taskId = data.task_id || data.id || data.workflow_run_id;
          if (taskId && this.onTaskIdCallback) {
            this.onTaskIdCallback(taskId);
          }
          callbacks.onStarted({
            conversationId: data.conversation_id ?? '',
            messageId: data.message_id,
            workflowRunId: data.id ?? data.workflow_run_id ?? data.task_id,
            tempKey: data.tempKey,
          });
        },
        onTextChunk: (token: unknown) => {
          let s = '';
          if (typeof token === 'string') {
            s = token;
          } else if (token && typeof token === 'object') {
            const t = token as Record<string, unknown>;
            if (typeof t['text'] === 'string') s = t['text'] as string;
            else if (typeof t['answer'] === 'string') s = t['answer'] as string;
            else if (typeof t['delta'] === 'string') s = t['delta'] as string;
          } else {
            s = String(token ?? '');
          }
          callbacks.onToken(s);
        },
        onTextReplace: () => {
          callbacks.onTextReplace?.();
        },
        onNodeStarted: (node: unknown) => {
          const source = unwrap(node);
          const mapped = mapNode(node, false);
          const { loopId, loopIndex, iterationId, iterationIndex } =
            extractWorkflowRunContainerContext(source);
          mapped.receivedOrder = nextReceiveOrder();
          if (loopId) {
            const key = loopId;
            const sess = loopSessions.get(key);
            const targetIndex =
              typeof loopIndex === 'number' ? loopIndex : (sess?.activeIndex ?? activeLoop.index);
            if (sess && typeof targetIndex === 'number') {
              const rIdx = sess.rounds.findIndex(r => r.index === targetIndex);
              if (rIdx < 0) sess.rounds.push({ index: targetIndex, nodes: [] });
              const round = sess.rounds.find(r => r.index === targetIndex);
              if (!round) return;
              const childKey = getWorkflowRunItemKey(mapped);
              const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
              const child: NodeInfo = {
                status: 'running',
                nodeId: mapped.nodeId,
                executionId: mapped.executionId,
                createdAtMs: mapped.createdAtMs,
                receivedOrder: mapped.receivedOrder,
                nodeType: mapped.nodeType,
                title: mapped.title,
                data: { input: mapped.data?.input },
              };
              if (cIdx >= 0) {
                const existing = round.nodes[cIdx];
                round.nodes[cIdx] = {
                  ...existing,
                  ...child,
                  createdAtMs: existing.createdAtMs ?? child.createdAtMs,
                  receivedOrder: existing.receivedOrder ?? child.receivedOrder,
                };
              } else round.nodes.push(child);
              sess.activeIndex = targetIndex;
              loopSessions.set(key, { ...sess });
              activeLoop = { nodeId: key, index: targetIndex };
              callbacks.onNodeStarted?.({
                status: 'running',
                nodeId: sess.nodeId,
                nodeType: 'loop',
                title: sess.title,
                loopInputs: sess.inputs,
                loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                  index: r.index,
                  nodes: sortWorkflowRunItems(r.nodes),
                })),
              });
              return;
            }
          }
          const targetIterationId = iterationId ?? activeIteration.nodeId;
          const targetIterationIndex =
            typeof iterationIndex === 'number' ? iterationIndex : activeIteration.index;
          if (targetIterationId && targetIterationIndex !== null) {
            const key = targetIterationId;
            const sess = iterationSessions.get(key);
            if (sess) {
              const rIdx = sess.rounds.findIndex(r => r.index === targetIterationIndex);
              if (rIdx < 0) sess.rounds.push({ index: targetIterationIndex, nodes: [] });
              const round = sess.rounds.find(r => r.index === targetIterationIndex);
              if (!round) return;
              const childKey = getWorkflowRunItemKey(mapped);
              const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
              const child: NodeInfo = {
                status: 'running',
                nodeId: mapped.nodeId,
                executionId: mapped.executionId,
                createdAtMs: mapped.createdAtMs,
                receivedOrder: mapped.receivedOrder,
                nodeType: mapped.nodeType,
                title: mapped.title,
                data: { input: mapped.data?.input },
              };
              if (cIdx >= 0) {
                const existing = round.nodes[cIdx];
                round.nodes[cIdx] = {
                  ...existing,
                  ...child,
                  createdAtMs: existing.createdAtMs ?? child.createdAtMs,
                  receivedOrder: existing.receivedOrder ?? child.receivedOrder,
                };
              } else round.nodes.push(child);
              sess.activeIndex = targetIterationIndex;
              iterationSessions.set(key, { ...sess });
              callbacks.onNodeStarted?.({
                status: 'running',
                nodeId: sess.nodeId,
                nodeType: 'iteration',
                title: sess.title,
                iterationInputs: sess.inputs,
                iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                  index: r.index,
                  nodes: sortWorkflowRunItems(r.nodes),
                })),
              });
              return;
            }
          }
          callbacks.onNodeStarted?.(mapped);
        },
        onNodeFinished: (node: unknown) => {
          const source = unwrap(node);
          const mapped = mapNode(node, true);
          const { loopId, loopIndex, iterationId, iterationIndex } =
            extractWorkflowRunContainerContext(source);
          mapped.receivedOrder = nextReceiveOrder();
          if (loopId) {
            const key = loopId;
            const sess = loopSessions.get(key);
            const targetIndex =
              typeof loopIndex === 'number' ? loopIndex : (sess?.activeIndex ?? activeLoop.index);
            if (sess && typeof targetIndex === 'number') {
              const rIdx = sess.rounds.findIndex(r => r.index === targetIndex);
              if (rIdx < 0) sess.rounds.push({ index: targetIndex, nodes: [] });
              const round = sess.rounds.find(r => r.index === targetIndex);
              if (!round) return;
              const childKey = getWorkflowRunItemKey(mapped);
              const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
              const child: NodeInfo = {
                status: mapped.status,
                nodeId: mapped.nodeId,
                executionId: mapped.executionId,
                createdAtMs: mapped.createdAtMs,
                receivedOrder: mapped.receivedOrder,
                nodeType: mapped.nodeType,
                title: mapped.title,
                elapsedTime: mapped.elapsedTime,
                error: mapped.error,
                data: {
                  input: mapped.data?.input,
                  output: mapped.data?.output,
                  modelInput: mapped.data?.modelInput,
                },
              };
              if (cIdx >= 0) {
                const existing = round.nodes[cIdx];
                round.nodes[cIdx] = {
                  ...existing,
                  ...child,
                  createdAtMs: existing.createdAtMs ?? child.createdAtMs,
                  receivedOrder: existing.receivedOrder ?? child.receivedOrder,
                };
              } else round.nodes.push(child);
              sess.activeIndex = targetIndex;
              loopSessions.set(key, { ...sess });
              activeLoop = { nodeId: key, index: targetIndex };
              callbacks.onNodeFinished?.({
                status: 'running',
                nodeId: sess.nodeId,
                nodeType: 'loop',
                title: sess.title,
                loopInputs: sess.inputs,
                loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                  index: r.index,
                  nodes: sortWorkflowRunItems(r.nodes),
                })),
              });
              return;
            }
          }
          const targetIterationId = iterationId ?? activeIteration.nodeId;
          const targetIterationIndex =
            typeof iterationIndex === 'number' ? iterationIndex : activeIteration.index;
          if (targetIterationId && targetIterationIndex !== null) {
            const key = targetIterationId;
            const sess = iterationSessions.get(key);
            if (sess) {
              const rIdx = sess.rounds.findIndex(r => r.index === targetIterationIndex);
              if (rIdx < 0) sess.rounds.push({ index: targetIterationIndex, nodes: [] });
              const round = sess.rounds.find(r => r.index === targetIterationIndex);
              if (!round) return;
              const childKey = getWorkflowRunItemKey(mapped);
              const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
              const child: NodeInfo = {
                status: mapped.status,
                nodeId: mapped.nodeId,
                executionId: mapped.executionId,
                createdAtMs: mapped.createdAtMs,
                receivedOrder: mapped.receivedOrder,
                nodeType: mapped.nodeType,
                title: mapped.title,
                elapsedTime: mapped.elapsedTime,
                error: mapped.error,
                data: {
                  input: mapped.data?.input,
                  output: mapped.data?.output,
                  modelInput: mapped.data?.modelInput,
                },
              };
              if (cIdx >= 0) {
                const existing = round.nodes[cIdx];
                round.nodes[cIdx] = {
                  ...existing,
                  ...child,
                  createdAtMs: existing.createdAtMs ?? child.createdAtMs,
                  receivedOrder: existing.receivedOrder ?? child.receivedOrder,
                };
              } else round.nodes.push(child);
              sess.activeIndex = targetIterationIndex;
              iterationSessions.set(key, { ...sess });
              callbacks.onNodeFinished?.({
                status: 'running',
                nodeId: sess.nodeId,
                nodeType: 'iteration',
                title: sess.title,
                iterationInputs: sess.inputs,
                iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                  index: r.index,
                  nodes: sortWorkflowRunItems(r.nodes),
                })),
              });
              return;
            }
          }
          callbacks.onNodeFinished?.(mapped);
        },
        onMessage: (meta: unknown) => {
          callbacks.onMessage(unwrap(meta));
        },
        onMessageEnd: (meta: unknown) => {
          if (callbacks.onMessageEnd) callbacks.onMessageEnd(unwrap(meta));
        },
        onWorkflowFinished: (ctx: unknown) => {
          const data = unwrap(ctx) as {
            id?: string;
            workflow_run_id?: string;
            status?: string;
            error?: unknown;
            elapsed_time?: number;
            message_id?: string;
          };
          const s = typeof data.status === 'string' ? data.status.toLowerCase() : '';
          const status = normalizeFinalRunStatus(s);
          let err: string | undefined;
          if (typeof data.error === 'string') err = data.error;
          else if (data.error && typeof data.error === 'object') {
            const m = (data.error as { message?: unknown }).message;
            err = typeof m === 'string' ? m : undefined;
          }
          callbacks.onFinished({
            status,
            error: err,
            elapsedTime: data.elapsed_time,
            messageId: data.message_id,
            workflowRunId:
              (typeof data.id === 'string' ? data.id : '') ||
              (typeof data.workflow_run_id === 'string' ? data.workflow_run_id : '') ||
              undefined,
            model: null,
          });
        },
        onIterationStarted: (node: unknown) => {
          const p = unwrap(node);
          const nodeId = typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined;
          const nodeType =
            typeof p['node_type'] === 'string' ? (p['node_type'] as string) : 'iteration';
          const title = typeof p['title'] === 'string' ? (p['title'] as string) : nodeType;
          const inputs = p['inputs'];
          const key = nodeId ?? title;
          iterationSessions.set(key, {
            nodeId,
            nodeType,
            title,
            inputs,
            rounds: [],
            activeIndex: null,
          });
          activeIteration = { nodeId: key, index: null };
          callbacks.onNodeStarted?.({
            status: 'running',
            nodeId,
            nodeType: 'iteration',
            title,
            iterationInputs: inputs,
            iterationRounds: [],
          });
        },
        onIterationNext: (node: unknown) => {
          const p = unwrap(node);
          const nodeId = typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined;
          const nodeType =
            typeof p['node_type'] === 'string' ? (p['node_type'] as string) : 'iteration';
          const title = typeof p['title'] === 'string' ? (p['title'] as string) : nodeType;
          const index = typeof p['index'] === 'number' ? (p['index'] as number) : 0;
          const key = nodeId ?? title;
          const sess = iterationSessions.get(key) ?? { nodeId, nodeType, title, rounds: [] };
          const hasRound = sess.rounds.some(r => r.index === index);
          if (!hasRound) sess.rounds.push({ index, nodes: [] });
          sess.activeIndex = index;
          iterationSessions.set(key, sess);
          activeIteration = { nodeId: key, index };
          callbacks.onNodeStarted?.({
            status: 'running',
            nodeId,
            nodeType: 'iteration',
            title,
            iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
              index: r.index,
              nodes: sortWorkflowRunItems(r.nodes),
            })),
          });
        },
        onIterationCompleted: (node: unknown) => {
          const p = unwrap(node);
          const nodeId = typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined;
          const nodeType =
            typeof p['node_type'] === 'string' ? (p['node_type'] as string) : 'iteration';
          const title = typeof p['title'] === 'string' ? (p['title'] as string) : nodeType;
          const elapsed = typeof p['elapsed_time'] === 'number' ? (p['elapsed_time'] as number) : 0;
          const error = typeof p['error'] === 'string' ? (p['error'] as string) : undefined;
          const outputs = p['outputs'];
          const key = nodeId ?? title;
          const sess = iterationSessions.get(key) ?? { nodeId, nodeType, title, rounds: [] };
          sess.elapsedTime = elapsed;
          sess.error = error;
          sess.outputs = outputs;
          sess.rounds = sess.rounds.map(r => ({
            ...r,
            elapsedTime: getWorkflowRunRoundElapsedTime(r),
          }));
          iterationSessions.set(key, sess);
          activeIteration = { nodeId: null, index: null };
          callbacks.onNodeFinished?.({
            status: error ? 'failed' : 'success',
            nodeId,
            nodeType: 'iteration',
            title,
            elapsedTime: elapsed,
            error,
            iterationOutputs: outputs,
            iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
              index: r.index,
              nodes: sortWorkflowRunItems(r.nodes),
              elapsedTime: r.elapsedTime,
            })),
          });
        },
        onLoopStarted: (node: unknown) => {
          const p = unwrap(node);
          const nodeId = typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined;
          const nodeType = typeof p['node_type'] === 'string' ? (p['node_type'] as string) : 'loop';
          const title = typeof p['title'] === 'string' ? (p['title'] as string) : nodeType;
          const inputs = p['inputs'];
          const key = nodeId ?? title;
          loopSessions.set(key, {
            nodeId,
            nodeType,
            title,
            inputs,
            rounds: [],
            activeIndex: null,
          });
          activeLoop = { nodeId: key, index: null };
          callbacks.onNodeStarted?.({
            status: 'running',
            nodeId,
            nodeType: 'loop',
            title,
            loopInputs: inputs,
            loopRounds: [],
          });
        },
        onLoopNext: (node: unknown) => {
          const p = unwrap(node);
          const nodeId = typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined;
          const nodeType = typeof p['node_type'] === 'string' ? (p['node_type'] as string) : 'loop';
          const title = typeof p['title'] === 'string' ? (p['title'] as string) : nodeType;
          const index = typeof p['index'] === 'number' ? (p['index'] as number) : 0;
          const key = nodeId ?? title;
          const sess = loopSessions.get(key) ?? { nodeId, nodeType, title, rounds: [] };
          const hasRound = sess.rounds.some(r => r.index === index);
          if (!hasRound) sess.rounds.push({ index, nodes: [] });
          sess.activeIndex = index;
          loopSessions.set(key, sess);
          activeLoop = { nodeId: key, index };
          callbacks.onNodeStarted?.({
            status: 'running',
            nodeId,
            nodeType: 'loop',
            title,
            loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
              index: r.index,
              nodes: sortWorkflowRunItems(r.nodes),
            })),
          });
        },
        onLoopCompleted: (node: unknown) => {
          const p = unwrap(node);
          const nodeId = typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined;
          const nodeType = typeof p['node_type'] === 'string' ? (p['node_type'] as string) : 'loop';
          const title = typeof p['title'] === 'string' ? (p['title'] as string) : nodeType;
          const elapsed = typeof p['elapsed_time'] === 'number' ? (p['elapsed_time'] as number) : 0;
          const status = typeof p['status'] === 'string' ? (p['status'] as string) : '';
          const isSuccess =
            status === 'success' || status === 'succeeded' || status === 'completed';
          const error = typeof p['error'] === 'string' ? (p['error'] as string) : undefined;
          const outputs = p['outputs'];
          const execMeta = p['execution_metadata'] as unknown;
          const variableMap: Record<string, unknown> | undefined =
            execMeta && typeof execMeta === 'object'
              ? ((execMeta as Record<string, unknown>)['loop_variable_map'] as
                  | Record<string, unknown>
                  | undefined)
              : undefined;
          const key = nodeId ?? title;
          const sess = loopSessions.get(key) ?? { nodeId, nodeType, title, rounds: [] };
          sess.elapsedTime = elapsed;
          sess.error = error;
          sess.outputs = outputs;
          sess.rounds = sess.rounds.map(r => {
            const variables = variableMap?.[String(r.index)];
            return {
              ...r,
              elapsedTime: getWorkflowRunRoundElapsedTime(r),
              variables: variables ?? r.variables,
            };
          });
          loopSessions.set(key, sess);
          activeLoop = { nodeId: null, index: null };
          callbacks.onNodeFinished?.({
            status: isSuccess ? 'success' : 'failed',
            nodeId,
            nodeType: 'loop',
            title,
            elapsedTime: elapsed,
            error,
            loopOutputs: outputs,
            loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
              index: r.index,
              nodes: sortWorkflowRunItems(r.nodes),
              elapsedTime: r.elapsedTime,
              variables: r.variables,
            })),
          });
        },
        onError: (err: unknown) => {
          callbacks.onError(new Error(String(err ?? 'Unknown error')));
        },
      },
      { abortSignal }
    );
  }
}

export default WebappConversationTransport;
