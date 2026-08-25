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

const audio = loadTypeScriptModule('src/components/chat/variants/aichat/voice/pcm-audio.ts');

const clipped = new Int16Array(
  audio.encodeMonoPCM16(new Float32Array([-2, -1, -0.5, 0, 0.5, 1, 2]), 16_000)
);
assert.deepEqual([...clipped], [-32768, -32768, -16384, 0, 16384, 32767, 32767]);

const fortyEightKHzSecond = new Float32Array(48_000).fill(0.25);
const resampled = audio.encodeMonoPCM16(fortyEightKHzSecond, 48_000);
assert.equal(resampled.byteLength, 16_000 * 2, 'PCM output must be exactly 16 kHz / 16 bit.');

const highFrequencyPattern = new Float32Array(48_000);
for (let index = 0; index < highFrequencyPattern.length; index += 3) {
  highFrequencyPattern[index] = 1;
  highFrequencyPattern[index + 1] = -1;
  highFrequencyPattern[index + 2] = 0;
}
const averagedDownsample = new Int16Array(audio.encodeMonoPCM16(highFrequencyPattern, 48_000));
assert.ok(
  Math.abs(averagedDownsample[0]) <= 1,
  'Downsampling must average the source window instead of aliasing by sample picking.'
);

const overLimit = new Float32Array(16_000 * 61).fill(0.1);
assert.equal(
  audio.encodeMonoPCM16(overLimit, 16_000).byteLength,
  16_000 * 59 * 2,
  'PCM output must never exceed the 59-second recording limit.'
);
assert.throws(
  () => audio.encodeMonoPCM16(new Float32Array(), 16_000),
  error => error?.code === 'EMPTY_AUDIO'
);

assert.equal(audio.mergeVoiceTranscript('', '  recognized text  '), 'recognized text');
assert.equal(
  audio.mergeVoiceTranscript('existing draft', ' recognized text '),
  'existing draft recognized text'
);

let committedDraft = '';
await audio.applyVoiceTranscription({
  audio: new ArrayBuffer(2),
  signal: new globalThis.AbortController().signal,
  transcribe: async () => 'recognized text',
  getDraft: () => 'draft edited while transcribing',
  onDraftChange: value => {
    committedDraft = value;
  },
});
assert.equal(committedDraft, 'draft edited while transcribing recognized text');

let changedAfterFailure = false;
await assert.rejects(
  audio.applyVoiceTranscription({
    audio: new ArrayBuffer(2),
    signal: new globalThis.AbortController().signal,
    transcribe: async () => {
      throw new Error('failed');
    },
    getDraft: () => 'must survive',
    onDraftChange: () => {
      changedAfterFailure = true;
    },
  })
);
assert.equal(changedAfterFailure, false, 'A failed transcription must not overwrite the draft.');

let resolveLateTranscript;
let draftAfterCancellation = 'unchanged';
const cancelledTranscription = new globalThis.AbortController();
const lateTranscription = audio.applyVoiceTranscription({
  audio: new ArrayBuffer(2),
  signal: cancelledTranscription.signal,
  transcribe: () =>
    new Promise(resolve => {
      resolveLateTranscript = resolve;
    }),
  getDraft: () => draftAfterCancellation,
  onDraftChange: value => {
    draftAfterCancellation = value;
  },
});
cancelledTranscription.abort();
resolveLateTranscript('late text');
await lateTranscription;
assert.equal(
  draftAfterCancellation,
  'unchanged',
  'A transcription that resolves after cancellation must not change the draft.'
);

assert.equal(audio.formatVoiceRecordingDuration(0), '00:00');
assert.equal(audio.formatVoiceRecordingDuration(9), '00:09');
assert.equal(audio.formatVoiceRecordingDuration(60), '01:00');

const draftCalls = [];
const webAppCalls = [];
const mocks = new Map([
  [
    '@/lib/http',
    {
      http: {
        post: (...args) => {
          draftCalls.push(args);
          return Promise.resolve({ data: { request_id: 'draft-request', text: 'draft text' } });
        },
      },
      webappHttp: {
        post: (...args) => {
          webAppCalls.push(args);
          return Promise.resolve({ data: { request_id: 'webapp-request', text: 'webapp text' } });
        },
      },
    },
  ],
]);
const service = loadTypeScriptModule('src/services/voice-transcription.service.ts', mocks);
const pcm = new ArrayBuffer(8);
const abortController = new globalThis.AbortController();

