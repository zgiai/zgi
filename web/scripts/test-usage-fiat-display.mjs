import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';

import enMessages from '../src/i18n/modules/dashboard/en-US.ts';
import zhHansMessages from '../src/i18n/modules/dashboard/zh-Hans.ts';

function collectStrings(value) {
  if (typeof value === 'string') return [value];
  if (!value || typeof value !== 'object') return [];
  return Object.values(value).flatMap(collectStrings);
}

for (const [locale, usageMessages] of [
  ['en-US', enMessages.usage],
  ['zh-Hans', zhHansMessages.usage],
]) {
  const visibleCopy = collectStrings(usageMessages).join('\n');

  assert.doesNotMatch(
    visibleCopy,
    /点数|\bpoints?\b|\bcredits?\b/i,
    `${locale} usage copy must describe fiat cost instead of internal credits`
  );
}

const usageSurfaceFiles = [
  '../src/app/dashboard/page.tsx',
  '../src/components/usage/stats-cards.tsx',
  '../src/components/usage/token-trend-chart.tsx',
  '../src/components/usage/model-details-section.tsx',
  '../src/components/usage/app-type-distribution-section.tsx',
];
const usageSurfaceSource = (
  await Promise.all(usageSurfaceFiles.map(file => readFile(new URL(file, import.meta.url), 'utf8')))
).join('\n');

assert.doesNotMatch(
  usageSurfaceSource,
  /formatAiCreditValue/,
  'every usage cost surface must use the organization fiat formatter'
);
assert.match(
  await readFile(new URL('../src/app/dashboard/page.tsx', import.meta.url), 'utf8'),
  /formatBillingDisplayAmountFromNormalizedCredits/,
  'the dashboard usage summary must not label raw internal credits as total cost'
);

console.log('Usage fiat display copy checks passed.');
