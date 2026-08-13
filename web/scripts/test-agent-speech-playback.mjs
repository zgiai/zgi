import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();

function loadTypeScriptModule(relativePath, mocks = new Map()) {
  const filename = path.join(root, relativePath);
  const source = readFileSync(filename, 'utf8');
  const output = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
      esModuleInterop: true,
    },
    fileName: filename,
  }).outputText;
  const testModule = new Module(filename);
  testModule.filename = filename;
  testModule.paths = Module._nodeModulePaths(path.dirname(filename));

  const originalLoad = Module._load;
  Module._load = function load(request, parent, isMain) {
    if (mocks.has(request)) return mocks.get(request);
    return originalLoad.call(this, request, parent, isMain);
  };
  try {
    testModule._compile(output, filename);
    return testModule.exports;
  } finally {
    Module._load = originalLoad;
  }
}

const speechText = loadTypeScriptModule('src/components/chat/variants/aichat/voice/speech-text.ts');
assert.equal(
  speechText.toAgentSpeechText(
    '# Result\n\n- **One**\n- [File](https://example.com/file)\n\n```ts\nconst x = 1;\n```'
  ),
  'Result\nOne\nFile https://example.com/file\nconst x = 1;'
);
assert.equal(speechText.toAgentSpeechText('  '), '');

const messageUtils = loadTypeScriptModule('src/components/chat/utils/aichat-message.ts');
assert.equal(
  messageUtils.isPersistedAIChatConversationPromotion('draft-aichat-1', 'conversation-1'),
  true,
  'Persisting the current draft must not reset speech auto-play state.'
);
assert.equal(
  messageUtils.isPersistedAIChatConversationPromotion('conversation-1', 'conversation-2'),
  false,
  'Selecting another persisted conversation must reset speech auto-play history.'
);

const playback = loadTypeScriptModule(
  'src/components/chat/variants/aichat/voice/speech-playback.ts'
);
const playbackErrors = loadTypeScriptModule(
  'src/components/chat/variants/aichat/voice/speech-playback-errors.ts'
);

assert.equal(playbackErrors.getSpeechPlaybackErrorKey({ response: { status: 402 } }), 'balance');
assert.equal(playbackErrors.getSpeechPlaybackErrorKey({ response: { status: 429 } }), 'quota');
assert.equal(
  playbackErrors.getSpeechPlaybackErrorKey({ response: { status: 503 } }),
  'unavailable'
);
assert.equal(playbackErrors.getSpeechPlaybackErrorKey({ response: { status: 504 } }), 'timeout');
assert.equal(
  playbackErrors.getSpeechPlaybackErrorKey({
    response: { data: { code: 'SPEECH_UNAVAILABLE' } },
  }),
  'unavailable'
);
assert.equal(playbackErrors.getSpeechPlaybackErrorKey(new Error('provider internals')), 'failed');

const historical = new Set();
playback.markCompletedSpeechMessagesSeen(
  [{ id: 'history', answer: 'old answer', status: 'completed' }],
  historical
);
assert.equal(
  playback.takeLatestUnseenCompletedSpeechMessage(
    [{ id: 'history', answer: 'old answer', status: 'completed' }],
    historical
  ),
  null,
  'History must not be replayed.'
);
assert.deepEqual(
  playback.takeLatestUnseenCompletedSpeechMessage(
    [
      { id: 'history', answer: 'old answer', status: 'completed' },
      { id: 'streaming', answer: 'partial', status: 'streaming' },
      { id: 'new', answer: 'new answer', status: 'completed' },
    ],
    historical
  ),
  { id: 'new', answer: 'new answer', status: 'completed' }
);

const sessions = [];
const states = [];
const synthCalls = [];
const controller = new playback.AgentSpeechPlaybackController({
  synthesize: async (text, signal) => {
    synthCalls.push({ text, signal });
    return new globalThis.ReadableStream({
      start(streamController) {
        streamController.enqueue(new Uint8Array([1, 2, 3]));
        streamController.close();
      },
    });
  },
  createSession: handlers => {
    const session = {
      handlers,
      attached: 0,
      playCalls: 0,
      pauseCalls: 0,
      closeCalls: 0,
      async attach() {
        this.attached += 1;
      },
      play() {
        this.playCalls += 1;
        handlers.onPlaying();
      },
      pause() {
        this.pauseCalls += 1;
        handlers.onPause();
      },
      close() {
        this.closeCalls += 1;
      },
    };
    sessions.push(session);
    return session;
  },
  onChange: state => states.push({ ...state }),
});