assert.equal(
  await service.transcribeAgentDraftVoice('agent/one', pcm, abortController.signal),
  'draft text'
);
assert.equal(draftCalls.length, 1);
assert.equal(draftCalls[0][0], '/console/api/agents/agent%2Fone/runtime/audio/transcriptions');
assert.strictEqual(draftCalls[0][1], pcm);
assert.deepEqual(draftCalls[0][2], {
  headers: { 'Content-Type': 'audio/pcm' },
  retryAttemptsOverride: 0,
  signal: abortController.signal,
  skipErrorHandling: true,
  timeout: 90_000,
});

assert.equal(
  await service.transcribeAgentWebAppVoice('webapp two', pcm, abortController.signal),
  'webapp text'
);
assert.equal(webAppCalls.length, 1);
assert.equal(webAppCalls[0][0], '/console/api/webapps/webapp%20two/runtime/audio/transcriptions');
assert.strictEqual(webAppCalls[0][1], pcm);
assert.deepEqual(webAppCalls[0][2], {
  headers: { 'Content-Type': 'audio/pcm' },
  retryAttemptsOverride: 0,
  signal: abortController.signal,
  skipErrorHandling: true,
  timeout: 90_000,
});

const originalNavigator = globalThis.navigator;
const originalWindow = globalThis.window;
const originalAudioWorkletNode = globalThis.AudioWorkletNode;
let activePort = null;
let maxDurationCallback = null;
let sourceDisconnected = false;
let workletDisconnected = false;
let gainDisconnected = false;
let contextClosed = false;
let trackStopped = false;

class FakeAudioContext {
  sampleRate = 16_000;
  destination = {};
  audioWorklet = {
    addModule: async modulePath => assert.equal(modulePath, '/zgi/audio/pcm-recorder-worklet.js'),
  };

  createMediaStreamSource() {
    return {
      connect() {},
      disconnect() {
        sourceDisconnected = true;
      },
    };
  }

  createGain() {
    return {
      gain: { value: 1 },
      connect() {},
      disconnect() {
        gainDisconnected = true;
      },
    };
  }

  async close() {
    contextClosed = true;
  }
}

class FakeAudioWorkletNode {
  port = {
    onmessage: null,
    close() {},
  };

  constructor() {
    activePort = this.port;
  }

  connect() {}

  disconnect() {
    workletDisconnected = true;
  }
}

Object.defineProperty(globalThis, 'navigator', {
  configurable: true,
  value: {
    mediaDevices: {
      getUserMedia: async constraints => {
        assert.deepEqual(constraints, {
          audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true },
        });
        return {
          getTracks: () => [
            {
              stop() {
                trackStopped = true;
              },
            },
          ],
        };
      },
    },
  },
});
globalThis.window = {
  AudioContext: FakeAudioContext,
  setTimeout(callback) {
    maxDurationCallback = callback;
    return 7;
  },
  clearTimeout() {},
};
globalThis.AudioWorkletNode = FakeAudioWorkletNode;

try {
  const recorderModule = loadTypeScriptModule(
    'src/components/chat/variants/aichat/voice/browser-pcm-recorder.ts',
    new Map([
      ['./pcm-audio', audio],
      ['@/lib/config', { withBasePath: value => `/zgi${value}` }],
    ])
  );
  let reachedLimit = false;
  const recorder = new recorderModule.BrowserPCMRecorder();
  await recorder.start(() => {
    reachedLimit = true;
  });
  activePort.onmessage({ data: new Float32Array([0, 0.5, -0.5]) });
  maxDurationCallback();
  assert.equal(reachedLimit, true, 'The recorder must enforce the 59-second stop callback.');
  const recordedPCM = new Int16Array(await recorder.stop());
  assert.deepEqual([...recordedPCM], [0, 16384, -16384]);
  assert.equal(trackStopped, true);
  assert.equal(sourceDisconnected, true);
  assert.equal(workletDisconnected, true);
  assert.equal(gainDisconnected, true);
  assert.equal(contextClosed, true);
} finally {
  Object.defineProperty(globalThis, 'navigator', {
    configurable: true,
    value: originalNavigator,
  });
  globalThis.window = originalWindow;
  globalThis.AudioWorkletNode = originalAudioWorkletNode;
}

