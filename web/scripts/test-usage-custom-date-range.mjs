import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';

import enUS from '../src/i18n/modules/dashboard/en-US.ts';
import zhHans from '../src/i18n/modules/dashboard/zh-Hans.ts';
import { isCustomDateRangeValid } from '../src/utils/usage-date-range.ts';

assert.equal(
  isCustomDateRangeValid('', '2026-07-01'),
  false,
  'a custom range must require both dates'
);
assert.equal(
  isCustomDateRangeValid('2026-07-10', '2026-07-01'),
  false,
  'a custom range must reject an end date earlier than its start date'
);
assert.equal(
  isCustomDateRangeValid('2026-07-10', '2026-07-10'),
  true,
  'a custom range must allow the same start and end date'
);
assert.equal(
  isCustomDateRangeValid('2026-07-01', '2026-07-10'),
  true,
  'a custom range must allow an end date later than its start date'
);

assert.equal(
  zhHans.usage.filters.dateRangeInvalid,
  '结束日期不能早于开始日期',
  'the Chinese UI must explain why a reversed range is invalid'
);
assert.equal(
  enUS.usage.filters.dateRangeInvalid,
  'End date cannot be earlier than start date',
  'the English UI must explain why a reversed range is invalid'
);

const usageOverviewSource = await readFile(
  new URL('../src/app/dashboard/usage/overview/page.tsx', import.meta.url),
  'utf8'
);
assert.match(
  usageOverviewSource,
  /enabled: isCustomRangeValid/,
  'an invalid custom range must not trigger a usage request'
);
assert.match(
  usageOverviewSource,
  /errorText={[\s\S]*usage\.filters\.dateRangeInvalid/,
  'a reversed custom range must render its validation error'
);

console.log('Usage custom date range checks passed.');
