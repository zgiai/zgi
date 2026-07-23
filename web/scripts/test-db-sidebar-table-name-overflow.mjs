import { readFileSync } from 'node:fs';

const source = readFileSync('src/app/console/db/[dbId]/layout.tsx', 'utf8');

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

assert(
  source.includes('className="min-w-0 overflow-hidden pl-3 space-y-0.5"'),
  'The table list must constrain long names to the sidebar width.'
);

assert(
  source.includes(
    'className="relative flex w-full min-w-0 items-center justify-center gap-1 overflow-hidden group"'
  ),
  'Each table row must prevent its flex children from overflowing the sidebar.'
);

assert(
  source.match(/h-7 w-0 min-w-0 grow/g)?.length === 2,
  'Both table links and non-link labels must shrink from the available row width.'
);

console.log('Database sidebar table-name overflow checks passed.');