await controller.toggle('message-1', 'answer one');
assert.equal(synthCalls.length, 1);
assert.equal(sessions[0].playCalls, 1);
assert.equal(sessions[0].attached, 1);
assert.deepEqual(controller.snapshot(), { messageId: 'message-1', phase: 'playing' });

await controller.toggle('message-1', 'answer one');
assert.equal(sessions[0].pauseCalls, 1);
assert.deepEqual(controller.snapshot(), { messageId: 'message-1', phase: 'paused' });

await controller.toggle('message-1', 'answer one');
assert.equal(sessions[0].playCalls, 2);
assert.deepEqual(controller.snapshot(), { messageId: 'message-1', phase: 'playing' });

await controller.play('message-2', 'answer two');
assert.equal(sessions[0].closeCalls, 1, 'Starting another message must close the previous audio.');
assert.deepEqual(controller.snapshot(), { messageId: 'message-2', phase: 'playing' });

controller.stop();
assert.equal(sessions[1].closeCalls, 1);
assert.deepEqual(controller.snapshot(), { messageId: null, phase: 'idle' });
assert.ok(states.some(state => state.phase === 'loading'));

const unsupportedErrors = [];
const unsupportedController = new playback.AgentSpeechPlaybackController({
  synthesize: async () => new globalThis.ReadableStream(),
  createSession: () => {
    throw new Error('unsupported');
  },
  onError: error => unsupportedErrors.push(error.message),
});
await unsupportedController.play('message', 'answer');
assert.deepEqual(unsupportedController.snapshot(), { messageId: null, phase: 'idle' });
assert.deepEqual(unsupportedErrors, ['unsupported']);

function createHookHarness() {
  const slots = [];
  let cursor = 0;
  let pendingEffects = [];

  const sameDependencies = (left, right) =>
    left?.length === right?.length && left.every((value, index) => Object.is(value, right[index]));

  const react = {
    useCallback(callback, dependencies) {
      return react.useMemo(() => callback, dependencies);
    },
    useEffect(effect, dependencies) {
      const index = cursor++;
      const slot = slots[index];
      if (slot && sameDependencies(slot.dependencies, dependencies)) return;
      pendingEffects.push(() => {
        slot?.cleanup?.();
        slots[index] = { dependencies, cleanup: effect() };
      });
    },
    useMemo(factory, dependencies) {
      const index = cursor++;
      const slot = slots[index];
      if (slot && sameDependencies(slot.dependencies, dependencies)) return slot.value;
      const value = factory();
      slots[index] = { dependencies, value };
      return value;
    },
    useRef(initialValue) {
      const index = cursor++;
      if (!slots[index]) slots[index] = { value: { current: initialValue } };
      return slots[index].value;
    },
    useState(initialValue) {
      const index = cursor++;
      if (!slots[index]) {
        const slot = {
          value: typeof initialValue === 'function' ? initialValue() : initialValue,
          setValue(nextValue) {
            slot.value = typeof nextValue === 'function' ? nextValue(slot.value) : nextValue;
          },
        };
        slots[index] = slot;
      }
      return [slots[index].value, slots[index].setValue];
    },
  };

  return {
    react,
    render(renderHook) {
      cursor = 0;
      const result = renderHook();
      const effects = pendingEffects;
      pendingEffects = [];
      effects.forEach(run => run());
      return result;
    },
    cleanup() {
      for (const slot of slots) slot?.cleanup?.();
    },
  };
}

const hookHarness = createHookHarness();
const speechHookMocks = new Map([
  ['react', hookHarness.react],
  ['sonner', { toast: { error() {} } }],
  ['@/lib/observability', { captureError() {} }],
  [
    '@/components/chat/utils/aichat-message',
    { isPersistedAIChatConversationPromotion: () => false },
  ],
  [
    './browser-speech-audio',
    {
      createBrowserSpeechAudioSession: handlers => ({
        async attach() {},
        play() {
          handlers.onPlaying();
        },
        pause() {
          handlers.onPause();
        },
        close() {},
      }),
    },
  ],
  ['./speech-playback', playback],
  ['./speech-text', speechText],
  ['./speech-playback-errors', playbackErrors],
]);
const speechHook = loadTypeScriptModule(
  'src/components/chat/variants/aichat/voice/use-agent-speech-playback.ts',
  speechHookMocks
);
let pendingSpeechSignal;
const pendingSynthesizer = (_text, signal) => {
  pendingSpeechSignal = signal;
  return new Promise(() => {});
};
const renderSpeechHook = playbackErrorMessages =>
  hookHarness.render(() =>
    speechHook.useAgentSpeechPlayback({
      synthesizer: pendingSynthesizer,
      messages: [],
      conversationId: 'conversation',
      isLoadingMessages: false,
      playbackErrorMessages,
    })
  );

