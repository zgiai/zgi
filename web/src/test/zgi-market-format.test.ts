import assert from 'node:assert/strict';

// prettier-ignore
// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { chargedByUsage, highestPrice, knownMarketMeter, lowestPrice, marketAmount, marketDate, marketDisplayPrice, marketErrorCode, marketExternalUrl, marketPriceCap, marketSameOrigin, marketText, skillPackageSummary } from '../components/zgi-market/market-format.ts';
// prettier-ignore
// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { marketApiQuickStart, marketGatewayOrigin, marketMcpClientConfig, marketMcpServerUrl } from '../components/zgi-market/market-usage.ts';

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
const listing = (prices?: Array<ReturnType<typeof price>>) =>
  ({ prices }) as unknown as Parameters<typeof lowestPrice>[0];
assert.equal(lowestPrice(listing()), undefined);
assert.equal(lowestPrice(listing([])), undefined);
// The cheapest is judged per unit: 3000 per 1000 is cheaper than 5 per 1.
assert.equal(lowestPrice(listing([price(5, 1), price(3000, 1000)]))?.unit_price_micros, 3000);
assert.equal(lowestPrice(listing([price(0, 1), price(10, 1)]))?.unit_price_micros, 0);
assert.equal(highestPrice(listing()), undefined);
assert.equal(
  highestPrice(listing([price(0, 1), price(2000, 1), price(0, 1)]))?.unit_price_micros,
  2000
);
assert.equal(highestPrice(listing([price(5, 1), price(3000, 1000)]))?.unit_price_micros, 5);

// A token price is read per million tokens; other meters keep the quantity of their row.
assert.deepEqual(
  marketDisplayPrice({
    meter: 'token_input',
    unit_price_micros: 3,
    per_quantity: 1000,
    currency: 'PTS',
  }),
  { meter: 'token_input', unit_price_micros: 3000, per_quantity: 1_000_000, currency: 'PTS' }
);
assert.equal(
  marketDisplayPrice({
    meter: 'request',
    unit_price_micros: 2500,
    per_quantity: 1,
    currency: 'PTS',
  }).per_quantity,
  1
);
assert.equal(marketDate('not a date', 'en-US'), '');
assert.equal(marketDate('2026-10-12T03:03:58Z', 'en-US').includes('2026'), true);
// Only an absolute http(s) address becomes a link.
assert.equal(marketExternalUrl('javascript:alert(1)'), undefined);
assert.equal(marketExternalUrl('github.com/x/y'), undefined);
assert.equal(marketExternalUrl('https://github.com/x/y'), 'https://github.com/x/y');

// Usage samples: the gateway of this instance, or a placeholder that says it is one.
assert.deepEqual(marketGatewayOrigin('https://market.example.com/'), {
  origin: 'https://market.example.com',
  configured: true,
});
assert.equal(marketGatewayOrigin('').configured, false);
const origin = 'https://market.example.com';
assert.equal(marketMcpServerUrl(origin, 'deepwiki'), `${origin}/v1/mcp/deepwiki`);
assert.equal(marketMcpServerUrl(origin, 'Bad Slug"'), `${origin}/v1/mcp/<server>`);
assert.equal(
  JSON.parse(marketMcpClientConfig(origin, 'deepwiki')).mcpServers.deepwiki.type,
  'http'
);
// An http offering shows its first reading operation; a model family shows its protocol's request.
const declared = marketApiQuickStart(origin, {
  slug: 'amap-standard',
  protocol_family: 'http',
  operations: [
    { method: 'POST', path: '/v3/write' },
    { method: 'GET', path: '/v3/geocode/geo' },
  ],
});
assert.equal(declared.concrete, true);
assert.equal(
  declared.curl.startsWith(`curl "${origin}/v1/gateway/amap-standard/v3/geocode/geo"`),
  true
);
const chat = marketApiQuickStart(origin, { slug: 'claude', protocol_family: 'anthropic_messages' });
assert.equal(chat.needsModel, true);
assert.equal(chat.curl.includes('/v1/gateway/claude/v1/messages'), true);
// A path that could leave the quoted address is never put into a command.
const unsafe = marketApiQuickStart(origin, {
  slug: 'x',
  protocol_family: 'http',
  operations: [{ method: 'GET', path: '/a"; rm -rf /' }],
});
assert.equal(unsafe.concrete, false);
assert.equal(unsafe.curl.includes('rm -rf'), false);

