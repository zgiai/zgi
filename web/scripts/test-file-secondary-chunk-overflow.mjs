import { readFileSync } from 'node:fs';

const source = readFileSync('src/components/files/detail/file-chunks-panel.tsx', 'utf8');

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

assert(
  source.includes('className="min-w-0 basis-full pt-1"'),
  'Secondary chunk flex container should be allowed to shrink inside the parent row.'
);

assert(
  source.includes('className="min-w-0 space-y-3"'),
  'Secondary chunk list should constrain child rows to the available width.'
);

assert(
  source.includes('[overflow-wrap:anywhere]'),
  'Secondary chunk content should wrap long image markdown URLs instead of overflowing.'
);

console.log('File secondary chunk overflow checks passed.');
