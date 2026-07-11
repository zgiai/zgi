import { create } from 'zustand';
import { createSelectors } from '@/store/utils/selectors';
import type {
  Conversation,
  Message,
  NodeInfo,
  RunStatus,
  TerminalRunStatus,
  GeneratedImage,
} from '@/components/chat/types';

interface ChatState {
  conversations: Record<string, Conversation>;
  currentId: string | null;

  // Conversation-level APIs
  initSingle: (conversation: Partial<Conversation> & { id: string }) => void;
  getConversations: () => Conversation[];
  getConversation: (id: string) => Conversation | undefined;
  setCurrent: (id: string) => void;
  updateConversation: (
    id: string,
    data: { conversationId?: string; title?: string; conversationData?: Record<string, unknown> }
  ) => void;
  migrateConversation: (oldId: string, newId: string) => void;
  deleteConversation: (id: string) => void;

  // Message streaming APIs (single message pairs)
  appendUserMessage: (
    id: string,
    payload: {
      query: string;
      parentId?: string;
      tempKey?: string;
      inputs?: Record<string, unknown>;
    }
  ) => { tempKey: string };
  updateMessageInputs: (id: string, tempKey: string, inputs: Record<string, unknown>) => void;
  ensureAiMessage: (id: string, tempKey: string) => void;
  appendAiChunk: (id: string, tempKey: string, chunk: string) => void;
  mergeAiMessage: (
    id: string,
    tempKey: string,
    data: {
      answer?: string;
      answerMode?: 'append' | 'replace';
      messageId?: string;
      workflowRunId?: string;
      conversationId?: string;
      metadata?: Record<string, unknown>;
      messageData?: Record<string, unknown>;
    }
  ) => void;
  replaceAiAnswer: (
    id: string,
    tempKey: string,
    answer: string,
    messageData?: Record<string, unknown>
  ) => void;
  updateGeneratedImages: (id: string, tempKey: string, images: GeneratedImage[]) => void;
  finalizeAiMessage: (
    id: string,
    tempKey: string,
    data: {
      messageId?: string;
      workflowRunId?: string;
      status: TerminalRunStatus;
      error?: string;
      elapsedTime?: number;
      model?: { modelName: string } | null;
      generatedImages?: GeneratedImage[];
    }
  ) => void;
  pauseAiMessage: (
    id: string,
    tempKey: string,
    data?: {
      elapsedTime?: number;
      workflowRunId?: string;
      status?: 'pending_approval' | 'pending_question';
    }
  ) => void;
  expireAiMessage: (id: string, tempKey: string, data?: { workflowRunId?: string }) => void;
  resumeAiMessage: (id: string, tempKey: string, data?: { workflowRunId?: string }) => void;
  updateRunNode: (id: string, tempKey: string, node: NodeInfo) => void;
}

const defaultConversation = (id: string): Conversation => ({
  id,
  conversationId: '',
  title: '新建会话',
  messages: [],
  conversationData: {},
});

function genTempKey(): string {
  return `${Date.now()}_${Math.random().toString(36).slice(2, 10)}`;
}

function findMessageIndexByTempKey(messages: Message[], tempKey: string): number {
  return messages.findIndex(m => m.messageData?.tempKey === tempKey);
}

function findMessageIndexByBackendIdentity(
  messages: Message[],
  identity: { messageId?: string; workflowRunId?: string }
): number {
  const messageId = identity.messageId?.trim();
  const workflowRunId = identity.workflowRunId?.trim();
  if (!messageId && !workflowRunId) return -1;

  return messages.findIndex(message => {
    const messageData = message.messageData ?? {};
    return (
      (Boolean(messageId) &&
        (message.messageId === messageId || messageData.message_id === messageId)) ||
      (Boolean(workflowRunId) &&
        (message.WorkflowRunInfo?.id === workflowRunId ||
          messageData.workflow_run_id === workflowRunId ||
          message.messageId === workflowRunId ||
          messageData.message_id === workflowRunId))
    );
  });
}

