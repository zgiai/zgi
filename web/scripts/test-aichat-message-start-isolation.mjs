import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();
const reducerPath = path.join(
  root,
  'src/components/chat/controllers/aichat/reducers/message.ts'
);
const output = ts.transpileModule(readFileSync(reducerPath, 'utf8'), {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
    esModuleInterop: true,
  },
  fileName: reducerPath,
}).outputText;

const createConversation = (id, title = '') => ({
  id,
  title,
  status: 'active',
  runtime_status: 'idle',
  dialogue_count: 0,
  created_at: 1,
  updated_at: 1,
});

const createMessage = ({ id, conversationId, parentId, query, modelName, modelProvider }) => ({
  id,
  conversation_id: conversationId,
  parent_id: parentId,
  query,
  answer: '',
  status: 'streaming',
  model_name: modelName,
  model_provider: modelProvider,
  created_at: 1,
  updated_at: 1,
});

const testModule = new Module(reducerPath);
testModule.filename = reducerPath;
testModule.paths = Module._nodeModulePaths(path.dirname(reducerPath));
const originalRequire = testModule.require.bind(testModule);
testModule.require = moduleID => {
  if (moduleID === '@/utils/model-output-filter') {
    return {
      SENSITIVE_OUTPUT_BLOCKED_FLAG: 'blocked',
      SENSITIVE_OUTPUT_BLOCKED_TOKEN: 'blocked-token',
      isSensitiveOutputBlockedValue: () => false,
    };
  }
  if (moduleID === '@/components/chat/controllers/aichat/types') {
    return { DEFAULT_AICHAT_MESSAGE_PAGINATION: { page: 1, page_size: 20, total: 0 } };
  }
  if (moduleID === '@/components/chat/utils/aichat-message') {
    return {
      createDraftAIChatConversation: createConversation,
      createStreamingAIChatMessage: createMessage,
      normalizeAIChatStatus: status => status,
      replaceAIChatConversation: (conversations, conversation) => [
        ...conversations.filter(item => item.id !== conversation.id),
        conversation,
      ],
      upsertAIChatMessage: (messages, message) => [
        ...messages.filter(item => item.id !== message.id),
        message,
      ],
    };
  }
  if (moduleID === '../selectors') return { getNextActiveSendingState: () => false };
  if (moduleID === './shared') {
    return {
      createAIChatFileMetadata: () => undefined,
      mergeMessageMetadata: (left, right) => ({ ...left, ...right }),
      clearRuntimeMessageMetadata: metadata => metadata,
      isStaleAIChatStreamEvent: () => false,
      preferCompleteIntermediateAnswerContent: value => value,
      removeTransientProgressItems: timeline => timeline ?? [],
    };
  }
  if (moduleID === '../presentation-order') {
    return {
      captureAnswerTimelineBoundary: value => value,
      clearAnswerTimelineBoundaryWithoutDurableTimeline: value => value,
      finalPresentationAnswer: value => value,
      mergePresentationItems: (left, right) => right ?? left,
      presentationPositionFromPayload: () => ({}),
      presentationStateFromMetadata: () => ({}),
      processTextPresentationItem: value => value,
      removePresentationSegment: value => value,
      upsertPresentationItem: value => value,
      withPresentationItems: value => value,
    };
  }
  if (moduleID === './skill') return { updateSkillInvocationMetadata: value => value };
  return originalRequire(moduleID);
};
testModule._compile(output, reducerPath);

const { applyMessageStartState } = testModule.exports;

const baseState = {
  conversations: [
    {
      ...createConversation('conversation-1'),
      current_leaf_message_id: 'message-1',
    },
  ],
  pagination: { page: 1, page_size: 20, total: 1 },
  activeConversationId: 'conversation-1',
  messagesByConversation: {
    'conversation-1': [createMessage({ id: 'message-1', conversationId: 'conversation-1' })],
  },
  messagePaginationByConversation: {
    'conversation-1': { page: 1, page_size: 20, total: 1 },
  },
  loadingOlderByConversation: {},
  streamingByMessageId: {
    'message-1': {
      conversation_id: 'conversation-1',
      message_id: 'message-1',
      answer: 'first answer',
      status: 'completed',
      timeline: [{ id: 'memory-1', type: 'memory_event', event: { action: 'update' } }],
    },
  },
  recoveringByConversation: {},
  stoppingByConversation: {},
  connectionByConversation: {},
  isLoadingList: false,
  isLoadingMessages: false,
  isSending: true,
  error: null,
};

const nextTurn = applyMessageStartState(
  baseState,
  {
    conversation_id: 'conversation-1',
    message_id: 'message-2',
    model: 'model',
    created_at: 2,
  },
  { previousConversationId: 'conversation-1', query: 'next turn' },
  'event-2'
);

assert.deepEqual(
  nextTurn.streamingByMessageId['message-2'].timeline,
  [],
  'A new turn in an existing conversation must not inherit the previous message timeline.'
);

const draftState = {
  ...baseState,
  conversations: [createConversation('draft-aichat-1')],
  activeConversationId: 'draft-aichat-1',
  messagesByConversation: {
    'draft-aichat-1': [
      createMessage({ id: 'draft-message-1', conversationId: 'draft-aichat-1' }),
    ],
  },
  messagePaginationByConversation: {
    'draft-aichat-1': { page: 1, page_size: 20, total: 1 },
  },
  streamingByMessageId: {
    'draft-message-1': {
      conversation_id: 'draft-aichat-1',
      message_id: 'draft-message-1',
      answer: '',
      status: 'streaming',
      timeline: [{ id: 'draft-progress', type: 'agent_progress', content: 'Starting' }],
    },
  },
};

const migratedDraft = applyMessageStartState(
  draftState,
  {
    conversation_id: 'conversation-2',
    message_id: 'message-3',
    model: 'model',
    created_at: 2,
  },
  { previousConversationId: 'draft-aichat-1', query: 'first turn' },
  'event-3'
);

assert.deepEqual(
  migratedDraft.streamingByMessageId['message-3'].timeline,
  draftState.streamingByMessageId['draft-message-1'].timeline,
  'Replacing a local draft conversation must preserve events already received for that draft.'
);

console.log('AIChat message start isolation checks passed.');
