import { readFileSync } from 'node:fs';

const source = readFileSync('src/components/aichat/contextual/context-envelope.ts', 'utf8');

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const forbiddenModelContext = [
  'ZGI AIChat system assistant',
  'AIChat is the ZGI sidebar operation assistant',
  'AIChat role: ZGI sidebar system assistant',
  'Describe AIChat as a ZGI operation assistant',
  'Current ZGI page context',
];

for (const phrase of forbiddenModelContext) {
  assert(!source.includes(phrase), `Context envelope must not expose branded identity: ${phrase}`);
}

assert(
  source.includes(
    "const PLATFORM_OPERATION_ASSISTANT_CONTEXT_ITEM_ID = 'assistant.platform_operation';"
  ),
  'The platform operation assistant must use a brand-neutral context resource ID.'
);
assert(
  source.includes("title: 'Platform operation assistant'"),
  'The model-visible context resource must use the neutral platform operation assistant title.'
);
assert(
  source.includes('item.id !== LEGACY_SYSTEM_CONTEXT_ITEM_ID'),
  'Legacy branded context resources must be removed when the current envelope is assembled.'
);

console.log('Contextual assistant brand neutrality checks passed.');
