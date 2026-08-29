import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const timelinePath = path.join(root, 'src/components/chat/variants/aichat/agentic-timeline.tsx');
const timelineSource = readFileSync(timelinePath, 'utf8');
const rowStart = timelineSource.indexOf('function MemoryTimelineRow(');
const rowEnd = timelineSource.indexOf('\nfunction governanceDecisionStatus(', rowStart);

assert.notEqual(rowStart, -1, 'MemoryTimelineRow must exist.');
assert.notEqual(rowEnd, -1, 'MemoryTimelineRow must have a stable function boundary.');

const memoryRowSource = timelineSource.slice(rowStart, rowEnd);

assert.doesNotMatch(
  memoryRowSource,
  /<button|onClick=|aria-expanded=|<ChevronDown|setIsOpen/,
  'Memory status rows must not expose an empty expandable interaction.'
);
assert.match(
  memoryRowSource,
  /item\.event\.display_name \|\| item\.event\.key/,
  'Memory status rows should continue to prefer the configured display name.'
);
assert.match(
  timelineSource,
  /content: showMemoryKey \? undefined : item\.event\.content_preview \|\| item\.event\.content/,
  'Editor memory status rows should not duplicate memory content in their title.'
);

console.log('AIChat memory event interaction checks passed.');
