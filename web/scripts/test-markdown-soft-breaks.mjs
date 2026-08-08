import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import ReactMarkdown from 'react-markdown';
import { URL } from 'node:url';
import { remarkSoftBreaks } from '../src/utils/markdown-soft-breaks.ts';

const tree = {
  type: 'root',
  children: [
    {
      type: 'paragraph',
      children: [{ type: 'text', value: 'first line\nsecond line\r\nthird line' }],
    },
    { type: 'code', value: 'const a = 1;\nconst b = 2;' },
    {
      type: 'paragraph',
      children: [{ type: 'inlineCode', value: 'a\nb' }],
    },
  ],
};

remarkSoftBreaks()(tree);

assert.deepEqual(tree.children, [
  {
    type: 'paragraph',
    children: [
      { type: 'text', value: 'first line' },
      { type: 'break' },
      { type: 'text', value: 'second line' },
      { type: 'break' },
      { type: 'text', value: 'third line' },
    ],
  },
  { type: 'code', value: 'const a = 1;\nconst b = 2;' },
  {
    type: 'paragraph',
    children: [{ type: 'inlineCode', value: 'a\nb' }],
  },
]);

const rendered = renderToStaticMarkup(
  React.createElement(ReactMarkdown, {
    remarkPlugins: [remarkSoftBreaks],
    children: 'first line\nsecond line',
  })
);
assert.match(rendered, /first line<br\s*\/?>(?:\n)?second line/);

const markdownViewerSource = await readFile(
  new URL('../src/components/common/markdown-viewer.tsx', import.meta.url),
  'utf8'
);
assert.doesNotMatch(
  markdownViewerSource,
  /whitespace-pre-wrap/,
  'block-level Markdown whitespace must not create visible blank lines'
);
assert.match(
  markdownViewerSource,
  /const remarkPluginsList:[^\n]*\[[^\]]*remarkSoftBreaks/,
  'soft source line breaks must remain explicit Markdown break nodes'
);

console.log('Markdown soft-break regression checks passed.');