// A skill package: the root first, then directories by name; scripts are named so a reader knows
// whether the skill brings code to run.
const summary = skillPackageSummary([
  { path: 'scripts/summarize.py', size: 130 },
  { path: 'references/usage.md', size: 47 },
  { path: 'SKILL.md', size: 213 },
  { path: 'scripts/lib/util.js', size: 9 },
]);
assert.deepEqual(
  summary.groups.map(group => [group.directory, group.files.map(file => file.path)]),
  [
    ['', ['SKILL.md']],
    ['references', ['references/usage.md']],
    ['scripts', ['scripts/lib/util.js', 'scripts/summarize.py']],
  ]
);
assert.deepEqual(
  summary.scripts.map(file => file.path),
  ['scripts/summarize.py', 'scripts/lib/util.js']
);
assert.equal(skillPackageSummary([{ path: 'SKILL.md', size: 1 }]).scripts.length, 0);

assert.equal(
  marketErrorCode({ response: { data: { error_code: 'MARKET_ACCESS.CREDENTIAL_LIMIT_REACHED' } } }),
  'MARKET_ACCESS.CREDENTIAL_LIMIT_REACHED'
);
assert.equal(marketErrorCode(new Error('network')), '');
assert.equal(marketErrorCode(null), '');

assert.equal(
  marketSameOrigin(
    'https://pre.example.com/dashboard/market/catalog?connected=1',
    'https://pre.example.com'
  ),
  true
);
assert.equal(marketSameOrigin('/dashboard/market/catalog', 'https://pre.example.com'), true);
assert.equal(
  marketSameOrigin(
    'https://market.example.com/console/organizations/0?error_code=X',
    'https://pre.example.com'
  ),
  false
);
assert.equal(marketSameOrigin('http://pre.example.com/x', 'https://pre.example.com'), false);
assert.equal(marketSameOrigin('javascript:alert(1)', 'https://pre.example.com'), false);
assert.equal(marketSameOrigin('https://pre.example.com/x', 'not an origin'), false);

console.log('zgi-market-format: ok');

// The unit meter is one this page names; a meter a newer market adds is none, and is shown by its
// identifier instead of failing the page.
assert.equal(knownMarketMeter('unit'), 'unit');
assert.equal(knownMarketMeter('request'), 'request');
assert.equal(knownMarketMeter('gpu_hour'), undefined);

// The most one call can cost: of the named offering, or of the dearest one when none is named.
const capped = {
  price_caps: [
    { offering_slug: 'web-search', max_charge_micros: 82_000 },
    { offering_slug: 'deep-search', max_charge_micros: 250_000 },
    { offering_slug: 'broken', max_charge_micros: 0 },
  ],
};
assert.equal(marketPriceCap(capped, 'web-search'), 82_000);
assert.equal(marketPriceCap(capped), 250_000);
assert.equal(marketPriceCap(capped, 'chat'), undefined);
assert.equal(marketPriceCap(capped, 'broken'), undefined);
assert.equal(marketPriceCap({}), undefined);

// A listing is charged by usage when anything but the call itself has a price.
const perRequest = {
  meter: 'request',
  unit_price_micros: 2000,
  per_quantity: 1,
  currency: 'PTS' as const,
};
const perCredit = {
  meter: 'unit',
  unit_name: 'credits',
  unit_price_micros: 8000,
  per_quantity: 1,
  currency: 'PTS' as const,
};
assert.equal(chargedByUsage([perRequest]), false);
assert.equal(chargedByUsage([perRequest, perCredit]), true);
assert.equal(chargedByUsage([perRequest, { ...perCredit, unit_price_micros: 0 }]), false);
assert.equal(chargedByUsage([{ ...perRequest, meter: 'tool_call' }]), false);
assert.equal(chargedByUsage([{ ...perRequest, meter: 'gpu_hour' }]), true);

