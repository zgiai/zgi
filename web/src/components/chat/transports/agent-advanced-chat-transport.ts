import type {
  ConversationTransport,
  ConversationSummary,
  ConversationDetail,
  Pagination,
  SendMessagePayload,
  ChatRunCallbacks,
} from '@/components/chat/controllers/types';
import { agentService } from '@/services/agent.service';
import type { NodeInfo } from '@/components/chat/types';
import { normalizeMessageRunStatus } from '@/components/chat/types';
import {
  extractLlmGatewayRequest,
  getWorkflowRunRoundDurationMap,
} from '@/utils/workflow/run-events';

/**
 * AgentAdvancedChatTransport
 * Transport for agent advanced-chat workflow SSE API.
 * Uses agent_id instead of version_uuid/web_app_id.
 */
export class AgentAdvancedChatTransport implements ConversationTransport {
  constructor(private agentId: string) {}

  // List not supported for agent advanced-chat; return empty
  async list(_params: {
    page: number;
    limit: number;
  }): Promise<{ items: ConversationSummary[]; pagination: Pagination }> {
    return {
      items: [],
      pagination: { page: _params.page, limit: _params.limit, total: 0, hasMore: false },
    };
  }

  // Get not supported; throw error
  async get(_conversationId: string): Promise<ConversationDetail> {
    throw new Error('AgentAdvancedChatTransport does not support get conversation');
  }

