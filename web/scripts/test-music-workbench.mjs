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
      jsx: ts.JsxEmit.ReactJSX,
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

function renderMusicComposer(mutation) {
  const stateValues = ['instrumental', 'A quiet piano piece', '', 1, null, false];
  let stateIndex = 0;
  const react = {
    useEffect() {},
    useRef(initialValue) {
      return { current: initialValue };
    },
    useState(initialValue) {
      const value = stateIndex < stateValues.length ? stateValues[stateIndex] : initialValue;
      stateIndex += 1;
      return [value, () => {}];
    },
  };
  const createElement = (type, props, key) => ({ key, props: props ?? {}, type });
  const components = new Proxy({}, { get: (_, name) => String(name) });
  const { MusicComposer } = loadTypeScriptModule(
    'src/components/music/music-composer.tsx',
    new Map([
      ['react', react],
      ['react/jsx-runtime', { Fragment: 'fragment', jsx: createElement, jsxs: createElement }],
      ['lucide-react', components],
      ['@/components/ui/button', components],
      ['@/components/ui/select', components],
      ['@/components/ui/textarea', components],
      ['@/hooks/music/use-music-tasks', { useCreateMusicTasks: () => mutation }],
      ['@/i18n', { useT: () => key => key }],
      ['@/lib/utils', { cn: (...values) => values.filter(Boolean).join(' ') }],
      ['@/utils/error-notifications', { getErrorMessage: () => '' }],
    ])
  );
  return MusicComposer({
    onCreated() {},
    reuseTask: null,
    model: 'provider-model',
    models: [],
    modelsLoading: false,
    modelsError: null,
    onModelChange() {},
  }).props.children;
}

const requests = [];
class FakeBaseService {
  constructor(config) {
    this.config = config;
  }

  request(method, path, data, options) {
    requests.push({ config: this.config, method, path, data, options });
    return Promise.resolve({ data: null });
  }
}

const service = loadTypeScriptModule(
  'src/services/music.service.ts',
  new Map([['@/lib/http/services', { BaseService: FakeBaseService }]])
).musicService;

await service.createTask({
  request_id: 'request-1',
  model: 'provider-model',
  mode: 'instrumental',
  prompt: 'A quiet piano piece',
});
await service.listTasks({ page: 2, page_size: 10, search: 'piano' });
await service.getTask('task/with unsafe characters');

assert.deepEqual(requests, [
  {
    config: { endpoint: 'main', basePath: '/console/api/music' },
    method: 'post',
    path: '/tasks',
    data: {
      request_id: 'request-1',
      model: 'provider-model',
      mode: 'instrumental',
      prompt: 'A quiet piano piece',
    },
    options: undefined,
  },
  {
    config: { endpoint: 'main', basePath: '/console/api/music' },
    method: 'get',
    path: '/tasks',
    data: undefined,
    options: { params: { page: 2, page_size: 10, search: 'piano' } },
  },
  {
    config: { endpoint: 'main', basePath: '/console/api/music' },
    method: 'get',
    path: '/tasks/task%2Fwith%20unsafe%20characters',
    data: undefined,
    options: undefined,
  },
]);

const variantRequestStart = requests.length;
await service.createTasks({
  model: 'provider-model',
  mode: 'instrumental',
  prompt: 'A quiet piano piece',
  variant_count: 2,
});
const variantRequests = requests.slice(variantRequestStart);
assert.equal(variantRequests.length, 2, 'variant count must create one task per requested variant');
assert.notEqual(
  variantRequests[0].data.request_id,
  variantRequests[1].data.request_id,
  'each generated variant must use an independent request ID'
);
for (const request of variantRequests) {
  assert.equal(request.method, 'post');
  assert.equal(request.path, '/tasks');
  assert.match(request.data.request_id, /^[0-9a-f-]{36}$/i);
  assert.deepEqual(
    { ...request.data, request_id: undefined },
    {
      request_id: undefined,
      model: 'provider-model',
      mode: 'instrumental',
      prompt: 'A quiet piano piece',
    },
    'variant_count is client orchestration state and must not leak into the backend request'
  );
}

let partialAttempt = 0;
class PartiallyFailingBaseService {
  constructor(config) {
    this.config = config;
  }

