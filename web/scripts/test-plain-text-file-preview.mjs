import assert from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { load, read, harness, jsx, nodes, flush } from './connector-fixture.mjs';

const { Response, ReadableStream, TextEncoder } = globalThis;

const imports = {
  'react/jsx-runtime': jsx,
  'lucide-react': { AlertCircle: 'alert-icon', Loader2: 'loader-icon' },
  '@/i18n': { useT: () => key => key },
};
const { readBoundedTextPreview, maxTextPreviewBytes } = load(
  'src/components/files/plain-text-file-preview.tsx',
  { ...imports, react: {} }
);
assert.equal(
  await readBoundedTextPreview(
    new Response('Read 1 fields\nSUCCESS: All bounding boxes are valid\n')
  ),
  'Read 1 fields\nSUCCESS: All bounding boxes are valid\n'
);
assert.equal(await readBoundedTextPreview(new Response(null)), '');
assert.equal(
  (await readBoundedTextPreview(new Response('a'.repeat(maxTextPreviewBytes)))).length,
  maxTextPreviewBytes
);
await assert.rejects(
  readBoundedTextPreview(new Response('a'.repeat(maxTextPreviewBytes + 1))),
  RangeError
);
await assert.rejects(
  readBoundedTextPreview(
    new Response('small', { headers: { 'content-length': String(maxTextPreviewBytes + 1) } })
  ),
  RangeError
);
await assert.rejects(readBoundedTextPreview(new Response('denied', { status: 403 })));
let canceled = false;
const oversized = new Response(
  new ReadableStream({
    pull(controller) {
      controller.enqueue(new Uint8Array(maxTextPreviewBytes + 1));
    },
    cancel() {
      canceled = true;
    },
  })
);
await assert.rejects(readBoundedTextPreview(oversized), RangeError);
assert.equal(canceled, true, 'oversized streams must stop reading');
const utf8 = new TextEncoder().encode('你好\n😀');
assert.equal(
  await readBoundedTextPreview(
    new Response(
      new ReadableStream({
        start(controller) {
          for (const byte of utf8) controller.enqueue(new Uint8Array([byte]));
          controller.close();
        },
      })
    )
  ),
  '你好\n😀'
);

const originalFetch = globalThis.fetch;
try {
  const pending = [];
  globalThis.fetch = (url, options) =>
    new Promise(resolve => pending.push({ url, options, resolve }));
  const instance = harness(
    react =>
      load('src/components/files/plain-text-file-preview.tsx', { ...imports, react })
        .PlainTextFilePreview
  );
  assert.ok(
    nodes(instance.render({ previewUrl: 'blob:first' })).some(n => n.props.role === 'status')
  );
  assert.equal(pending[0].options.credentials, 'include');
  const unsafe = '<script>alert("never")</script>\n原文';
  pending[0].resolve(new Response(unsafe));
  await flush();
  const rendered = instance.render({ previewUrl: 'blob:first' });
  assert.equal(rendered.type, 'pre');
  assert.equal(rendered.props.children, unsafe);
  const html = renderToStaticMarkup(createElement('pre', null, rendered.props.children));
  assert.ok(html.includes('&lt;script&gt;') && !html.includes('<script>'));
  assert.ok(
    nodes(instance.render({ previewUrl: 'blob:second' })).some(n => n.props.role === 'status'),
    'old bytes must never appear under a new URL'
  );
  assert.equal(pending[0].options.signal.aborted, true);
  instance.unmount();
  assert.equal(pending[1].options.signal.aborted, true);
  pending[1].resolve(new Response('stale'));
  await flush();

  const failing = harness(
    react =>
      load('src/components/files/plain-text-file-preview.tsx', { ...imports, react })
        .PlainTextFilePreview
  );
  failing.render({ previewUrl: 'blob:denied' });
  pending.at(-1).resolve(new Response('sensitive internal error', { status: 403 }));
  await flush();
  const errorTree = failing.render({ previewUrl: 'blob:denied' });
  assert.ok(nodes(errorTree).some(n => n.props.role === 'alert'));
  assert.ok(!JSON.stringify(errorTree).includes('sensitive internal error'));
  failing.unmount();
} finally {
  globalThis.fetch = originalFetch;
}
const universal = read('src/components/files/universal-file-preview-dialog.tsx');
assert.ok(universal.includes('<PlainTextFilePreview previewUrl={previewUrl} />'));
const textBranch = universal.slice(
  universal.indexOf("if (previewKind === 'browser')"),
  universal.indexOf('function getPreviewFormatLabel')
);
assert.ok(!textBranch.includes('sandbox=""'), 'plain text must not use Blob iframe navigation');
console.log('Bounded text preview, escaping, authorization failure and lifecycle checks passed');