const voiceErrors = loadTypeScriptModule(
  'src/components/chat/variants/aichat/voice/voice-input-errors.ts'
);
assert.equal(voiceErrors.getVoiceInputErrorKey({ name: 'NotAllowedError' }), 'permissionDenied');
assert.equal(
  voiceErrors.getVoiceInputErrorKey({ response: { data: { code: 'NO_SPEECH_DETECTED' } } }),
  'noSpeech'
);
assert.equal(
  voiceErrors.getVoiceInputErrorKey({ response: { data: { code: 'INSUFFICIENT_BALANCE' } } }),
  'balance'
);
assert.equal(
  voiceErrors.getVoiceInputErrorKey({ response: { data: { code: 'INSUFFICIENT_QUOTA' } } }),
  'quota'
);
assert.equal(
  voiceErrors.getVoiceInputErrorKey({ response: { data: { code: 'VOICE_UNAVAILABLE' } } }),
  'unavailable'
);
assert.equal(voiceErrors.getVoiceInputErrorKey({ name: 'AbortError' }), 'cancelled');
assert.equal(
  voiceErrors.getVoiceInputErrorKey({ name: 'CanceledError', code: 'ERR_CANCELED' }),
  'cancelled',
  'Axios cancellation must be treated as an expected user action.'
);
assert.equal(
  voiceErrors.getVoiceInputErrorKey({ code: 'ECONNABORTED' }),
  'timeout',
  'Axios timeout must be reported as a transcription timeout.'
);
assert.equal(voiceErrors.getVoiceInputErrorKey({ code: 'ETIMEDOUT' }), 'timeout');
assert.equal(voiceErrors.getVoiceInputErrorKey({ name: 'NotSupportedError' }), 'unsupported');
assert.equal(voiceErrors.getVoiceInputErrorKey(new Error('provider internals')), 'failed');

const previewSource = readFileSync(
  path.join(root, 'src/components/agents/agent-runtime/preview-panel.tsx'),
  'utf8'
);
const pageModelSource = readFileSync(
  path.join(root, 'src/components/agents/agent-runtime/hooks/use-agent-runtime-page-model.tsx'),
  'utf8'
);
const webAppSource = readFileSync(
  path.join(root, 'src/components/webapp/agent-chat/index.tsx'),
  'utf8'
);
const workChatSource = readFileSync(path.join(root, 'src/app/console/work/chat/page.tsx'), 'utf8');
const inputAreaSource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/input-area.tsx'),
  'utf8'
);
const voiceControlSource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/voice/voice-input-control.tsx'),
  'utf8'
);
const modelSettingsSource = readFileSync(
  path.join(root, 'src/app/dashboard/settings/model/page.tsx'),
  'utf8'
);
assert.match(pageModelSource, /useDefaultModelByUseCase\('speech-to-text'\)/);
assert.match(
  previewSource,
  /voiceTranscriber=\{voiceInputEnabled \? handleVoiceTranscription : undefined\}/
);
assert.match(
  webAppSource,
  /voiceTranscriber=\{voiceInputEnabled \? handleVoiceTranscription : undefined\}/
);
assert.doesNotMatch(workChatSource, /voiceTranscriber=/);
assert.match(
  modelSettingsSource,
  /key: 'speech-to-text'/,
  'Organization model settings must expose the default STT model.'
);
assert.match(inputAreaSource, /hasUploadError \|\| voiceInputBusy/);
assert.match(inputAreaSource, /canClickSend && !voiceInputBusy/);
assert.match(
  voiceControlSource,
  /formatVoiceRecordingDuration\(elapsedSeconds\)/,
  'The recording control must render its elapsed duration.'
);

console.log('Agent voice input checks passed.');