  request() {
    partialAttempt += 1;
    if (partialAttempt === 2) return Promise.reject(new Error('variant 2 failed'));
    return Promise.resolve({ data: { id: `task-${partialAttempt}` } });
  }
}
const partialService = loadTypeScriptModule(
  'src/services/music.service.ts',
  new Map([['@/lib/http/services', { BaseService: PartiallyFailingBaseService }]])
).musicService;
const partialResult = await partialService.createTasks({
  model: 'provider-model',
  mode: 'instrumental',
  prompt: 'A quiet piano piece',
  variant_count: 3,
});
assert.deepEqual(
  partialResult.responses.map(response => response.data.id),
  ['task-1', 'task-3'],
  'successful variants must remain visible when another variant request fails'
);
assert.equal(partialResult.failedCount, 1, 'partial variant failures must be reported honestly');

let finishFirstSubmission;
const firstSubmission = new Promise(resolve => {
  finishFirstSubmission = resolve;
});
let submissionAttempts = 0;
const originalSetTimeout = globalThis.setTimeout;
const cooldownTimers = [];
globalThis.setTimeout = (callback, delay) => {
  cooldownTimers.push({ callback, delay });
  return cooldownTimers.length;
};
try {
  const composerForm = renderMusicComposer({
    error: null,
    isPending: false,
    reset() {},
    mutateAsync() {
      submissionAttempts += 1;
      if (submissionAttempts === 1) return firstSubmission;
      if (submissionAttempts === 2) return Promise.reject(new Error('create failed'));
      return Promise.resolve({ responses: [], failedCount: 0 });
    },
  });
  assert.ok(composerForm, 'music composer must render a form');
  const submitEvent = { preventDefault() {} };
  const pendingSubmission = composerForm.props.onSubmit(submitEvent);
  const ignoredSubmission = composerForm.props.onSubmit(submitEvent);
  assert.equal(
    submissionAttempts,
    1,
    'rapid repeated submits must create only one batch while the first request is pending'
  );
  finishFirstSubmission({ responses: [], failedCount: 0 });
  await Promise.all([pendingSubmission, ignoredSubmission]);

  await composerForm.props.onSubmit(submitEvent);
  assert.equal(submissionAttempts, 1, 'a completed request must remain blocked for two seconds');
  assert.equal(cooldownTimers[0]?.delay, 2000, 'the submit cooldown must last two seconds');

  cooldownTimers.shift()?.callback();
  await composerForm.props.onSubmit(submitEvent);
  await composerForm.props.onSubmit(submitEvent);
  assert.equal(submissionAttempts, 2, 'a failed request must keep the same two-second cooldown');

  cooldownTimers.shift()?.callback();
  await composerForm.props.onSubmit(submitEvent);
  assert.equal(submissionAttempts, 3, 'submits must resume after each cooldown ends');
} finally {
  globalThis.setTimeout = originalSetTimeout;
}

const state = loadTypeScriptModule('src/components/music/music-task-state.ts');
for (const status of ['queued', 'generating_lyrics', 'generating', 'compensation_pending']) {
  assert.equal(state.shouldPollMusicTask(status), true, `${status} must keep polling`);
}
for (const status of ['succeeded', 'failed']) {
  assert.equal(state.shouldPollMusicTask(status), false, `${status} must stop polling`);
}

