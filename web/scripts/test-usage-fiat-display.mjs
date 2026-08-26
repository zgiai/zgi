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
  const visibleCopy = collectStrings({
    ...usageMessages,
    invocations: {
      ...usageMessages.invocations,
      details: undefined,
    },
  }).join('\n');

  assert.doesNotMatch(
    visibleCopy,
    /点数|\bpoints?\b|\bcredits?\b/i,
    `${locale} non-detail usage copy must describe fiat cost instead of internal credits`
  );
}

const usageSurfaceFiles = [
  '../src/app/dashboard/page.tsx',
  '../src/app/dashboard/usage/overview/page.tsx',
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

const cjkPattern = /[\u3400-\u9fff]/u;
assert.match('查看全部', cjkPattern, 'the CJK regression pattern must detect Chinese UI copy');
assert.doesNotMatch(
  usageSurfaceSource,
  cjkPattern,
  'localized usage surfaces must not hardcode Chinese UI copy'
);

const usageOverviewSource = await readFile(
  new URL('../src/app/dashboard/usage/overview/page.tsx', import.meta.url),
  'utf8'
);
assert.match(
  usageOverviewSource,
  /t\('usage\.filters\.description'\)/,
  'the usage filter description must use the dashboard translation module'
);

const modelDetailsSource = await readFile(
  new URL('../src/components/usage/model-details-section.tsx', import.meta.url),
  'utf8'
);
assert.match(
  modelDetailsSource,
  /t\('usage\.modelDetails\.showAllModels',\s*\{\s*count:/,
  'the model expansion control must translate its interpolated model count'
);
assert.match(
  modelDetailsSource,
  /t\('usage\.modelDetails\.collapse'\)/,
  'the model collapse control must use the dashboard translation module'
);

assert.equal(
  enMessages.usage.filters.description,
  'Select a time range, app type, and model to view matching data.'
);
assert.equal(enMessages.usage.modelDetails.showAllModels, 'View all {count} models');
assert.equal(enMessages.usage.modelDetails.collapse, 'Collapse');
assert.equal(
  zhHansMessages.usage.filters.description,
  '选择时间范围、应用类型和模型后查看对应数据。'
);
assert.equal(zhHansMessages.usage.modelDetails.showAllModels, '查看全部 {count} 个模型');
assert.equal(zhHansMessages.usage.modelDetails.collapse, '收起');

const tokenTrendSource = await readFile(
  new URL('../src/components/usage/token-trend-chart.tsx', import.meta.url),
  'utf8'
);
assert.match(
  tokenTrendSource,
  /dataKey="officialTokens"[\s\S]*stackId="tokens"/,
  'the token trend must render official-channel tokens in the channel stack'
);
assert.match(
  tokenTrendSource,
  /dataKey="privateTokens"[\s\S]*stackId="tokens"/,
  'the token trend must render private-channel tokens in the channel stack'
);
assert.doesNotMatch(
  tokenTrendSource,
  /showStackedTokens\s*\?\s*\(\s*<>/,
  'Recharts bar series must be direct chart children instead of Fragment children'
);

console.log('Usage fiat display copy checks passed.');