function findMessageIndexByTempKeyOrIdentity(
  messages: Message[],
  tempKey: string,
  identity?: { messageId?: string; workflowRunId?: string }
): number {
  const byTempKey = findMessageIndexByTempKey(messages, tempKey);
  if (byTempKey >= 0) return byTempKey;
  return findMessageIndexByBackendIdentity(messages, identity ?? {});
}

function nodeKeyOf(n: NodeInfo): string {
  const id = typeof n.nodeId === 'string' ? n.nodeId : '';
  if (id.length > 0) return id;
  const type = typeof n.nodeType === 'string' ? n.nodeType : '';
  const title = typeof n.title === 'string' ? n.title : '';
  return `${type}|${title}`;
}

function collectContainerChildKeys(node: NodeInfo): Set<string> {
  const keys = new Set<string>();
  const rounds = [...(node.iterationRounds ?? []), ...(node.loopRounds ?? [])];
  rounds.forEach(round => {
    (round.nodes ?? []).forEach(child => {
      keys.add(nodeKeyOf(child));
    });
  });
  return keys;
}

function removeNestedContainerChildren(nodes: NodeInfo[]): NodeInfo[] {
  const containerChildKeys = new Set<string>();
  const containerKeys = new Set<string>();

  nodes.forEach(node => {
    const childKeys = collectContainerChildKeys(node);
    if (childKeys.size === 0) return;
    containerKeys.add(nodeKeyOf(node));
    childKeys.forEach(key => containerChildKeys.add(key));
  });

  if (containerChildKeys.size === 0) return nodes;

  return nodes.filter(node => {
    const key = nodeKeyOf(node);
    return containerKeys.has(key) || !containerChildKeys.has(key);
  });
}