assert.equal(
  state.toMusicDownloadURL('/console/api/files/file-1/signed-url'),
  '/console/api/files/file-1/signed-url?download=1'
);
assert.equal(
  state.toMusicDownloadURL('/console/api/files/file-1/signed-url?token=value'),
  '/console/api/files/file-1/signed-url?token=value&download=1'
);
assert.equal(
  state.toMusicAssetURL('/console/api/files/tools/file-1.mp3?sign=value', 'http://localhost:8025'),
  'http://localhost:8025/console/api/files/tools/file-1.mp3?sign=value'
);
assert.equal(
  state.toMusicAssetURL(
    'http://internal-api:8025/console/api/files/tools/file-1.mp3?sign=value',
    'https://api.example.com/'
  ),
  'https://api.example.com/console/api/files/tools/file-1.mp3?sign=value'
);
assert.equal(state.toMusicAssetURL('javascript:alert(1)', 'https://api.example.com'), null);
assert.equal(
  state.resolveMusicDurationSeconds(Number.POSITIVE_INFINITY, 120_000),
  120,
  'seek must fall back to decoded or stored duration when the media element reports Infinity'
);
assert.equal(state.clampMusicSeekTime(130, Number.POSITIVE_INFINITY, 120_000), 120);
assert.equal(state.clampMusicSeekTime(-10, 120, 120_000), 0);
assert.equal(state.MUSIC_TIMELINE_SEGMENTS, 4, 'every player timeline must use four visual blocks');
assert.equal(
  state.shouldPrepareMusicTask('succeeded', '/console/api/files/file-1'),
  true,
  'a completed selected track must prepare the shared player before the first play request'
);
assert.equal(state.shouldPrepareMusicTask('generating', '/console/api/files/file-1'), false);
assert.equal(state.shouldPrepareMusicTask('succeeded', undefined), false);
assert.deepEqual(
  state.resolveMusicSourcePlaybackTransition('task-1', 'task-1', true, 0.35),
  { progress: 0.35, shouldResume: true },
  'refreshing the signed URL for a playing track must preserve playback and progress'
);
assert.deepEqual(
  state.resolveMusicSourcePlaybackTransition('task-1', 'task-1', false, 0.35),
  { progress: 0.35, shouldResume: false },
  'refreshing the signed URL for a paused track must preserve progress without autoplay'
);
assert.deepEqual(
  state.resolveMusicSourcePlaybackTransition('task-1', 'task-2', true, 0.35),
  { progress: 0, shouldResume: false },
  'selecting another track must not inherit playback state'
);

const waveformData = loadTypeScriptModule('src/components/music/music-waveform-data.ts');
assert.equal(
  waveformData.toMusicWaveformSource(
    '/console/api/files/tools/file-1.mp3?expires_at=1&delivery=direct&sign=value'
  ),
  '/console/api/files/tools/file-1.mp3?expires_at=1&sign=value',
  'deferred waveform loading must keep using the authenticated proxy URL'
);
assert.deepEqual(
  waveformData.buildMusicWaveformPeaks(
    {
      numberOfChannels: 2,
      length: 4,
      duration: 0.004,
      getChannelData: channel =>
        channel === 0
          ? new Float32Array([0.1, 0.2, 0.4, 0.1])
          : new Float32Array([-0.1, -0.2, -0.4, -0.1]),
    },
    4
  ),
  [25, 50, 100, 25]
);

const availableMusicModels = [
  {
    id: 'model-1',
    model: 'music-2.6',
    model_name: 'Music 2.6',
    provider: 'minimax',
    endpoints: { music_generation: true },
  },
  {
    id: 'model-2',
    model: 'music-3.0',
    model_name: 'Music 3.0',
    provider: 'minimax',
    endpoints: { music_generation: true },
  },
];
const musicModelsHook = loadTypeScriptModule(
  'src/hooks/music/use-music-models.ts',
  new Map([
    [
      '@tanstack/react-query',
      {
        useQuery: () => ({
          data: { data: { items: availableMusicModels, total: availableMusicModels.length } },
          isLoading: false,
          error: null,
        }),
      },
    ],
    ['@/hooks/query-keys', { MUSIC_KEYS: { models: organizationId => ['music', organizationId] } }],
    ['@/services/model.service', { modelService: { getAvailableModels: () => Promise.resolve() } }],
    [
      '@/store/organization-store',
      {
        useOrganizationStore: {
          use: { currentOrganization: () => ({ id: 'organization-1' }) },
        },
      },
    ],
  ])
);
assert.deepEqual(
  musicModelsHook.useMusicModels().models.map(model => model.model),
  ['music-2.6', 'music-3.0'],
  'available music models returned by the API must remain selectable'
);

const modelHookSource = readFileSync(
  path.join(root, 'src/hooks/music/use-music-models.ts'),
  'utf8'
);
assert.match(modelHookSource, /use_case:\s*'music-gen'/);
assert.match(modelHookSource, /music_generation/);
assert.match(modelHookSource, /currentOrganization/);
assert.doesNotMatch(modelHookSource, /music-2\.6|music-3\.0/);

