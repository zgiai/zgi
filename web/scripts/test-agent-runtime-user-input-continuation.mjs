import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();
const transportPath = path.join(root, 'src/components/chat/transports/agent-runtime-transport.ts');
const dispatchCalls = [];
const draftSseCalls = [];
const webappSseCalls = [];

const createClient = calls => ({
  sse: (url, options) => {
    calls.push({ url, options });
    return Promise.resolve({ close() {} });
  },
});

const mocks = new Map([
  [
    '@/lib/http',
    {
      http: createClient(draftSseCalls),
      webappHttp: createClient(webappSseCalls),
    },
  ],
  [
    '@/components/chat/controllers/aichat',
    { DEFAULT_AICHAT_MESSAGE_PAGINATION: { page: 1, limit: 20 } },
  ],
  [
    '@/components/chat/transports/aichat-transport',
    {
      dispatchAIChatStreamEvent: (...args) => dispatchCalls.push(args),
      mapAIChatSearchResult: value => value,
    },
  ],
]);

function loadTransportModule() {
  const source = readFileSync(transportPath, 'utf8');
  const output = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
      esModuleInterop: true,
    },
    fileName: transportPath,
  }).outputText;
  const testModule = new Module(transportPath);
  testModule.filename = transportPath;
  testModule.paths = Module._nodeModulePaths(path.dirname(transportPath));

  const originalLoad = Module._load;
  Module._load = function load(request, parent, isMain) {
    if (mocks.has(request)) {
      return mocks.get(request);
    }
    return originalLoad.call(this, request, parent, isMain);
  };
  try {
    testModule._compile(output, transportPath);
    return testModule.exports;
  } finally {
    Module._load = originalLoad;
  }
}

function createCallbacks() {
  return {
    onRequestError() {},
    onClose() {},
  };
}

const { createAgentDraftTransport, createAgentWebAppTransport } = loadTransportModule();
const payload = {
  answers: { environment: 'production', region: 'us-east-1' },
  surface: 'agent',
  runtime_context: 'runtime-context',
  operation_context: { operation: 'deploy' },
};
const callbacks = createCallbacks();
const abortController = new globalThis.AbortController();

await createAgentDraftTransport('agent-123').continueUserInput(
  'conversation/one',
  'message two',
  'request?three',
  payload,
  callbacks,
  abortController.signal
);

assert.equal(draftSseCalls.length, 1);
const draftCall = draftSseCalls[0];
assert.equal(
  draftCall.url,
  '/console/api/agents/agent-123/runtime/conversations/conversation%2Fone/messages/message%20two/user-input/request%3Fthree/continue'
);
assert.equal(draftCall.options.method, 'POST');
assert.strictEqual(draftCall.options.body, payload);
assert.strictEqual(draftCall.options.abortSignal, abortController.signal);
assert.strictEqual(draftCall.options.onError, callbacks.onRequestError);
assert.strictEqual(draftCall.options.onClose, callbacks.onClose);
assert.equal(typeof draftCall.options.isTerminalMessage, 'function');

const eventData = { message_id: 'message two', delta: 'continued' };
draftCall.options.onMessage({
  event: 'ignored-envelope-event',
  data: { event: 'message_chunk', data: eventData },
  id: 'event-7',
});
assert.deepEqual(dispatchCalls, [['message_chunk', eventData, 'event-7', callbacks]]);

await createAgentWebAppTransport('webapp-456').continueUserInput(
  'conversation-2',
  'message-2',
  'request-2',
  payload,
  callbacks
);

assert.equal(webappSseCalls.length, 1);
assert.equal(
  webappSseCalls[0].url,
  '/console/api/webapps/webapp-456/runtime/conversations/conversation-2/messages/message-2/user-input/request-2/continue'
);
assert.strictEqual(webappSseCalls[0].options.body, payload);

const continuationSource = readFileSync(
  path.join(
    root,
    'src/components/chat/runtime/controller/use-chat-runtime-message-actions/continuation.ts'
  ),
  'utf8'
);
const availabilityCheck = continuationSource.indexOf('if (!transport.continueUserInput)');
const unavailableError = continuationSource.indexOf(
  "throw new Error('User input continuation is unavailable.');",
  availabilityCheck
);
const continuationBinding = continuationSource.indexOf(
  'const continueUserInputStream = transport.continueUserInput?.bind(transport);',
  availabilityCheck
);

assert.notEqual(availabilityCheck, -1, 'The controller must preflight user-input continuation.');
assert.notEqual(
  unavailableError,
  -1,
  'The unavailable transport path must keep its explicit error.'
);
assert.ok(
  unavailableError < continuationBinding,
  'The unavailable transport error must be raised before continuation stream setup.'
);

console.log('Agent runtime user-input continuation checks passed.');
