import { load } from './connector-fixture.mjs';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import en from '../src/i18n/modules/connectors/en-US.ts';
import zh from '../src/i18n/modules/connectors/zh-Hans.ts';
const {
  formatConnectorDateTime,
  localizeConnectorCatalogItem,
  localizeConnectorRuntimeConfirmation,
  resolveConnectorLocalizedText,
} = load('src/components/connectors/connector-metadata.ts');

const root = process.cwd();

function keyShape(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return typeof value;
  return Object.fromEntries(
    Object.keys(value)
      .sort()
      .map(key => [key, keyShape(value[key])])
  );
}

assert.deepEqual(keyShape(zh), keyShape(en), 'English and Simplified Chinese keys must match');

for (const key of [
  'title',
  'description',
  'account.title',
  'usage.title',
  'maintenance.title',
  'reconnect.title',
  'removal.title',
  'removal.forceAction',
  'feedback.removed',
  'setup.unknownTitle',
  'setup.checkConnections',
  'setup.statusReadFailedTitle',
  'setup.statusReadFailedDescription',
  'setup.retryStatus',
  'setup.errors.resultUnknown',
  'connections.title',
  'connections.addConnection',
  'setup.nameConnection',
  'metadata.descriptionUnavailable',
]) {
  const value = key.split('.').reduce((current, segment) => current?.[segment], en);
  assert.equal(typeof value, 'string', `connectors.${key} must be translated`);
  assert.ok(value.trim(), `connectors.${key} must not be empty`);
}

const exa = {
  id: 'web-search',
  plugin_id: 'zgi.exa-search',
  name: 'Exa Web Search',
  name_i18n: { 'en-US': 'Exa Web Search', zh_CN: 'Exa 网页搜索' },
  description: 'Search and read public webpages with Exa.',
  description_i18n: {
    'en-US': 'Search and read public webpages with Exa.',
    'zh-Hans': '通过 Exa 搜索并读取公开网页。',
  },
  classification: 'connector',
  key_fields: [
    {
      name: 'api_key',
      label: 'Exa API Key',
      label_i18n: { 'zh-CN': 'Exa API 密钥' },
      placeholder: 'Paste your Exa API Key',
      placeholder_i18n: { zh: '粘贴 Exa API 密钥' },
      required: true,
      secret: true,
    },
  ],
  capabilities: [
    {
      id: 'web.search',
      name: 'Search Web',
      name_i18n: { 'zh-Hans': '搜索网页' },
      description: 'Find relevant public webpages.',
      description_i18n: { zh_CN: '查找相关公开网页。' },
      usable: true,
    },
  ],
  examples: ['Find recent research'],
  connection_summary: { count: 0 },
};

const zhExa = localizeConnectorCatalogItem(exa, 'zh-CN', {
  description: zh.metadata.descriptionUnavailable,
  capabilityDescription: zh.metadata.capabilityDescriptionUnavailable,
});
assert.equal(zhExa.name, 'Exa 网页搜索');
assert.equal(zhExa.classification, 'connector', 'localization must retain the Exa Connector type');
assert.equal(zhExa.description, '搜索网页，读取网页内容');
assert.equal(zhExa.key_fields[0].label, 'Exa API 密钥');
assert.equal(zhExa.key_fields[0].placeholder, '粘贴 Exa API 密钥');
assert.equal(zhExa.capabilities[0].name, '搜索网页');
assert.equal(zhExa.capabilities[0].description, '查找相关公开网页。');
assert.deepEqual(zhExa.examples, ['Find recent research']);
assert.equal(localizeConnectorCatalogItem(exa, 'en-US').name, 'Exa Web Search');
assert.equal(localizeConnectorCatalogItem(exa, 'en-US').classification, 'connector');
const referenceLibrary = {
  id: 'reference-library',
  plugin_id: 'reference-library-plugin',
  name: 'Reference Library',
  name_i18n: { 'zh-Hans': '参考资料库' },
  description: 'Read archived documents',
  classification: 'data_source',
  key_fields: [],
  capabilities: [],
  connection_summary: { count: 0 },
};
assert.equal(
  localizeConnectorCatalogItem(referenceLibrary, 'zh-Hans').classification,
  'data_source'
);
assert.equal(
  localizeConnectorCatalogItem({ ...exa, classification: 'data_source' }, 'zh-Hans').classification,
  'data_source',
  'a stale server classification must not be silently overridden by a frontend Exa special case'
);
assert.equal(resolveConnectorLocalizedText({ en_US: 'English' }, 'en-US', 'Fallback'), 'English');
assert.equal(resolveConnectorLocalizedText({ 'zh-Hans': '中文' }, 'zh_Hans', 'Fallback'), '中文');
assert.equal(resolveConnectorLocalizedText({ 'zh-Hans': ' ' }, 'zh-CN', 'Fallback'), 'Fallback');
assert.equal(resolveConnectorLocalizedText(undefined, 'fr-FR', 'Plugin name'), 'Plugin name');
assert.equal(exa.name, 'Exa Web Search', 'display localization must not mutate cached DTOs');
assert.equal(zhExa.id, exa.id);
assert.equal(zhExa.key_fields[0].name, 'api_key');
assert.equal(zhExa.capabilities[0].id, 'web.search');