const taskHookSource = readFileSync(path.join(root, 'src/hooks/music/use-music-tasks.ts'), 'utf8');
assert.match(taskHookSource, /useCurrentWorkspace/);
assert.match(taskHookSource, /workspaceId/);
assert.match(taskHookSource, /currentOrganization/);
assert.doesNotMatch(taskHookSource, /Workspace context is required/);

const taskQueryOptions = [];
const taskMutationOptions = [];
const createdTaskRequests = [];
const musicTasksHook = loadTypeScriptModule(
  'src/hooks/music/use-music-tasks.ts',
  new Map([
    [
      '@tanstack/react-query',
      {
        useMutation: options => {
          taskMutationOptions.push(options);
          return options;
        },
        useQuery: options => {
          taskQueryOptions.push(options);
          return options;
        },
        useQueryClient: () => ({
          invalidateQueries: () => Promise.resolve(),
          setQueryData: () => {},
        }),
      },
    ],
    ['@/components/music/music-task-state', { shouldPollMusicTask: () => false }],
    [
      '@/hooks/query-keys',
      {
        MUSIC_KEYS: {
          detail: (organizationId, workspaceId, id) => [
            'music',
            'detail',
            organizationId,
            workspaceId,
            id,
          ],
          list: (organizationId, workspaceId, params) => [
            'music',
            'list',
            organizationId,
            workspaceId,
            params,
          ],
          lists: (organizationId, workspaceId) => [
            'music',
            'list',
            organizationId,
            workspaceId,
          ],
        },
      },
    ],
    [
      '@/services/music.service',
      {
        musicService: {
          createTasks: request => {
            createdTaskRequests.push(request);
            return Promise.resolve({ responses: [] });
          },
          getTask: () => Promise.resolve(),
          listTasks: () => Promise.resolve(),
        },
      },
    ],
    [
      '@/store/organization-store',
      {
        useOrganizationStore: {
          use: { currentOrganization: () => ({ id: 'organization-1' }) },
        },
      },
    ],
    ['@/store/workspace-store', { useCurrentWorkspace: () => null }],
  ])
);

musicTasksHook.useMusicTasks({ page: 1, page_size: 20 });
musicTasksHook.useMusicTask('task-1');
musicTasksHook.useCreateMusicTasks();
assert.equal(taskQueryOptions[0].enabled, true, 'personal music task list must load by organization');
assert.equal(taskQueryOptions[1].enabled, true, 'personal music task detail must load by organization');
assert.deepEqual(taskQueryOptions[0].queryKey.slice(0, 4), [
  'music',
  'list',
  'organization-1',
  null,
]);
await taskMutationOptions[0].mutationFn({
  request_id: 'personal-request',
  model: 'music-3.0',
  mode: 'instrumental',
  prompt: 'personal warm piano',
});
assert.equal(createdTaskRequests.length, 1, 'personal music task creation must reach the API');