const firstSpeechHook = renderSpeechHook({
  timeout: 'timeout',
  balance: 'balance',
  quota: 'quota',
  unavailable: 'unavailable',
  failed: 'failed',
});
firstSpeechHook.toggle('message', 'answer');
assert.equal(pendingSpeechSignal.aborted, false);
renderSpeechHook({
  timeout: 'new timeout',
  balance: 'new balance',
  quota: 'new quota',
  unavailable: 'new unavailable',
  failed: 'new failed',
});
assert.equal(
  pendingSpeechSignal.aborted,
  false,
  'Updating translated playback errors must not recreate the controller and cancel speech.'
);
hookHarness.cleanup();

const draftCalls = [];
const webAppCalls = [];
const responseStream = new globalThis.ReadableStream();
const serviceMocks = new Map([
  [
    '@/lib/http',
    {
      http: {
        post: (...args) => {
          draftCalls.push(args);
          return Promise.resolve(responseStream);
        },
      },
      webappHttp: {
        post: (...args) => {
          webAppCalls.push(args);
          return Promise.resolve(responseStream);
        },
      },
    },
  ],
]);
const service = loadTypeScriptModule('src/services/voice-speech.service.ts', serviceMocks);
const signal = new globalThis.AbortController().signal;

assert.strictEqual(
  await service.generateAgentDraftSpeech('agent/one', 'answer', signal),
  responseStream
);
assert.equal(draftCalls[0][0], '/console/api/agents/agent%2Fone/runtime/audio/speech');
assert.deepEqual(draftCalls[0][1], { input: 'answer' });
assert.equal(draftCalls[0][2].adapter, 'fetch');
assert.equal(draftCalls[0][2].responseType, 'stream');
assert.equal(draftCalls[0][2].retryAttemptsOverride, 0);
assert.strictEqual(draftCalls[0][2].signal, signal);

assert.strictEqual(
  await service.generateAgentWebAppSpeech('webapp two', 'answer', signal),
  responseStream
);
assert.equal(webAppCalls[0][0], '/console/api/webapps/webapp%20two/runtime/audio/speech');
assert.deepEqual(webAppCalls[0][1], { input: 'answer' });
assert.equal(webAppCalls[0][2].adapter, 'fetch');
assert.equal(webAppCalls[0][2].responseType, 'stream');

await assert.rejects(
  () => service.generateAgentDraftSpeech('agent', ' ', signal),
  error => error?.code === 'INVALID_SPEECH_INPUT'
);
assert.equal(draftCalls.length, 1, 'Invalid input must fail before HTTP dispatch.');

const defaultVoice = loadTypeScriptModule('src/app/dashboard/settings/model/default-voice.ts');
assert.deepEqual(
  defaultVoice.getDefaultVoiceOptions([
    { name: 'speed', options: ['0.9', '1.0'] },
    {
      name: 'default_voice',
      options: [' voice-a ', 'voice-b', 'voice-a', ''],
    },
  ], 'zh-Hans'),
  [
    { value: 'voice-a', label: 'voice-a' },
    { value: 'voice-b', label: 'voice-b' },
  ],
  'Provider voice options must be normalized before rendering the selector.'
);
assert.deepEqual(
  defaultVoice.getDefaultVoiceOptions([{ name: 'default_voice' }], 'zh-Hans'),
  [],
  'Missing provider voice metadata must be reported instead of inventing a fallback option.'
);

const modelSettingsSource = readFileSync(
  path.join(root, 'src/app/dashboard/settings/model/page.tsx'),
  'utf8'
);
assert.match(
  modelSettingsSource,
  /key: 'text-to-speech'/,
  'Organization model settings must expose the default TTS model.'
);
assert.match(
  modelSettingsSource,
  /default_voice/,
  'The TTS model setting must require an explicit provider voice identifier.'
);
assert.match(modelSettingsSource, /aria-invalid=/, 'The required voice field must expose errors.');
assert.match(
  modelSettingsSource,
  /useModelParameterRules/,
  'The voice field must read provider parameter metadata.'
);
assert.match(
  modelSettingsSource,
  /defaultVoiceOptions\.length > 0/,
  'Provider voice options must switch the field to a selector.'
);
assert.match(
  modelSettingsSource,
  /defaultVoiceMetadataMissing/,
  'Missing provider voice metadata must be an explicit validation error.'
);
assert.doesNotMatch(
  modelSettingsSource,
  /manualMode|<Input/,
  'The default voice must never fall back to a provider ID text input.'
);

console.log('Agent speech playback tests passed.');
