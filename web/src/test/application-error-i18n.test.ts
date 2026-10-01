import assert from 'node:assert/strict';

// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import enUS from '../i18n/modules/common/en-US.ts';
// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import zhHans from '../i18n/modules/common/zh-Hans.ts';

// T17 acceptance 7: every code Pre can project to the browser has zh-Hans and
// en-US copy covering both the toast and streaming rendering paths.
const APPLICATION_ERROR_CODES = [
  'billing.organization.balance_insufficient',
  'billing.private_channel.balance_insufficient',
  'billing.workspace.quota_insufficient',
  'billing.model.pricing_not_configured',
  'billing.exchange_rate.unavailable',
  'billing.prededuct.failed',
  'billing.settle.failed',
  'billing.request.invalid',
  'billing.deduction.not_found',
  'billing.deduction.mismatch',
  'billing.deduction.expired',
  'billing.actual_exceeds_reserve',
  'video.request.invalid',
  'video.task.not_found',
  'video.task.failed',
  'video.upstream.timeout',
  'video.runtime.unavailable',
  'video.asset.save_failed',
  'image.request.invalid',
  'image.task.not_found',
  'image.task.conflict',
  'image.task.timeout',
  'image.upstream.failed',
  'image.asset.save_failed',
  'image.runtime.failed',
  'music.request.invalid',
  'music.task.not_found',
  'music.task.conflict',
  'music.model.unavailable',
  'music.task.failed',
  'music.task.not_deletable',
  'llm.provider.none_available',
  'llm.provider.not_found',
  'llm.channel.not_found',
  'llm.provider.unavailable',
  // Upstream 429s are projected as llm.provider.rate_limited.
  'llm.provider.rate_limited',
  'llm.quota.exceeded',
  'llm.balance.insufficient',
] as const;

interface CodeCopy {
  title: string;
  action: string;
}

const enCodes = enUS.requestErrors.codes as unknown as Record<string, CodeCopy>;
const zhCodes = zhHans.requestErrors.codes as unknown as Record<string, CodeCopy>;

for (const code of APPLICATION_ERROR_CODES) {
  // next-intl rejects message keys containing ".", so the catalog stores the frozen
  // application error codes with "__" as the separator.
  const key = code.replace(/\./g, '__');
  const en = enCodes[key];
  const zh = zhCodes[key];
  assert.ok(en && en.title.trim().length > 0, `missing en-US title for ${code}`);
  assert.ok(en && en.action.trim().length > 0, `missing en-US action for ${code}`);
  assert.ok(zh && zh.title.trim().length > 0, `missing zh-Hans title for ${code}`);
  assert.ok(zh && zh.action.trim().length > 0, `missing zh-Hans action for ${code}`);
  assert.notEqual(en.title, zh.title, `code ${code} is not localized`);
}

// The retry/error_id labels exist for both locales so the toast can offer a
// retry entry and display a copyable error_id.
assert.ok(enUS.requestErrors.retry.trim().length > 0);
assert.ok(zhHans.requestErrors.retry.trim().length > 0);
assert.ok(enUS.requestErrors.errorId.trim().length > 0);
assert.ok(zhHans.requestErrors.errorId.trim().length > 0);

console.log('application error i18n catalog OK');