const workbenchSource = readFileSync(
  path.join(root, 'src/components/music/music-workbench.tsx'),
  'utf8'
);
const composerSource = readFileSync(
  path.join(root, 'src/components/music/music-composer.tsx'),
  'utf8'
);
const trackListSource = readFileSync(
  path.join(root, 'src/components/music/music-track-list.tsx'),
  'utf8'
);
const musicEnglishMessagesSource = readFileSync(
  path.join(root, 'src/i18n/modules/music/en-US.ts'),
  'utf8'
);
const musicChineseMessagesSource = readFileSync(
  path.join(root, 'src/i18n/modules/music/zh-Hans.ts'),
  'utf8'
);
const waveformSource = readFileSync(
  path.join(root, 'src/components/music/music-waveform.tsx'),
  'utf8'
);
const lyricsSource = readFileSync(
  path.join(root, 'src/components/music/music-lyrics-dialog.tsx'),
  'utf8'
);
const playerSource = readFileSync(path.join(root, 'src/components/music/music-player.tsx'), 'utf8');
const waveformHookSource = readFileSync(
  path.join(root, 'src/components/music/use-music-waveform.ts'),
  'utf8'
);
const segmentedProgressSource = readFileSync(
  path.join(root, 'src/components/music/music-segmented-progress.tsx'),
  'utf8'
);
const musicSources = [
  workbenchSource,
  composerSource,
  trackListSource,
  waveformSource,
  lyricsSource,
  playerSource,
  segmentedProgressSource,
];
assert.match(composerSource, /mode === 'vocal'/);
assert.match(trackListSource, /MusicWaveform/);
assert.match(lyricsSource, /task\.lyrics/);
assert.match(playerSource, /toMusicDownloadURL/);
assert.match(playerSource, /useMusicWaveform/);
assert.match(playerSource, /onPlaying=.*setWaveformLoadSource\(source\)/s);
assert.match(waveformHookSource, /if \(!enabled \|\| !source \|\| initial\.peaks\.length > 0\) return/);
assert.match(composerSource, /variantCount/);
assert.match(playerSource, /<Slider/);
assert.match(playerSource, /seekSeconds/);
assert.match(playerSource, /onWaveformChange/);
assert.match(playerSource, /onProgressChange/);
assert.match(playerSource, /seekRequestToken/);
assert.match(playerSource, /MusicSegmentedProgress/);
assert.match(trackListSource, /waveformDataByTaskId/);
assert.match(trackListSource, /playbackProgress/);
assert.match(trackListSource, /onSeek/);
assert.match(trackListSource, /data-ui=["']music-track-meta["']/);
assert.match(trackListSource, /data-ui=["']music-track-controls["']/);
assert.match(trackListSource, /bg-gradient-to-br/);
assert.match(trackListSource, /bg-\[rgb\(250,250,249\)\]/);
assert.match(workbenchSource, /handleTrackSeek/);
assert.match(workbenchSource, /onSelect=\{handleSelect\}/);
assert.match(
  workbenchSource,
  /function handleSelect[\s\S]*setPendingPlaybackId\(null\)[\s\S]*setPendingSeek\(null\)/,
  'selecting another track must cancel a stale deferred play request'
);
assert.match(workbenchSource, /seekRequestToken/);
assert.match(workbenchSource, /shouldPrepareMusicTask/);
assert.match(
  workbenchSource,
  /const shouldPlay = pendingPlaybackId === selectedTask\.id/,
  'preparing a selected track must not require a play request'
);
assert.match(segmentedProgressSource, /data-ui=["']music-player-segmented-progress["']/);
assert.match(
  segmentedProgressSource,
  /data-ui=["']music-player-progress-track-segment["']/,
  'every playback section must render its own neutral background segment'
);
assert.doesNotMatch(
  segmentedProgressSource,
  /data-ui=["']music-player-progress-track["']/,
  'the timeline background must not collapse all sections into one continuous track'
);
assert.match(segmentedProgressSource, /onSeek/);
for (const marker of ['music-studio', 'music-generation-tab', 'music-workbench-grid']) {
  assert.match(workbenchSource, new RegExp(`data-ui=["']${marker}["']`));
}
for (const marker of ['music-composer-card', 'music-prompt-surface', 'music-composer-footer']) {
  assert.match(composerSource, new RegExp(`data-ui=["']${marker}["']`));
}
for (const marker of ['music-results-toolbar', 'music-track-card']) {
  assert.match(trackListSource, new RegExp(`data-ui=["']${marker}["']`));
}
assert.match(trackListSource, /DropdownMenu/);
assert.doesNotMatch(
  trackListSource,
  /onSelect=\{\(\) => onShowLyrics\(task\)\}/,
  'view lyrics is a primary track action and must not be duplicated in the overflow menu'
);
assert.match(playerSource, /data-ui=["']music-player-controls["']/);
assert.match(workbenchSource, /generationTab/);
assert.match(composerSource, /data-ui=["']music-model-selector["']/);
assert.doesNotMatch(workbenchSource, /data-ui=["']music-model-selector["']/);
assert.match(workbenchSource, /useMusicModels/);
assert.doesNotMatch(
  composerSource,
  /\{item\.model_name \|\| item\.model\}\s*·\s*\{item\.provider\}/,
  'the music model selector must not append a provider already represented by the display name'
);
assert.doesNotMatch(composerSource, /modelsQuery/);
assert.match(composerSource, /data-ui=["']music-variant-select["']/);
assert.match(
  composerSource,
  /data-ui=["']music-variant-select["'][\s\S]*data-ui=["']music-model-selector["'][\s\S]*type=["']submit["']/,
  'the composer footer must place model selection between variant count and generation'
);
assert.match(trackListSource, /data-ui=["']music-track-waveform["']/);
assert.match(trackListSource, /data-ui=["']music-track-duration["']/);
assert.match(trackListSource, /data-ui=["']music-track-download["']/);
assert.match(trackListSource, /onDownload/);
assert.match(
  trackListSource,
  /\{active\s*\?\s*\([\s\S]*t\('generationWaitHint'\)[\s\S]*:\s*null\}/,
  'active music tasks must explain that generation time can vary'
);
assert.match(
  musicChineseMessagesSource,
  /generationWaitHint:\s*'歌曲生成通常需要约 2 分钟，实际时间会因模型和当前任务量有所不同，请稍候。'/
);
assert.match(
  musicEnglishMessagesSource,
  /generationWaitHint:[\s\S]*'Music generation usually takes about 2 minutes\./
);
assert.match(workbenchSource, /musicService\.getTask/);
assert.match(workbenchSource, /toMusicDownloadURL/);
assert.match(trackListSource, /TRACK_ACCENTS/);
assert.equal(
  musicSources.reduce((count, source) => count + (source.match(/<audio/g)?.length ?? 0), 0),
  1
);
assert.doesNotMatch(musicSources.join('\n'), /autoPlay|Math\.random/);
assert.match(
  playerSource,
  /if \(!playRequestToken \|\| !source \|\| !audio\) return;[\s\S]*if \(handledPlayRequestTokenRef\.current === playRequestToken\) return;[\s\S]*handledPlayRequestTokenRef\.current = playRequestToken;/,
  'playback requests must wait for a source and run only once'
);
assert.match(
  workbenchSource,
  /resolveMusicSourcePlaybackTransition[\s\S]*setSeekRequestProgress\(transition\.progress\)[\s\S]*transition\.shouldResume/,
  'signed URL refreshes must restore the current track according to its playback state'
);
assert.match(waveformSource, /peaks/);
assert.doesNotMatch(
  waveformSource,
  /if \(!normalizedPeaks\.length\) \{\s*return/,
  'a successful track must remain seekable before decoded waveform peaks are available'
);

const musicTypesSource = readFileSync(path.join(root, 'src/services/types/music.ts'), 'utf8');
for (const field of ['title', 'style_tags', 'duration_ms', 'waveform_peaks']) {
  assert.match(musicTypesSource, new RegExp(`${field}[?]?:`), `MusicTask must expose ${field}`);
}

const configSource = readFileSync(path.join(root, 'src/lib/config.ts'), 'utf8');
const envExampleSource = readFileSync(path.join(root, '.env.example'), 'utf8');
const sidebarSource = readFileSync(
  path.join(root, 'src/components/console/console-sidebar.tsx'),
  'utf8'
);
const musicPageSource = readFileSync(
  path.join(root, 'src/app/console/work/music/page.tsx'),
  'utf8'
);
for (const source of [configSource, envExampleSource, sidebarSource, musicPageSource]) {
  assert.doesNotMatch(source, /NEXT_PUBLIC_ENABLE_MUSIC_WORKBENCH|ENABLE_MUSIC_WORKBENCH/);
}
assert.match(sidebarSource, /href:\s*'\/console\/work\/music'/);
assert.match(musicPageSource, /return <MusicWorkbench \/>/);

const navigationSource = readFileSync(path.join(root, 'src/routes/console-navigation.ts'), 'utf8');
assert.match(navigationSource, /\/console\/work\/music/);
assert.match(navigationSource, /purpose: 'music generation workbench'/);
assert.match(
  navigationSource,
  /href: '\/console\/work\/music'[\s\S]*?purpose: 'music generation workbench'[\s\S]*?scope: 'organization'/,
  'music workbench must be visible in organization-scoped personal workbench mode'
);

console.log('Music workbench data contract checks passed.');
