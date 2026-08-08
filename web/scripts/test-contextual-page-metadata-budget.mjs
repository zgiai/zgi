import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const formatterSource = readFileSync(
  'src/components/aichat/contextual/format-context-metadata.ts',
  'utf8'
);
const skillPageSource = readFileSync(
  'src/components/dashboard/organization/aichat-skill-settings-section.tsx',
  'utf8'
);

assert.match(formatterSource, /MAX_CONTEXT_METADATA_KEYS = 24/);
assert.match(formatterSource, /MAX_CONTEXT_METADATA_LENGTH = 1600/);
assert.match(formatterSource, /parts\.length >= MAX_CONTEXT_METADATA_KEYS/);
assert.match(formatterSource, /nextLength > MAX_CONTEXT_METADATA_LENGTH/);
assert.match(
  formatterSource,
  /sanitizeAIChatContextText\(`\$\{prefix\}\$\{value \?\? ''\}`\)/,
  'metadata keys and values must be sanitized together'
);

for (const key of [
  'visible_skill_count',
  'enabled_skill_count',
  'invalid_skill_count',
  'dependency_unavailable_count',
  'scenario_filter',
  'capability_filter',
  'source_filter',
  'status_filter',
  'dependency_filter',
  'auto_save_status',
  'selected_skill_name',
  'selected_skill_status',
  'selected_skill_dependency_availability',
]) {
  assert.match(skillPageSource, new RegExp(`${key}:`), `missing ${key}`);
}

console.log('Contextual page metadata budget checks passed.');
