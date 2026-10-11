import assert from 'node:assert/strict';

// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { lowestPrice, marketAmount, marketText } from '../components/zgi-market/market-format.ts';

const text = { 'zh-CN': '搜索', 'en-US': 'Search' };
assert.equal(marketText(text, 'zh-Hans'), '搜索');
assert.equal(marketText(text, 'en-US'), 'Search');
// An empty language falls back to the other one, and a missing text is empty.
assert.equal(marketText({ 'zh-CN': '', 'en-US': 'Only English' }, 'zh-Hans'), 'Only English');
assert.equal(marketText({ 'zh-CN': '只有中文', 'en-US': '' }, 'en-US'), '只有中文');
assert.equal(marketText(undefined, 'en-US'), '');

// Points are milli-points without padding; yuan are micros with at least two decimals.
assert.deepEqual(marketAmount({ unit_price_micros: 2500, currency: 'PTS' }), {
  amount: '2.5',
  unit: 'points',
});
assert.deepEqual(marketAmount({ unit_price_micros: 3000, currency: 'PTS' }), {
  amount: '3',
  unit: 'points',
});
assert.deepEqual(marketAmount({ unit_price_micros: 1, currency: 'PTS' }), {
  amount: '0.001',
  unit: 'points',
});
assert.deepEqual(marketAmount({ unit_price_micros: 50_000, currency: 'CNY' }), {
  amount: '0.05',
  unit: 'yuan',
});
assert.deepEqual(marketAmount({ unit_price_micros: 2_000_000, currency: 'CNY' }), {
  amount: '2.00',
  unit: 'yuan',
});
assert.deepEqual(marketAmount({ unit_price_micros: 1_234_567, currency: 'CNY' }), {
  amount: '1.234567',
  unit: 'yuan',
});

const price = (unit: number, per: number) => ({
  offering_slug: 'echo-standard',
  meter: 'request',
  unit_price_micros: unit,
  per_quantity: per,
  currency: 'PTS' as const,
});
const listing = (prices?: ReturnType<typeof price>[]) =>
  ({ prices }) as unknown as Parameters<typeof lowestPrice>[0];
assert.equal(lowestPrice(listing()), undefined);
assert.equal(lowestPrice(listing([])), undefined);
// The cheapest is judged per unit: 3000 per 1000 is cheaper than 5 per 1.
assert.equal(lowestPrice(listing([price(5, 1), price(3000, 1000)]))?.unit_price_micros, 3000);
assert.equal(lowestPrice(listing([price(0, 1), price(10, 1)]))?.unit_price_micros, 0);

console.log('zgi-market-format: ok');
