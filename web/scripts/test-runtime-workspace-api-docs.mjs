// Every code block of the external access API docs sits in a <CodeGroup>:
// a bare fenced block renders as inline code, one grey strip per line,
// without highlighting or a copy button.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const docs = [
  '../src/features/runtime-workspaces/runtime-workspace-api.zh-Hans.mdx',
  '../src/features/runtime-workspaces/runtime-workspace-api.en-US.mdx',
];

for (const doc of docs) {
  const lines = readFileSync(fileURLToPath(new URL(doc, import.meta.url)), 'utf8').split(/\r?\n/);
  let group = false;
  let fence = false;
  const bare = [];
  lines.forEach((line, index) => {
    if (fence) {
      if (line.startsWith('```')) fence = false;
      return;
    }
    if (line.startsWith('<CodeGroup')) group = true;
    else if (line.startsWith('</CodeGroup>')) group = false;
    else if (line.startsWith('```')) {
      fence = true;
      if (!group) bare.push(index + 1);
    }
  });
  assert.equal(fence, false, `${doc} has an unclosed code block`);
  assert.deepEqual(
    bare,
    [],
    `${doc} has code blocks outside <CodeGroup> at lines ${bare.join(', ')}`
  );
}

console.log('runtime workspace API docs: every code block is in a CodeGroup');
