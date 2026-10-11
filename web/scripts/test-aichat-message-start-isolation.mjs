import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();
const reducerPath = path.join(root, 'src/components/chat/controllers/aichat/reducers/message.ts');
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
  if (moduleID === '@/components/chat/utils/runtime-input-reconcile') {
    return { applyAuthoritativeRuntimeInputSnapshot: messages => messages };
  }
  if (moduleID === '../selectors')
    return {
      getNextActiveSendingState: () => false,
      sanitizeTimelineForMessage: (_message, timeline) => timeline,
    };
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
    const sourcePath = path.join(
      root,
      'src/components/chat/controllers/aichat/presentation-order.ts'
    );
    const actual = new Module(sourcePath);
    actual._compile(
      ts.transpileModule(readFileSync(sourcePath, 'utf8'), {
        compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
      }).outputText,
      sourcePath
    );
    return actual.exports;
  }
  if (moduleID === './skill') return { updateSkillInvocationMetadata: value => value };
  return originalRequire(moduleID);
};
testModule._compile(output, reducerPath);

const {
  applyMessageStartState,
  applyMessageEndState,
  removeStreamingStateByConversation,
  mergeAIChatMessages,
} = testModule.exports;

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
  nextTurn.streamingByMessageId['message-1'],
  baseState.streamingByMessageId['message-1'],
  'A new turn must preserve older page-local snapshots.'
);

assert.deepEqual(
  nextTurn.streamingByMessageId['message-2'].timeline,
  [],
  'A new turn in an existing conversation must not inherit the previous message timeline.'
);

const processItem = {
  presentation_version: 2,
  presentation_id: 'message-1:text:1',
  presentation_sequence: 1,
  kind: 'text',
  segment_id: 'message-1:text:1',
  content_phase: 'process',
  content: '正在处理文件。',
};
const live = {
  ...baseState,
  streamingByMessageId: {
    'message-1': {
      ...baseState.streamingByMessageId['message-1'],
      status: 'streaming',
      timeline: [],
      presentationVersion: 2,
      presentationItems: [processItem],
    },
  },
};
const ended = applyMessageEndState(live, {
  conversation_id: 'conversation-1',
  message_id: 'message-1',
  status: 'completed',
  answer: '完成。',
  metadata: { presentation: { version: 2, items: [] } },
});
assert.deepEqual(
  ended.streamingByMessageId['message-1'].presentationItems,
  [processItem],
  'Completion must retain live process text omitted from durable metadata.'
);
const switched = removeStreamingStateByConversation(ended.streamingByMessageId, 'conversation-1');
assert.deepEqual(
  switched['message-1'].presentationItems,
  [processItem],
  'Conversation selection must preserve completed process text.'
);
const history = mergeAIChatMessages(ended.messagesByConversation['conversation-1'], [
  {
    ...baseState.messagesByConversation['conversation-1'][0],
    status: 'completed',
    metadata: { presentation: { version: 2, items: [] } },
  },
]);
assert.equal(
  history[0].metadata.presentation.items.length,
  0,
  'History can stay compact: the separate page snapshot is the visual authority.'
);
assert.deepEqual(ended.streamingByMessageId['message-1'].presentationItems, [processItem]);
const replaced = applyMessageStartState(
  ended,
  {
    conversation_id: 'conversation-1',
    message_id: 'message-1',
    replace: true,
  },
  { previousConversationId: 'conversation-1' }
);
assert.equal(
  replaced.streamingByMessageId['message-1'].presentationItems,
  undefined,
  'Regenerating the same message must clear its old process text.'
);

const utilitiesPath = path.join(
  root,
  'src/components/chat/runtime/controller/chat-runtime-controller-utils.ts'
);
const utilities = new Module(utilitiesPath);
utilities.require = name =>
  name === '@/utils/client-id' ? { generateClientId: () => 'test-id' } : originalRequire(name);
utilities._compile(
  ts.transpileModule(readFileSync(utilitiesPath, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText,
  utilitiesPath
);
const refreshed = utilities.exports.removeRunningStreamingStateByConversation(
  ended.streamingByMessageId,
  'conversation-1'
);
assert.deepEqual(
  refreshed['message-1'].presentationItems,
  [processItem],
  'Silent idle refresh must keep completed presentation-only snapshots.'
);

const draftState = {
  ...baseState,
  conversations: [createConversation('draft-aichat-1')],
  activeConversationId: 'draft-aichat-1',
  messagesByConversation: {
    'draft-aichat-1': [createMessage({ id: 'draft-message-1', conversationId: 'draft-aichat-1' })],
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
