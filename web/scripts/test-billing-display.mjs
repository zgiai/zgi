import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';

import {
  DEFAULT_BILLING_DISPLAY,
  billingDisplayInputToUSD,
  billingDisplayInputValueFromUSD,
  formatBillingDisplayAmountFromUSD,
  formatBillingDisplayAmountFromNormalizedCredits,
  getBillingDisplaySettings,
  normalizedAiCreditsToUSD,
} from '../src/utils/billing-display.ts';
import { normalizeAiCreditValue, normalizeModelUsageData } from '../src/utils/ai-credits.ts';
import { formatTokenCount } from '../src/utils/token-format.ts';

const usdSettings = {
  currency: 'USD',
  usdToCnyRate: 7,
};

const cnySettings = {
  currency: 'CNY',
  usdToCnyRate: 7,
};

assert.equal(
  formatTokenCount(1_056, 'en-US'),
  '1,056',
  'usage token counts must use grouping separators without K/M abbreviation'
);
assert.equal(formatTokenCount(0, 'en-US'), '0', 'zero token usage must remain visible');

assert.deepEqual(
  getBillingDisplaySettings(),
  DEFAULT_BILLING_DISPLAY,
  'organizations without display settings must use the documented USD default'
);
assert.deepEqual(
  getBillingDisplaySettings({
    billing_display_currency: 'CNY',
    usd_to_cny_rate: '7.2',
  }),
  { currency: 'CNY', usdToCnyRate: 7.2 },
  'the organization exchange rate may arrive as a database numeric string'
);
assert.deepEqual(
  getBillingDisplaySettings({
    billing_display_currency: 'CNY',
    usd_to_cny_rate: 0,
  }),
  { currency: 'CNY', usdToCnyRate: 7 },
  'invalid organization exchange rates must fall back to the documented default'
);

assert.equal(
  billingDisplayInputValueFromUSD(1, true, cnySettings),
  '7',
  'USD token prices must use the organization exchange rate for CNY inputs'
);
assert.equal(
  billingDisplayInputToUSD('7', cnySettings),
  '1',
  'CNY token price inputs must be converted back to canonical USD before saving'
);
assert.equal(
  billingDisplayInputValueFromUSD(0, true, cnySettings),
  '0',
  'configured zero token prices must remain visible in CNY mode'
);
assert.equal(
  billingDisplayInputToUSD('', cnySettings),
  '',
  'empty token price inputs must stay empty so validation can reject them'
);
assert.equal(
  billingDisplayInputValueFromUSD(0.0001806, true, usdSettings),
  '0.0001806',
  'model price inputs must preserve prices beyond six decimal places'
);
assert.equal(
  billingDisplayInputToUSD('0.0012642', cnySettings),
  '0.0001806',
  'CNY model price conversion must preserve the canonical USD decimal value'
);
assert.equal(
  formatBillingDisplayAmountFromUSD(0.0001806, usdSettings),
  '$0.0001806',
  'small model prices must be displayed directly without truncation'
);
assert.equal(
  formatBillingDisplayAmountFromUSD('0.000167591', usdSettings),
  '$0.000167591',
  'exact settled USD strings must not be reconstructed from rounded integer credits'
);

assert.equal(
  normalizedAiCreditsToUSD(9_661.55),
  9.66155,
  'normalized frontend credits must be divided by 1,000 exactly once'
);
for (const invalidValue of [undefined, null, Number.NaN, Number.POSITIVE_INFINITY, -1]) {
  assert.equal(
    normalizedAiCreditsToUSD(invalidValue),
    null,
    'invalid or negative normalized credits must not be presented as money'
  );
}
assert.equal(normalizedAiCreditsToUSD(0), 0, 'zero usage must remain an exact zero');

assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(9_661.55, usdSettings, { locale: 'en-US' }),
  '$9.66',
  'USD usage cost should be formatted from normalized frontend credits'
);

assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(9_661.55, cnySettings, { locale: 'en-US' }),
  '≈¥67.63',
  'CNY usage cost should use the organization exchange rate and remain marked as an estimate'
);

assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(0.01, cnySettings, { locale: 'zh-CN' }),
  '≈¥0.00007',
  'a non-zero CNY amount must be displayed directly below the former threshold'
);

assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(0.05, usdSettings, { locale: 'en-US' }),
  '$0.00005',
  'non-zero usage must be displayed directly below the former threshold'
);
assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(0.1, usdSettings, { locale: 'en-US' }),
  '$0.0001',
  'the smallest exact visible amount must not use the less-than marker'
);
assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(0, usdSettings, { locale: 'en-US' }),
  '$0.00',
  'zero usage must be visibly distinct from missing usage'
);
assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(1_234_567_890, usdSettings, {
    locale: 'en-US',
  }),
  '$1,234,567.89',
  'large usage amounts must keep grouping separators and cents'
);
assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(
    1,
    { currency: 'CNY', usdToCnyRate: Number.NaN },
    { locale: 'en-US' }
  ),
  '-',
  'a CNY amount without a valid exchange rate must stay unknown'
);
assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(
    Number.MAX_VALUE,
    { currency: 'CNY', usdToCnyRate: Number.MAX_VALUE },
    { locale: 'en-US' }
  ),
  '-',
  'a finite input that overflows during currency conversion must stay unknown'
);

assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(Number.NaN, usdSettings),
  '-',
  'invalid usage values must stay unknown instead of being presented as zero'
);

const normalizedUsage = normalizeModelUsageData({
  period: { start_time: 0, end_time: 1 },
  summary: {
    attempt_count: 2,
    success_count: 2,
    failed_count: 0,
    partial_count: 0,
    prompt_tokens: 1_000,
    completion_tokens: 500,
    total_tokens: 1_500,
    official_points: 273_000_000,
    private_points: 171_500_000,
    total_points: 444_500_000,
  },
  by_model: [
    {
      model_id: 'official-model',
      model_name: 'official-model',
      provider_id: 'provider',
      provider_name: 'provider',
      attempt_count: 1,
      success_count: 1,
      failed_count: 0,
      partial_count: 0,
      prompt_tokens: 600,
      completion_tokens: 300,
      total_tokens: 900,
      official_points: 273_000_000,
      private_points: 0,
      total_points: 273_000_000,
      points_share: 61.42,
    },
    {
      model_id: 'private-model',
      model_name: 'private-model',
      provider_id: 'provider',
      provider_name: 'provider',
      attempt_count: 1,
      success_count: 1,
      failed_count: 0,
      partial_count: 0,
      prompt_tokens: 400,
      completion_tokens: 200,
      total_tokens: 600,
      official_points: 0,
      private_points: 171_500_000,
      total_points: 171_500_000,
      points_share: 38.58,
    },
  ],
  by_app_type: [],
  daily_trend: [
    {
      date: '2026-07-23',
      attempt_count: 2,
      success_count: 2,
      failed_count: 0,
      partial_count: 0,
      prompt_tokens: 1_000,
      completion_tokens: 500,
      total_tokens: 1_500,
      official_tokens: 900,
      private_tokens: 600,
      official_points: 273_000_000,
      private_points: 171_500_000,
      total_points: 444_500_000,
    },
  ],
});

assert.equal(
  normalizeAiCreditValue(9_661_550, { precision: 3 }),
  9_661.55,
  'the API-to-frontend normalization boundary must divide raw points by 1,000'
);
assert.deepEqual(
  {
    official: normalizedUsage.summary.official_points,
    private: normalizedUsage.summary.private_points,
    total: normalizedUsage.summary.total_points,
  },
  { official: 273_000, private: 171_500, total: 444_500 },
  'all usage lanes must be normalized with the same scale'
);
assert.equal(
  normalizedUsage.summary.official_points + normalizedUsage.summary.private_points,
  normalizedUsage.summary.total_points,
  'normalization must preserve the official + private = total invariant'
);
assert.deepEqual(
  {
    official: formatBillingDisplayAmountFromNormalizedCredits(
      normalizedUsage.summary.official_points,
      cnySettings,
      { locale: 'en-US' }
    ),
    private: formatBillingDisplayAmountFromNormalizedCredits(
      normalizedUsage.summary.private_points,
      cnySettings,
      { locale: 'en-US' }
    ),
    total: formatBillingDisplayAmountFromNormalizedCredits(
      normalizedUsage.summary.total_points,
      cnySettings,
      { locale: 'en-US' }
    ),
  },
  {
    official: '≈¥1,911.00',
    private: '≈¥1,200.50',
    total: '≈¥3,111.50',
  },
  'raw API points must become the expected CNY usage amounts without a double conversion'
);

const pricingFallbackSource = await readFile(
  new URL('../src/components/settings/pricing-fallback-panel.tsx', import.meta.url),
  'utf8'
);
const [enSettingsSource, zhHansSettingsSource] = await Promise.all([
  readFile(new URL('../src/i18n/modules/settings/en-US.ts', import.meta.url), 'utf8'),
  readFile(new URL('../src/i18n/modules/settings/zh-Hans.ts', import.meta.url), 'utf8'),
]);
const tokenPriceCellSource = pricingFallbackSource.slice(
  pricingFallbackSource.indexOf('function TokenPriceCell'),
  pricingFallbackSource.indexOf('function ImagePriceCell')
);

assert.match(
  pricingFallbackSource,
  /useOrganizationStore\.use\.currentOrganization\(\)/,
  'pricing fallback rules must use the current organization display settings'
);
assert.match(
  pricingFallbackSource,
  /billingDisplayInputValueFromUSD/,
  'pricing fallback rules must convert canonical USD prices into display input values'
);
assert.match(
  tokenPriceCellSource,
  /tokenPriceDisplayValue/,
  'token price cells must consume the converted display input value'
);
assert.match(
  tokenPriceCellSource,
  /billingDisplayInputToUSD/,
  'token price cells must convert display input values back to canonical USD'
);
assert.doesNotMatch(
  tokenPriceCellSource,
  /value=\{displayRule\.price_usd_per_1m_tokens \?\? ''\}/,
  'token price inputs must not bind directly to canonical USD values'
);
assert.match(
  pricingFallbackSource,
  /settings\.pricingFallback\.cnyPerMillionShort/,
  'pricing fallback token units must support CNY display mode'
);
assert.match(
  zhHansSettingsSource,
  /cnyPerMillionShort: '人民币 \/ 100 万 token'/,
  'the Chinese token price unit must follow CNY display mode'
);
assert.match(
  enSettingsSource,
  /cnyPerMillionShort: 'CNY \/ 1M tokens'/,
  'the English token price unit must follow CNY display mode'
);

console.log('Billing display checks passed.');
