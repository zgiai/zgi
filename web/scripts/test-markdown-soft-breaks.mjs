import assert from 'node:assert/strict';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import ReactMarkdown from 'react-markdown';
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

console.log('Markdown soft-break regression checks passed.');