// The reason of an approval asked for cost alone is read back into its two amounts.
// prettier-ignore
// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { marketCostApprovalAmounts } from '../components/zgi-market/market-format.ts';
assert.deepEqual(
  marketCostApprovalAmounts(
    'one call can cost up to 201 points, above the confirmation threshold of 100 points'
  ),
  { max: '201', threshold: '100' }
);
assert.deepEqual(
  marketCostApprovalAmounts(
    ' one call can cost up to 201.5 points, above the confirmation threshold of 0.001 points '
  ),
  { max: '201.5', threshold: '0.001' }
);
assert.equal(marketCostApprovalAmounts('tool manifest requires approval'), undefined);
assert.equal(marketCostApprovalAmounts('one call can cost up to many points'), undefined);
assert.equal(marketCostApprovalAmounts(undefined), undefined);

// The pending approval says why a market tool asks when the reason is its cost, and nothing more
// for any other reason. The listing names the unit its offerings count in.
// prettier-ignore
// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { marketUnitName, withMarketCostReason } from '../components/zgi-market/market-format.ts';
import { readFileSync } from 'node:fs';

const say = (values: { sentence: string; max: string; threshold: string }) =>
  `${values.sentence}单次最高 ${values.max} 点，超过确认阈值 ${values.threshold} 点。`;
assert.equal(
  withMarketCostReason(
    '读取 1 个市场工具。',
    'one call can cost up to 201 points, above the confirmation threshold of 100 points',
    say
  ),
  '读取 1 个市场工具。单次最高 201 点，超过确认阈值 100 点。'
);
assert.equal(
  withMarketCostReason('调用 1 个市场工具。', 'tool manifest requires approval', say),
  '调用 1 个市场工具。'
);
assert.equal(withMarketCostReason('运行此工具。', undefined, say), '运行此工具。');

const request = {
  meter: 'request',
  unit_price_micros: 1000,
  per_quantity: 1,
  currency: 'PTS' as const,
};
const credit = {
  meter: 'unit',
  unit_name: 'credits',
  unit_price_micros: 20000,
  per_quantity: 1,
  currency: 'PTS' as const,
};
assert.equal(marketUnitName({ prices: [request, credit] }), 'credits');
assert.equal(
  marketUnitName({ prices: [credit, { ...credit, unit_name: 'pages' }, credit] }),
  'credits / pages'
);
assert.equal(marketUnitName({ prices: [request, { ...credit, unit_name: undefined }] }), '');
assert.equal(marketUnitName({}), '');

// The card a person approves on is built from the action sentence alone: the cost reason has to be
// put into that sentence, with the reason of the decision, and both languages have the words.
const here = new URL('.', import.meta.url);
const timeline = readFileSync(
  new URL('../components/chat/variants/aichat/agentic-timeline.tsx', here),
  'utf8'
);
assert.match(
  timeline,
  /const actionSentence = withMarketCostReason\(\s*governanceActionSentence\([^)]*\),\s*rawReason,/,
  'the pending approval sentence carries the cost reason'
);
assert.match(
  timeline,
  /actionSentence: view\.actionSentence/,
  'the pending approval shows that sentence'
);
for (const language of ['zh-Hans', 'en-US']) {
  const words = readFileSync(new URL(`../i18n/modules/webapp/${language}.ts`, here), 'utf8');
  assert.match(
    words,
    /withMarketCost:\s*'[^']*\{sentence\}[^']*\{max\}[^']*\{threshold\}/,
    language
  );
}
const panel = readFileSync(
  new URL('../components/zgi-market/zgi-market-listing-panel.tsx', here),
  'utf8'
);
assert.match(
  panel.replace(/\s+/g, ' '),
  /name === 'unit' \? marketUnitName\(listing\) \|\| meter\(name\)/,
  'the meter row names the unit'
);

// A refusal where nobody can approve is read back, so the page can say it in its language.
// prettier-ignore
// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { marketUnattendedRefusal } from '../components/zgi-market/market-format.ts';
assert.deepEqual(
  marketUnattendedRefusal(
    "This tool writes data and needs a person's confirmation, so it does not run unattended"
  ),
  { kind: 'write' }
);
assert.deepEqual(
  marketUnattendedRefusal(
    'One call of this tool can cost up to 201 points, above the confirmation threshold of 100 points, so it does not run unattended'
  ),
  { kind: 'cost', max: '201', threshold: '100' }
);
assert.equal(
  marketUnattendedRefusal(
    'the current Agent does not have persistent authorization for this tool action'
  ),
  undefined
);
