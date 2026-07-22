import assert from 'node:assert/strict';

import {
  formatBillingDisplayAmountFromNormalizedCredits,
  normalizedAiCreditsToUSD,
} from '../src/utils/billing-display.ts';

const usdSettings = {
  currency: 'USD',
  usdToCnyRate: 7,
};

const cnySettings = {
  currency: 'CNY',
  usdToCnyRate: 7,
};

assert.equal(
  normalizedAiCreditsToUSD(9_661.55),
  9.66155,
  'normalized frontend credits must be divided by 1,000 exactly once'
);

assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(9_661.55, usdSettings),
  '$9.66',
  'USD usage cost should be formatted from normalized frontend credits'
);

assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(9_661.55, cnySettings),
  '≈¥67.63',
  'CNY usage cost should use the organization exchange rate and remain marked as an estimate'
);

assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(0.05, usdSettings),
  '<$0.0001',
  'non-zero usage must not be rounded down to a visible zero'
);

assert.equal(
  formatBillingDisplayAmountFromNormalizedCredits(Number.NaN, usdSettings),
  '-',
  'invalid usage values must stay unknown instead of being presented as zero'
);

console.log('Billing display checks passed.');
