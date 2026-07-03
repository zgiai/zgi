import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');

const read = relativePath => fs.readFileSync(path.join(root, relativePath), 'utf8');

const source = read('src/components/files/detail/file-qa-panel.tsx');

const requiredTextareaSnippets = [
  'border-muted-foreground/20',
  'bg-muted/10',
  'shadow-[0_1px_3px_rgba(15,23,42,0.08)]',
  'hover:border-primary/30',
  'focus-visible:border-primary/50',
  'focus-visible:ring-2',
  'focus-visible:ring-primary/10',
  'placeholder:text-muted-foreground/75',
];

for (const snippet of requiredTextareaSnippets) {
  if (!source.includes(snippet)) {
    throw new Error(`Missing subtle QA input affordance class: ${snippet}`);
  }
}

const requiredButtonSnippets = [
  'border-primary/25',
  'shadow-[0_1px_3px_rgba(15,23,42,0.08)]',
  'disabled:shadow-none',
];

for (const snippet of requiredButtonSnippets) {
  if (!source.includes(snippet)) {
    throw new Error(`Missing matching QA send button affordance class: ${snippet}`);
  }
}

console.log('File QA input affordance checks passed.');
