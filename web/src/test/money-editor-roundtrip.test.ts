import assert from 'node:assert/strict';

// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { addMoney, convertMoney, currencySymbol, divMoney, formatMoney, mulMoney, parseMoney, quantizeMoney, subMoney } from '../utils/money.ts';
// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { billingDisplayInputToUSD, billingDisplayInputValueFromUSD } from '../utils/billing-display.ts';
// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { buildCustomModelPricing, readCustomModelPricePerImage } from '../utils/build-custom-model-pricing.ts';

// Frozen editor round-trip vectors, mirrored from rating-vectors-v1.json
// (editor_roundtrip_vectors R001-R003). Any change here requires a spec bump.
const editorRoundtripVectors = [
  {
    id: 'R001',
    currency: 'CNY',
    rate: '7',
    submittedAmount: '7',
    expectStoredUsd: '1',
    expectPopulatedAmount: '7',
    expectResubmitStoredUsd: '1',
  },
  {
    id: 'R002',
    currency: 'CNY',
    rate: '6',
    submittedAmount: '6',
    expectStoredUsd: '1',
    expectPopulatedAmount: '6',
    expectResubmitStoredUsd: '1',
  },
  {
    id: 'R003',
    currency: 'USD',
    rate: '7',
    submittedAmount: '1.6667',
    expectStoredUsd: '1.6667',
    expectPopulatedAmount: '1.6667',
    expectResubmitStoredUsd: '1.6667',
  },
] as const;

for (const vector of editorRoundtripVectors) {
  const settings = { currency: vector.currency, usdToCnyRate: Number(vector.rate) };
  const storedUsd = billingDisplayInputToUSD(vector.submittedAmount, settings);
  assert.equal(storedUsd, vector.expectStoredUsd, vector.id + ' submit must store canonical USD');

  const populated = billingDisplayInputValueFromUSD(storedUsd, true, settings);
  assert.equal(populated, vector.expectPopulatedAmount, vector.id + ' populate must show display currency');

  const resubmitted = billingDisplayInputToUSD(populated, settings);
  assert.equal(
    resubmitted,
    vector.expectResubmitStoredUsd,
    vector.id + ' resubmit must match the frozen vector'
  );
  assert.equal(resubmitted, storedUsd, vector.id + ' submit -> populate -> submit must be identical');
}

// The same invariant across every currency/rate combination the frozen vectors
// cover: an extra conversion must never divide the canonical USD amount twice.
for (const currency of ['USD', 'CNY'] as const) {
  for (const rate of [6, 7]) {
    const settings = { currency, usdToCnyRate: rate };
    const stored = billingDisplayInputToUSD('10', settings);
    const populated = billingDisplayInputValueFromUSD(stored, true, settings);
    const resubmitted = billingDisplayInputToUSD(populated, settings);
    assert.equal(resubmitted, stored, currency + ' @ ' + rate + ' must be idempotent');

    // A second submit of the already-canonical value must not drift either.
    const doubleSubmit = billingDisplayInputToUSD(
      billingDisplayInputValueFromUSD(resubmitted, true, settings),
      settings
    );
    assert.equal(doubleSubmit, stored, currency + ' @ ' + rate + ' must not double-convert');
  }
}

// CNY rate 7: a stored 1 USD must never be divided again into 0.142857... .
assert.equal(
  billingDisplayInputToUSD(billingDisplayInputValueFromUSD('1', true, { currency: 'CNY', usdToCnyRate: 7 }), {
    currency: 'CNY',
    usdToCnyRate: 7,
  }),
  '1',
  'resubmitting a populated CNY price must not divide the USD amount a second time'
);

// Media prices share the same populate/submit symmetry as token prices.
const cnySettings = { currency: 'CNY' as const, usdToCnyRate: 7 };
const storedPerImage = billingDisplayInputToUSD('1', cnySettings);
assert.equal(storedPerImage, '0.142857142857', 'CNY media price must store exactly one division');
const populatedPerImage = billingDisplayInputValueFromUSD(storedPerImage, true, cnySettings);
assert.equal(populatedPerImage, '1', 'stored media price must populate back to the CNY input');

const mediaPricing = buildCustomModelPricing({
  useCases: ['image-gen'],
  currency: cnySettings.currency,
  pricePerImage: storedPerImage,
});
assert.equal(
  readCustomModelPricePerImage(mediaPricing),
  '0.142857142857',
  'media pricing must keep the canonical USD amount'
);
assert.equal(mediaPricing?.price_per_image, 0.142857142857, 'per-image price crosses the wire as USD');

// The pricing document declares its canonical currency explicitly on every
// projection so the backend never has to infer (and never converts twice).
const videoPricing = buildCustomModelPricing({
  useCases: ['video-gen'],
  currency: 'CNY',
  videoRates: [{ resolution: '720p', pricePerSecond: '0.8' }],
});
assert.deepEqual(
  videoPricing?.video_generation,
  { currency: 'USD', billing_unit: 'second', rates: [{ resolution: '720p', price_per_second: 0.8 }] },
  'video pricing must carry the explicit canonical USD currency'
);
const meteredPricing = buildCustomModelPricing({
  useCases: ['speech-to-text'],
  currency: 'CNY',
  speechToTextPricePerHour: '2.52',
});
assert.deepEqual(
  (meteredPricing?.metered as Array<{ price: { currency: string } }>)[0].price.currency,
  'USD',
  'metered pricing must carry the explicit canonical USD currency'
);

// money.ts primitives stay exact decimal strings.
assert.equal(parseMoney('abc'), null, 'invalid money must not parse');
assert.equal(addMoney('0.1', '0.2'), '0.3', 'decimal addition must avoid float drift');
assert.equal(subMoney('0.3', '0.1'), '0.2', 'decimal subtraction must stay exact');
assert.equal(mulMoney('1.428571428571', '7'), '9.999999999997', 'decimal multiplication is exact');
assert.equal(divMoney('1', '7'), '0.14285714285714285714', 'decimal division keeps full precision');
assert.equal(quantizeMoney('1.428571428571428571', 12), '1.428571428571', 'quantize rounds once');
assert.equal(
  convertMoney('7', { from: 'CNY', to: 'USD', usdToCnyRate: '7' }),
  '1',
  'CNY -> USD divides by the current rate once'
);
assert.equal(
  convertMoney('1', { from: 'USD', to: 'CNY', usdToCnyRate: '6' }),
  '6',
  'USD -> CNY multiplies by the current rate once'
);
assert.throws(
  () => convertMoney('1', { from: 'USD', to: 'CNY', usdToCnyRate: '0' }),
  /exchange rate/i,
  'a missing exchange rate must fail closed'
);
assert.equal(currencySymbol('CNY'), '¥', 'CNY must use the yuan symbol');
assert.equal(formatMoney('1.5', 'CNY', { estimate: true }), '≈¥1.50', 'CNY estimates carry ≈');

console.log('Money editor round-trip tests passed.');