const useChatStoreBase = create<ChatState>()((set, get) => ({
  conversations: {},
  currentId: null,

  initSingle: conversation => {
    set(state => {
      const existing = state.conversations[conversation.id];
      const conv: Conversation = {
        ...(existing || defaultConversation(conversation.id)),
        conversationId: conversation.conversationId ?? existing?.conversationId ?? '',
        title: conversation.title ?? existing?.title ?? '',
        messages: conversation.messages ?? existing?.messages ?? [],
        conversationData: conversation.conversationData ?? existing?.conversationData ?? {},
      };
      return {
        conversations: { ...state.conversations, [conversation.id]: conv },
        currentId: conversation.id,
      };
    });
  },

  getConversations: () => Object.values(get().conversations),
  getConversation: id => get().conversations[id],

  setCurrent: id => set({ currentId: id }),

  updateConversation: (id, data) => {
    set(state => {
      const conv = state.conversations[id];
      if (!conv) return state;
      const next: Conversation = {
        ...conv,
        conversationId: data.conversationId ?? conv.conversationId,
        title: data.title ?? conv.title,
        conversationData: data.conversationData ?? conv.conversationData,
      };
      return { conversations: { ...state.conversations, [id]: next } };
    });
  },

  migrateConversation: (oldId, newId) => {
    set(state => {
      if (oldId === newId || !state.conversations[oldId]) return state;

      const conv = state.conversations[oldId];
      const newConv: Conversation = { ...conv, id: newId };

      const nextConversations = { ...state.conversations };
      delete nextConversations[oldId];
      nextConversations[newId] = newConv;

      const nextCurrentId = state.currentId === oldId ? newId : state.currentId;

      return {
        conversations: nextConversations,
        currentId: nextCurrentId,
      };
    });
  },

  deleteConversation: id => {
    // Guarded in singleTest: keep at least one conversation
    set(state => {
      if (!state.conversations[id]) return state;
      const keys = Object.keys(state.conversations);
      if (keys.length <= 1) return state;
      const next = { ...state.conversations };
      delete next[id];
      const nextCurrent =
        state.currentId === id ? (keys.find(k => k !== id) ?? null) : state.currentId;
      return { conversations: next, currentId: nextCurrent };
    });
  },

  appendUserMessage: (id, payload) => {
    const tempKey =
      payload.tempKey && payload.tempKey.trim().length > 0 ? payload.tempKey : genTempKey();
    set(state => {
      const conv = state.conversations[id] ?? defaultConversation(id);
      const message: Message = {
        messageId: '',
        query: payload.query,
        answer: '',
        parentId: payload.parentId ?? '',
        // Client-side immediate loading indicator, regardless of workflow usage
        clientState: { phase: 'requesting', startedAt: Date.now() },
        inputs: payload.inputs,
        model: null,
        messageData: { tempKey },
      };
      const next: Conversation = { ...conv, messages: [...conv.messages, message] };
      return {
        conversations: { ...state.conversations, [id]: next },
      };
    });
    return { tempKey };
  },

  updateMessageInputs: (id, tempKey, inputs) => {
    set(state => {
      const conv = state.conversations[id];
      if (!conv) return state;
      const idx = findMessageIndexByTempKey(conv.messages, tempKey);
      if (idx < 0) return state;
      const target = conv.messages[idx];
      const updated: Message = {
        ...target,
        inputs: { ...target.inputs, ...inputs },
      };
      const nextMsgs = conv.messages.slice();
      nextMsgs[idx] = updated;
      return { conversations: { ...state.conversations, [id]: { ...conv, messages: nextMsgs } } };
    });
  },

  ensureAiMessage: (id, tempKey) => {
    set(state => {
      const conv = state.conversations[id];
      if (!conv) return state;
      const idx = findMessageIndexByTempKey(conv.messages, tempKey);
      if (idx >= 0) return state;
      const message: Message = {
        messageId: '',
        query: '',
        answer: '',
        parentId: '',
        clientState: { phase: 'requesting', startedAt: Date.now() },
        model: null,
        messageData: { tempKey },
      };
      const next: Conversation = { ...conv, messages: [...conv.messages, message] };
      return { conversations: { ...state.conversations, [id]: next } };
    });
  },

  appendAiChunk: (id, tempKey, chunk) => {
    set(state => {
      const conv = state.conversations[id];
      if (!conv) return state;
      const idx = findMessageIndexByTempKey(conv.messages, tempKey);
      if (idx < 0) return state;
      const target = conv.messages[idx];
      if (target.messageData?.sensitiveOutputBlocked === true) return state;
      const updated: Message = {
        ...target,
        answer: `${target.answer}${chunk}`,
        clientState: { ...(target.clientState ?? { phase: 'requesting' }), phase: 'streaming' },
      };
      const nextMsgs = conv.messages.slice();
      nextMsgs[idx] = updated;
      return { conversations: { ...state.conversations, [id]: { ...conv, messages: nextMsgs } } };
    });
  },

  mergeAiMessage: (id, tempKey, data) => {
    set(state => {
      const conv = state.conversations[id];
      if (!conv) return state;
      const idx = findMessageIndexByTempKeyOrIdentity(conv.messages, tempKey, {
        messageId: data.messageId,
        workflowRunId: data.workflowRunId,
      });
      if (idx < 0) return state;

      const target = conv.messages[idx];
      if (target.messageData?.sensitiveOutputBlocked === true) return state;

      const shouldPatchAnswer = typeof data.answer === 'string';
      const nextAnswer = shouldPatchAnswer
        ? data.answerMode === 'replace'
          ? (data.answer as string)
          : `${target.answer}${data.answer}`
        : target.answer;
      const nextRun = data.workflowRunId
        ? {
            id: data.workflowRunId,
            status: target.WorkflowRunInfo?.status ?? ('running' as RunStatus),
            error: target.WorkflowRunInfo?.error,
            elapsedTime: target.WorkflowRunInfo?.elapsedTime,
            runNodeInfo: target.WorkflowRunInfo?.runNodeInfo ?? [],
          }
        : target.WorkflowRunInfo;
      const nextMessageData = {
        ...target.messageData,
        ...(tempKey ? { tempKey } : {}),
        ...(data.messageId ? { message_id: data.messageId } : {}),
        ...(data.workflowRunId ? { workflow_run_id: data.workflowRunId } : {}),
        ...(data.conversationId ? { conversation_id: data.conversationId } : {}),
        ...(data.metadata ? { metadata: data.metadata } : {}),
        ...(data.messageData ?? {}),
      };
      const updated: Message = {
        ...target,
        answer: nextAnswer,
        messageId: data.messageId ?? target.messageId,
        WorkflowRunInfo: nextRun,
        messageData: nextMessageData,
        clientState: shouldPatchAnswer
          ? { ...(target.clientState ?? { phase: 'requesting' }), phase: 'streaming' }
          : target.clientState,
      };

      const nextMsgs = conv.messages.slice();
      nextMsgs[idx] = updated;
      return { conversations: { ...state.conversations, [id]: { ...conv, messages: nextMsgs } } };
    });
  },

  replaceAiAnswer: (id, tempKey, answer, messageData) => {
    set(state => {
      const conv = state.conversations[id];
      if (!conv) return state;
      const idx = findMessageIndexByTempKey(conv.messages, tempKey);
      if (idx < 0) return state;
      const target = conv.messages[idx];
      const updated: Message = {
        ...target,
        answer,
        messageData: { ...target.messageData, ...(messageData ?? {}) },
        clientState: { ...(target.clientState ?? { phase: 'requesting' }), phase: 'completed' },
      };
      const nextMsgs = conv.messages.slice();
      nextMsgs[idx] = updated;
      return { conversations: { ...state.conversations, [id]: { ...conv, messages: nextMsgs } } };
    });
  },

  updateGeneratedImages: (id, tempKey, images) => {
    set(state => {
      const conv = state.conversations[id];
      if (!conv) return state;
      const idx = findMessageIndexByTempKey(conv.messages, tempKey);
      if (idx < 0) return state;
      const target = conv.messages[idx];
      const updated: Message = {
        ...target,
        generatedImages: images,
        clientState: { ...(target.clientState ?? { phase: 'requesting' }), phase: 'streaming' },
      };
      const nextMsgs = conv.messages.slice();
      nextMsgs[idx] = updated;
      return { conversations: { ...state.conversations, [id]: { ...conv, messages: nextMsgs } } };
    });
  },

  finalizeAiMessage: (id, tempKey, data) => {
    set(state => {
      const conv = state.conversations[id];
      if (!conv) return state;
      const idx = findMessageIndexByTempKeyOrIdentity(conv.messages, tempKey, {
        messageId: data.messageId,
        workflowRunId: data.workflowRunId,
      });
      if (idx < 0) return state;
      const target = conv.messages[idx];
      const nextRun = {
        id: data.workflowRunId ?? target.WorkflowRunInfo?.id ?? '',
        // Finalize workflow-level status here only once to avoid jitter
        status: data.status,
        error: data.error ?? target.WorkflowRunInfo?.error,
        elapsedTime: data.elapsedTime ?? target.WorkflowRunInfo?.elapsedTime,
        runNodeInfo: target.WorkflowRunInfo?.runNodeInfo ?? [],
      };
      const updated: Message = {
        ...target,
        messageId: data.messageId ?? target.messageId,
        messageData: {
          ...target.messageData,
          ...(tempKey ? { tempKey } : {}),
          ...(data.messageId ? { message_id: data.messageId } : {}),
          ...(data.workflowRunId ? { workflow_run_id: data.workflowRunId } : {}),
        },
        model: data.model ?? target.model,
        generatedImages: data.generatedImages ?? target.generatedImages,
        WorkflowRunInfo: nextRun,
        clientState: {
          phase: 'completed',
          status:
            data.status === 'completed'
              ? 'completed'
              : data.status === 'stopped'
                ? 'stopped'
                : data.status === 'expired'
                  ? 'expired'
                  : 'error',
          error: data.error,
          startedAt: target.clientState?.startedAt,
          finishedAt: Date.now(),
        },
      };
      const nextMsgs = conv.messages.slice();
      nextMsgs[idx] = updated;
      return { conversations: { ...state.conversations, [id]: { ...conv, messages: nextMsgs } } };
    });
  },

  pauseAiMessage: (id, tempKey, data) => {
    set(state => {
      const conv = state.conversations[id];
      if (!conv) return state;
      const idx = findMessageIndexByTempKeyOrIdentity(conv.messages, tempKey, {
        workflowRunId: data?.workflowRunId,
      });
      if (idx < 0) return state;
      const target = conv.messages[idx];
      const pendingStatus = data?.status ?? ('pending_approval' as const);
      const nextRun = {
        id: data?.workflowRunId ?? target.WorkflowRunInfo?.id ?? '',
        status: pendingStatus as RunStatus,
        error: target.WorkflowRunInfo?.error,
        elapsedTime: data?.elapsedTime ?? target.WorkflowRunInfo?.elapsedTime,
        runNodeInfo: target.WorkflowRunInfo?.runNodeInfo ?? [],
      };
      const updated: Message = {
        ...target,
        messageData: {
          ...target.messageData,
          ...(tempKey ? { tempKey } : {}),
          ...(data?.workflowRunId ? { workflow_run_id: data.workflowRunId } : {}),
        },
        WorkflowRunInfo: nextRun,
        clientState: {
          ...(target.clientState ?? { phase: 'streaming' }),
          phase: 'completed',
          status: pendingStatus,
        },
      };
      const nextMsgs = conv.messages.slice();
      nextMsgs[idx] = updated;
      return { conversations: { ...state.conversations, [id]: { ...conv, messages: nextMsgs } } };
    });
  },

  resumeAiMessage: (id, tempKey, data) => {
    set(state => {
      const conv = state.conversations[id];
      if (!conv) return state;
      const idx = findMessageIndexByTempKey(conv.messages, tempKey);
      if (idx < 0) return state;
      const target = conv.messages[idx];
      const nextRun = {
        id: data?.workflowRunId ?? target.WorkflowRunInfo?.id ?? '',
        status: 'running' as RunStatus,
        error: target.WorkflowRunInfo?.error,
        elapsedTime: target.WorkflowRunInfo?.elapsedTime,
        runNodeInfo: target.WorkflowRunInfo?.runNodeInfo ?? [],
      };
      const updated: Message = {
        ...target,
        WorkflowRunInfo: nextRun,
        clientState: {
          ...(target.clientState ?? { startedAt: Date.now() }),
          phase: 'streaming',
          status: undefined,
          error: undefined,
          finishedAt: undefined,
        },
      };
      const nextMsgs = conv.messages.slice();
      nextMsgs[idx] = updated;
      return { conversations: { ...state.conversations, [id]: { ...conv, messages: nextMsgs } } };
    });
  },

  expireAiMessage: (id, tempKey, data) => {
    set(state => {
      const conv = state.conversations[id];
      if (!conv) return state;
      const idx = findMessageIndexByTempKeyOrIdentity(conv.messages, tempKey, {
        workflowRunId: data?.workflowRunId,
      });
      if (idx < 0) return state;
      const target = conv.messages[idx];
      const nextRun = {
        id: data?.workflowRunId ?? target.WorkflowRunInfo?.id ?? '',
        status: 'expired' as RunStatus,
        error: target.WorkflowRunInfo?.error,
        elapsedTime: target.WorkflowRunInfo?.elapsedTime,
        runNodeInfo: target.WorkflowRunInfo?.runNodeInfo ?? [],
      };
      const updated: Message = {
        ...target,
        messageData: {
          ...target.messageData,
          ...(tempKey ? { tempKey } : {}),
          ...(data?.workflowRunId ? { workflow_run_id: data.workflowRunId } : {}),
        },
        WorkflowRunInfo: nextRun,
        clientState: {
          ...(target.clientState ?? { phase: 'completed' }),
          phase: 'completed',
          status: 'expired',
          finishedAt: Date.now(),
        },
      };
      const nextMsgs = conv.messages.slice();
      nextMsgs[idx] = updated;
      return { conversations: { ...state.conversations, [id]: { ...conv, messages: nextMsgs } } };
    });
  },

  updateRunNode: (id, tempKey, node) => {
    set(state => {
      const conv = state.conversations[id];
      if (!conv) return state;
      const idx = findMessageIndexByTempKey(conv.messages, tempKey);
      if (idx < 0) return state;
      const target = conv.messages[idx];
      // Preserve workflow-level status as running until finalize step decides outcome
      const run = target.WorkflowRunInfo ?? {
        id: '',
        status: 'running' as RunStatus,
        runNodeInfo: [],
      };
      const runNodeInfo = [...(run.runNodeInfo ?? [])];

      // Upsert by nodeId when available to avoid duplicates and stale running entries
      const key = nodeKeyOf(node);
      let existingIdx = runNodeInfo.findIndex(n => nodeKeyOf(n as NodeInfo) === key);
      // If new event has no nodeId, try to merge to last same-type item without id
      if (existingIdx < 0 && (!node.nodeId || node.nodeId.length === 0)) {
        for (let i = runNodeInfo.length - 1; i >= 0; i--) {
          const prev = runNodeInfo[i] as NodeInfo;
          const prevHasId = typeof prev.nodeId === 'string' && prev.nodeId.length > 0;
          if (!prevHasId && prev.nodeType === node.nodeType) {
            existingIdx = i;
            break;
          }
        }
      }
      // If new event has nodeId but we didn't find by nodeId key, try to merge with last item of same type/title without id (upgrade id)
      if (existingIdx < 0 && node.nodeId && node.nodeId.length > 0) {
        for (let i = runNodeInfo.length - 1; i >= 0; i--) {
          const prev = runNodeInfo[i] as NodeInfo;
          const prevHasId = typeof prev.nodeId === 'string' && prev.nodeId.length > 0;
          const sameType = prev.nodeType === node.nodeType;
          const sameTitle = (prev.title ?? '') === (node.title ?? '');
          if (!prevHasId && sameType && sameTitle) {
            existingIdx = i;
            break;
          }
        }
      }
      if (existingIdx >= 0) {
        const prevNode = runNodeInfo[existingIdx] as NodeInfo;
        const merged: NodeInfo = {
          nodeId: node.nodeId ?? prevNode.nodeId,
          nodeType: node.nodeType ?? prevNode.nodeType,
          title: node.title ?? prevNode.title,
          status: node.status,
          error: node.error ?? undefined,
          elapsedTime:
            typeof node.elapsedTime === 'number' ? node.elapsedTime : prevNode.elapsedTime,
          data: {
            input: node.data?.input ?? prevNode.data?.input,
            output: node.data?.output ?? prevNode.data?.output,
            modelInput: node.data?.modelInput ?? prevNode.data?.modelInput,
          },
          iterationInputs: node.iterationInputs ?? prevNode.iterationInputs,
          iterationOutputs: node.iterationOutputs ?? prevNode.iterationOutputs,
          iterationRounds: node.iterationRounds ?? prevNode.iterationRounds,
          loopInputs: node.loopInputs ?? prevNode.loopInputs,
          loopOutputs: node.loopOutputs ?? prevNode.loopOutputs,
          loopRounds: node.loopRounds ?? prevNode.loopRounds,
          steps: typeof node.steps === 'number' ? node.steps : prevNode.steps,
        };
        runNodeInfo[existingIdx] = merged;
      } else {
        runNodeInfo.push({ ...node });
      }

      const normalizedRunNodeInfo = removeNestedContainerChildren(runNodeInfo as NodeInfo[]);

      const updated: Message = {
        ...target,
        // Do not override workflow-level status here to avoid UI jitter; only append node info
        WorkflowRunInfo: { ...run, runNodeInfo: normalizedRunNodeInfo },
      };
      const nextMsgs = conv.messages.slice();
      nextMsgs[idx] = updated;
      return { conversations: { ...state.conversations, [id]: { ...conv, messages: nextMsgs } } };
    });
  },
}));

export const useChatStore = createSelectors(useChatStoreBase);