  // Create a client-side draft conversation
  async create(payload?: { title?: string }): Promise<ConversationSummary> {
    const draft: ConversationSummary = {
      id: `draft-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
      conversationId: '',
      title: payload?.title ?? '',
      dialogueCount: 0,
      updatedAt: Date.now(),
      status: 'draft',
    };
    return draft;
  }

  // Remove not supported; resolve for drafts
  async remove(_conversationId: string): Promise<void> {
    // Draft conversation: nothing to delete on server
    return Promise.resolve();
  }

  send(payload: SendMessagePayload, callbacks: ChatRunCallbacks, abortSignal?: AbortSignal): void {
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
        nodeType,
        title,
        data: {
          input: inputs,
          output: finished ? outputs : undefined,
          modelInput,
        },
      };
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

    agentService.sseAdvancedChatRun(
      this.agentId,
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
          const execMeta =
            source['execution_metadata'] && typeof source['execution_metadata'] === 'object'
              ? (source['execution_metadata'] as Record<string, unknown>)
              : undefined;
          const loopId =
            (typeof source['loop_id'] === 'string' ? (source['loop_id'] as string) : undefined) ??
            (typeof execMeta?.['loop_id'] === 'string'
              ? (execMeta['loop_id'] as string)
              : undefined);
          const loopIndex =
            (typeof source['loop_index'] === 'number'
              ? (source['loop_index'] as number)
              : undefined) ??
            (typeof execMeta?.['loop_index'] === 'number'
              ? (execMeta['loop_index'] as number)
              : undefined);
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
              const childKey = (mapped.nodeId ?? '') || `${mapped.nodeType}|${mapped.title}`;
              const cIdx = round.nodes.findIndex(
                c => ((c.nodeId ?? '') || `${c.nodeType}|${c.title}`) === childKey
              );
              const child: NodeInfo = {
                status: 'running',
                nodeId: mapped.nodeId,
                nodeType: mapped.nodeType,
                title: mapped.title,
                data: { input: mapped.data?.input },
              };
              if (cIdx >= 0) round.nodes[cIdx] = { ...round.nodes[cIdx], ...child };
              else round.nodes.push(child);
              sess.activeIndex = targetIndex;
              loopSessions.set(key, { ...sess });
              activeLoop = { nodeId: key, index: targetIndex };
              callbacks.onNodeStarted?.({
                status: 'running',
                nodeId: sess.nodeId,
                nodeType: 'loop',
                title: sess.title,
                loopInputs: sess.inputs,
                loopRounds: sess.rounds.map(r => ({ index: r.index, nodes: r.nodes })),
              });
              return;
            }
          }
          if (callbacks.onNodeStarted) callbacks.onNodeStarted(mapped);
        },
        onNodeFinished: (node: unknown) => {
          const source = unwrap(node);
          const mapped = mapNode(node, true);
          const execMeta =
            source['execution_metadata'] && typeof source['execution_metadata'] === 'object'
              ? (source['execution_metadata'] as Record<string, unknown>)
              : undefined;
          const loopId =
            (typeof source['loop_id'] === 'string' ? (source['loop_id'] as string) : undefined) ??
            (typeof execMeta?.['loop_id'] === 'string'
              ? (execMeta['loop_id'] as string)
              : undefined);
          const loopIndex =
            (typeof source['loop_index'] === 'number'
              ? (source['loop_index'] as number)
              : undefined) ??
            (typeof execMeta?.['loop_index'] === 'number'
              ? (execMeta['loop_index'] as number)
              : undefined);
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
              const childKey = (mapped.nodeId ?? '') || `${mapped.nodeType}|${mapped.title}`;
              const cIdx = round.nodes.findIndex(
                c => ((c.nodeId ?? '') || `${c.nodeType}|${c.title}`) === childKey
              );
              const child: NodeInfo = {
                status: mapped.status,
                nodeId: mapped.nodeId,
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
              if (cIdx >= 0) round.nodes[cIdx] = { ...round.nodes[cIdx], ...child };
              else round.nodes.push(child);
              sess.activeIndex = targetIndex;
              loopSessions.set(key, { ...sess });
              activeLoop = { nodeId: key, index: targetIndex };
              callbacks.onNodeFinished?.({
                status: 'running',
                nodeId: sess.nodeId,
                nodeType: 'loop',
                title: sess.title,
                loopInputs: sess.inputs,
                loopRounds: sess.rounds.map(r => ({ index: r.index, nodes: r.nodes })),
              });
              return;
            }
          }
          if (callbacks.onNodeFinished) callbacks.onNodeFinished(mapped);
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
          const normalizedStatus = normalizeMessageRunStatus(data.status);
          let err: string | undefined;
          if (typeof data.error === 'string') err = data.error;
          else if (data.error && typeof data.error === 'object') {
            const m = (data.error as { message?: unknown }).message;
            err = typeof m === 'string' ? m : undefined;
          }
          callbacks.onFinished({
            status: normalizedStatus === 'completed' ? 'completed' : 'error',
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
            loopRounds: sess.rounds.map(r => ({ index: r.index, nodes: r.nodes })),
          });
        },
        onLoopCompleted: (node: unknown) => {
          const p = unwrap(node);
          const nodeId = typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined;
          const nodeType = typeof p['node_type'] === 'string' ? (p['node_type'] as string) : 'loop';
          const title = typeof p['title'] === 'string' ? (p['title'] as string) : nodeType;
          const elapsed =
            typeof p['elapsed_time'] === 'number' ? (p['elapsed_time'] as number) : undefined;
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
          const roundDurations = getWorkflowRunRoundDurationMap(p, 'loop');
          const key = nodeId ?? title;
          const sess = loopSessions.get(key) ?? { nodeId, nodeType, title, rounds: [] };
          sess.elapsedTime = elapsed;
          sess.error = error;
          sess.outputs = outputs;
          if (roundDurations.size > 0 || variableMap) {
            sess.rounds = sess.rounds.map(r => {
              const variables = variableMap?.[String(r.index)];
              return {
                ...r,
                elapsedTime: roundDurations.get(r.index) ?? r.elapsedTime,
                variables: variables ?? r.variables,
              };
            });
          }
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
            loopRounds: sess.rounds.map(r => ({
              index: r.index,
              nodes: r.nodes,
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

export default AgentAdvancedChatTransport;
