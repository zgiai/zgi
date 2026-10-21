import assert from 'node:assert/strict';
import { Blob } from 'node:buffer';
import { load, read, harness, jsx, nodes, flush } from './connector-fixture.mjs';

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

// Exercise the real component handler, not just its source. Browser-save proof
// is a separate real acceptance gate; these mocks cannot establish delivery.
const originalDocument = globalThis.document;
const originalWindow = globalThis.window;
const { URL } = globalThis;
const originalCreateURL = URL.createObjectURL;
const originalRevokeURL = URL.revokeObjectURL;
try {
  const created = [];
  const revoked = [];
  const anchors = [];
  const timers = [];
  const errors = [];
  URL.createObjectURL = value => {
    created.push(value);
    return `blob:owned-${created.length}`;
  };
  URL.revokeObjectURL = value => revoked.push(value);
  globalThis.document = {
    createElement(tag) {
      assert.equal(tag, 'a');
      const anchor = {
        clicks: 0,
        click() {
          this.clicks++;
        },
      };
      anchors.push(anchor);
      return anchor;
    },
  };
  globalThis.window = { setTimeout: callback => timers.push(callback) };
  const instance = harness(
    react =>
      load('src/components/chat/variants/aichat/protected-generated-file-card.tsx', {
        react,
        'react/jsx-runtime': jsx,
        '@tanstack/react-query': { useQuery() {} },
        'lucide-react': {},
        sonner: { toast: { error: value => errors.push(value) } },
        '@/components/ui/button': { Button: 'button' },
        '@/components/ui/dialog': {},
        '@/components/files/universal-file-preview-dialog': {},
        '@/i18n/translations': { useT: () => key => key },
      }).ProtectedGeneratedFileCard
  );
  const pendingReads = [];
  const props = {
    file: { ...normalized, filename: 'private-result.txt' },
    conversationId: 'owned-conversation',
    readFile: (conversationId, fileId) =>
      new Promise((resolve, reject) => {
        pendingReads.push({ conversationId, fileId, resolve, reject });
      }),
  };
  const button = (tree, label) => nodes(tree).find(node => node.props['aria-label'] === label);
  const downloadLabel = 'consoleChat.attachments.downloadGeneratedFile';
  button(instance.render(props), 'consoleChat.attachments.previewGeneratedFile').props.onClick();
  button(instance.render(props), downloadLabel).props.onClick();
  assert.equal(pendingReads.length, 1);
  assert.equal(pendingReads[0].conversationId, props.conversationId);
  assert.equal(pendingReads[0].fileId, normalized.file_id);
  const busy = button(instance.render(props), downloadLabel);
  assert.equal(busy.props.disabled, true);
  busy.props.onClick();
  assert.equal(pendingReads.length, 1, 'in-flight download must not start another read');
  assert.equal(created.length, 0, 'no URL before the authenticated read succeeds');
  pendingReads[0].resolve(blob);
  await flush();
  assert.equal(created[0], blob);
  assert.equal(anchors[0].href, 'blob:owned-1');
  assert.equal(anchors[0].download, 'private-result.txt');
  assert.equal(anchors[0].clicks, 1);
  assert.equal(revoked.length, 0, 'URL must not be revoked before click handoff');
  timers.shift()();
  assert.deepEqual(revoked, ['blob:owned-1']);
  button(instance.render(props), downloadLabel).props.onClick();
  assert.equal(pendingReads.length, 2, 'repeated download must reauthorize');
  pendingReads[1].reject(new Error('private authorization detail'));
  await flush();
  assert.deepEqual(errors, ['consoleChat.attachments.protectedFileReadFailed']);
  assert.equal(created.length, 1, 'denied read must not create a download URL');
  assert.equal(button(instance.render(props), downloadLabel).props.disabled, false);
  for (const unavailable of [
    { ...props, file: { ...props.file, availability: 'expired' } },
    { ...props, file: { ...props.file, expires_at: 1 } },
    { ...props, conversationId: '' },
    { ...props, readFile: undefined },
  ]) {
    const disabled = button(instance.render(unavailable), downloadLabel);
    assert.equal(disabled.props.disabled, true);
    disabled.props.onClick();
  }
  assert.equal(pendingReads.length, 2, 'unavailable files must not be read');
  instance.unmount();
} finally {
  URL.createObjectURL = originalCreateURL;
  URL.revokeObjectURL = originalRevokeURL;
  if (originalDocument === undefined) delete globalThis.document;
  else globalThis.document = originalDocument;
  if (originalWindow === undefined) delete globalThis.window;
  else globalThis.window = originalWindow;
}
for (const [application, directory] of [
  ['agents', 'src/components/agents/api/md/agent-api'],
  ['digital-humans', 'src/components/digital-human/md/digital-human-api'],
]) {
  for (const locale of ['en-US', 'zh-Hans']) {
    const documentation = read(`${directory}.${locale}.mdx`);
    assert.ok(
      documentation.includes(
        `url="/api/v1/${application}/conversations/{conversation_id}/files/{file_id}"`
      )
    );
    assert.ok(documentation.includes('access_mode: authenticated'));
    assert.ok(documentation.includes('Cache-Control: private, no-store'));
  }
}
console.log('Protected execution file transport and artifact normalization passed');