const localizedConfirmation = localizeConnectorRuntimeConfirmation(
  {
    route_receipt: 'receipt',
    connector_id: 'web-search',
    connector_name: 'Exa Web Search',
    connector_name_i18n: { zh_CN: 'Exa 网页搜索' },
    operation_id: 'web.search',
    action_name: 'Search Web',
    action_name_i18n: { 'zh-Hans': '搜索网页' },
    target: { type: 'chat', id: 'conversation' },
    consequence: {
      external_effect: 'read',
      description: 'Searches public webpages.',
      description_i18n: { zh: '搜索公开网页。' },
      data_egress: true,
    },
    risk_classification: 'unclassified',
  },
  'zh-CN',
  exa
);
assert.equal(localizedConfirmation.connector_name, 'Exa 网页搜索');
assert.equal(localizedConfirmation.action_name, '搜索网页');
assert.equal(localizedConfirmation.consequence.description, '搜索公开网页。');
const unknownConfirmation = {
  ...localizedConfirmation,
  connector_id: 'other-plugin',
  connector_name: 'Other plugin',
  connector_name_i18n: undefined,
  action_name: 'Custom web operation',
  action_name_i18n: undefined,
};
assert.equal(
  localizeConnectorRuntimeConfirmation(unknownConfirmation, 'zh-Hans', exa).connector_name,
  'Other plugin',
  'a catalog item from another Connector must never rename a confirmation'
);
assert.equal(
  localizeConnectorRuntimeConfirmation(unknownConfirmation, 'zh-Hans').action_name,
  'Custom web operation',
  'built-in operation fallbacks must include the Connector identity'
);

assert.match(formatConnectorDateTime('2026-09-02T08:00:00Z', 'zh-CN', '-'), /2026/);
assert.equal(formatConnectorDateTime('not-a-date', 'zh-CN', '尚未验证'), '尚未验证');
assert.equal(
  formatConnectorDateTime('2026-09-02T08:00:00Z', 'zh_Hans', '-'),
  formatConnectorDateTime('2026-09-02T08:00:00Z', 'zh-Hans', '-')
);

for (const messages of [en, zh]) {
  assert.doesNotMatch(
    `${messages.setup.errors.resultUnknown} ${messages.setup.errors.requestFailed}`,
    /not saved|was not saved|invalid|未被保存|未保存|无效/i,
    'unconfirmed transport results must not claim that the Key is invalid or was not saved'
  );
}

const loaderSource = readFileSync(path.join(root, 'src', 'i18n', 'loader.ts'), 'utf8');
const modulesSource = readFileSync(path.join(root, 'src', 'i18n', 'modules', 'index.ts'), 'utf8');
const routeModulesSource = readFileSync(path.join(root, 'src', 'i18n', 'route-modules.ts'), 'utf8');
assert.match(loaderSource, /'connectors'/);
assert.match(loaderSource, /modules\/connectors\/zh-Hans/);
assert.match(loaderSource, /modules\/connectors\/en-US/);
assert.match(modulesSource, /connectors from '\.\/connectors\/en-US'/);
assert.match(modulesSource, /connectors,/);
assert.match(routeModulesSource, /'connectors'/);

// Legacy Key setup remains simple. Versioned application and messaging
// onboarding may expose QR steps without adding a Key policy editor.
const {
  app: enApp,
  messaging: enMessaging,
  pending: enPending,
  platform: enPlatform,
  ...enKeyCopy
} = en;
const {
  app: zhApp,
  messaging: zhMessaging,
  pending: zhPending,
  platform: zhPlatform,
  ...zhKeyCopy
} = zh;
assert.match(enPlatform.dingtalk, /choose the original robot/);
assert.match(zhPlatform.dingtalk, /请选择原机器人/);
assert.match(enPlatform.wecom, /documents?|table/);
assert.match(zhPlatform.feishu, /可能更新应用配置/);
assert.match(enPlatform.resourceAtExecution, /checked when the operation runs/);
assert.match(zhPlatform.resourceAtExecution, /执行操作时/);
assert.match(en.mail.localUsage, /inside ZGI/);
assert.match(zh.mail.localUsage, /限制 ZGI/);
assert.match(en.mail.verifiedHint, /does not verify delivery/);
assert.match(zh.mail.verifiedHint, /不代表投递已验证/);
assert.match(enPending.qrUnavailable, /unavailable/);
assert.match(zhPending.qrUnavailable, /暂时无法/);
assert.doesNotMatch(enPending.resumeQRHint, /real tenant|has not been verified/);
assert.doesNotMatch(zhPending.resumeQRHint, /真实租户验证/);
assert.match(enPending.lastChecked, /\{date\}/);
assert.match(zhPending.retention, /\{date\}/);
const visibleCopy = `${JSON.stringify(enKeyCopy)}\n${JSON.stringify(zhKeyCopy)}`;
assert.match(enApp.qr, /QR/);
assert.match(zhApp.qr, /扫码/);
assert.match(enMessaging.qr, /QR/);
assert.match(zhMessaging.qr, /扫码/);
assert.match(zhMessaging.unavailable, /暂不可用/);
assert.doesNotMatch(JSON.stringify(zhMessaging), /尚未开放|准入验证/);
assert.doesNotMatch(JSON.stringify(enMessaging), /deployment admission|not been enabled/);
assert.match(enMessaging.conditional, /sending/);
assert.match(zhApp.configurationOnly, /不|不是/);
for (const forbidden of [
  /OAuth/i,
  /QR code/i,
  /credential source/i,
  /configure (?:grants|scopes|policies|audit|approvals|risk)/i,
  /扫码/,
  /凭据来源/,
  /配置(?:授权|范围|策略|审计|审批|风险)/,
]) {
  assert.doesNotMatch(visibleCopy, forbidden, 'Connector copy must preserve the simplified setup');
}

console.log('Connector English/Chinese i18n shape and registration checks passed.');
