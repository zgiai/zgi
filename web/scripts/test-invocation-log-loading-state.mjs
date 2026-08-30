import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const hookSource = fs.readFileSync(path.join(root, 'src/hooks/statistics/index.ts'), 'utf8');
const sectionSource = fs.readFileSync(
  path.join(root, 'src/components/usage/invocation-log-section.tsx'),
  'utf8'
);

assert.match(
  hookSource,
  /failureCount >= 1[\s\S]*status === undefined \|\| status === 429 \|\| status >= 500/,
  'the invocation log should retry one transient request failure'
);
assert.match(
  sectionSource,
  /hasInitialLoadError = query\.isError && !query\.data/,
  'an initial request failure must be distinct from a successful empty result'
);
assert.match(
  sectionSource,
  /hasInitialLoadError \?[\s\S]*usage\.invocations\.loadFailedDescription[\s\S]*query\.refetch\(\)[\s\S]*: query\.isLoading \?/,
  'the initial failure must render a retryable inline error before loading or empty states'
);
assert.match(
  sectionSource,
  /!hasInitialLoadError \? \([\s\S]*<CardContent[\s\S]*items\.length === 0/,
  'the empty table must not render for an initial request failure'
);
assert.match(
  sectionSource,
  /billingDisplay\.currency === 'CNY' && summary\?\.total_cost_cny[\s\S]*formatRecordedBillingAmount\(summary\.total_cost_cny, 'CNY'/,
  'the invocation summary must use recorded CNY costs when CNY display is selected'
);
assert.match(
  sectionSource,
  /summary\?\.total_cost_usd[\s\S]*formatRecordedBillingAmountFromUSD\([\s\S]*summary\.total_cost_usd,[\s\S]*billingDisplay\.currency,[\s\S]*billingDisplay\.usdToCnyRate/,
  'the invocation summary must use the current rate when a recorded CNY total is unavailable'
);
assert.match(
  sectionSource,
  /formatBillingDisplayAmountFromNormalizedCredits\(fallbackPoints, billingDisplay/,
  'the invocation summary points fallback must follow the current display currency and rate'
);

console.log('Invocation log loading-state checks passed.');
