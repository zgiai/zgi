import assert from 'node:assert/strict';
import { Blob } from 'node:buffer';
import { load, read } from './connector-fixture.mjs';

const { AgentRuntimeTransport } = load(
  'src/components/chat/transports/agent-runtime-transport.ts',
  {
    '@/lib/http': { http: {}, webappHttp: {}, SSE_IDLE_TIMEOUT_MS: 1000 },
    '@/components/chat/controllers/aichat': { DEFAULT_AICHAT_MESSAGE_PAGINATION: {} },
    '@/components/chat/transports/aichat-transport': {
      dispatchAIChatStreamEvent() {},
      mapAIChatSearchResult() {},
    },
  }
);
const calls = [];
const blob = new Blob(['private-result']);
const client = {
  get: async (path, options) => {
    calls.push({ path, options });
    return blob;
  },
};
for (const base of [
  '/console/api/webapps/web/runtime',
  '/console/api/digital-humans/dh/runtime',
  '/api/v1/digital-human-webapps/web',
]) {
  const transport = new AgentRuntimeTransport({
    runtimeBasePath: base,
    chatPath: `${base}/chat`,
    client,
  });
  const controller = new globalThis.AbortController();
  assert.equal(await transport.readExecutionFile('conv/id', 'file/id', controller.signal), blob);
  const call = calls.at(-1);
  assert.equal(call.path, `${base}/conversations/conv%2Fid/files/file%2Fid`);
  assert.equal(call.options.responseType, 'blob');
  assert.equal(call.options.retryAttemptsOverride, 0);
  assert.equal(call.options.signal, controller.signal);
  assert.ok(!call.path.includes('sign='));
}

const files = load(
  'src/components/chat/controllers/aichat/reducers/files.ts',
  {
    './shared': { isStaleAIChatStreamEvent: () => false },
  },
  '\nmodule.exports.normalize = normalizeSkillArtifactFile;'
);
const payload = {
  skill_id: 'agent_execution',
  tool_name: 'workspace_export_file',
  file_id: 'private-file',
  filename: 'result.csv',
  extension: '.csv',
  mime_type: 'text/csv',
  size: 12,
  access_mode: 'authenticated',
};
const normalized = files.normalize(payload);
assert.equal(normalized.access_mode, 'authenticated');
assert.equal(normalized.url, '');
assert.equal(normalized.download_url, undefined);
assert.ok(
  files.normalize({
    ...payload,
    file: {
      ...payload,
      url: 'https://untrusted.invalid',
      download_url: 'https://untrusted.invalid',
    },
  }).url === ''
);
assert.equal(
  files.normalize({ ...payload, access_mode: undefined }),
  null,
  'legacy URL requirement must remain'
);

const card = read('src/components/chat/variants/aichat/protected-generated-file-card.tsx');
assert.ok(
  !card.includes('file.url') && !card.includes('file.download_url'),
  'protected cards must never consume metadata URLs'
);
assert.ok(
  card.includes('gcTime: 0') && card.includes('URL.revokeObjectURL'),
  'preview blobs must have bounded lifetimes'
);
assert.ok(
  card.includes('readFile(conversationId, file.file_id)'),
  'download must reauthorize, not reuse the preview blob'
);
assert.ok(card.includes('allowExternalHtmlOpen={false}'), 'protected HTML must stay sandboxed');
assert.ok(card.includes('previewInstance'), 'preview cache must be local to the mounted card');
for (const [application, directory] of [
  ['agents', 'src/components/agents/api/md/agent-api'],
  ['digital-humans', 'src/components/digital-human/md/digital-human-api'],
]) {
  for (const locale of ['en-US', 'zh-Hans']) {
    const documentation = read(`${directory}.${locale}.mdx`);
    assert.ok(documentation.includes(`url="/api/v1/${application}/conversations/{conversation_id}/files/{file_id}"`));
    assert.ok(documentation.includes('access_mode: authenticated'));
    assert.ok(documentation.includes('Cache-Control: private, no-store'));
  }
}
console.log('Protected execution file transport and artifact normalization passed');
