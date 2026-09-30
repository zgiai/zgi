import assert from 'node:assert/strict';
import { read, load, harness, nodes, jsx, ui } from './connector-fixture.mjs';
for (const [file, forbidden] of [
  ['setup/connector-create-dialog.tsx', /mail\.notice|setup\.addDescription|app\.notice/],
  ['setup/connector-key-setup-panel.tsx', /setup\.(nameAfterReady|securityNotice|ownerOnly)/],
  [
    'setup/connector-mail-setup-panel.tsx',
    /type="radio"|mail\.(localUsage|verifiedHint|secret|notice)/,
  ],
  [
    'setup/connector-app-setup-panel.tsx',
    /app\.(secretNotice|notice|readyHint)|pending\.(secretHint|closeHint)/,
  ],
  ['setup/connector-messaging-setup-panel.tsx', /messaging\.(phase|notice|secret)/],
  [
    'connections/connection-management-page.tsx',
    /ConnectorMaintenanceCard|<ConnectionMaintenanceCard|ConnectorReadinessBanner|ConnectorPlatformNotice|removal\.disconnectDescription/,
  ],
  ['catalog/connector-center.tsx', /catalog\.pluginNote/],
  ['picker/connector-picker-dialog.tsx', /defaults\.scopeHint|picker\.group\.footer/],
])
  assert.doesNotMatch(read(`src/components/connectors/${file}`), forbidden, file);
const purpose = load('src/components/connectors/connector-presentation.ts');
assert.equal(purpose.connectorPurpose('unknown-plugin', 'zh-Hans'), undefined);
for (const provider of [
  'web-search',
  'github',
  'feishu',
  'dingtalk',
  'wecom',
  'weixin',
  'gmail',
  'qqmail',
  'netease163',
]) {
  for (const locale of ['en-US', 'zh-Hans']) {
    const text = purpose.connectorPurpose(provider, locale);
    assert.ok(text && text.length < 110);
    assert.doesNotMatch(text, /digest|token|checkpoint|real tenant|真实租户/);
  }
}
const metadata = load('src/components/connectors/connector-metadata.ts');
const h = harness(react => {
  const component = load('src/components/connectors/detail/connector-capability-list.tsx', {
    react,
    'react/jsx-runtime': jsx,
    'next-intl': { useLocale: () => 'en-US' },
    'lucide-react': ui,
    '@/components/ui/button': ui,
    '@/components/ui/badge': ui,
    '@/components/ui/card': ui,
    '@/components/ui/label': ui,
    '@/components/ui/select': ui,
    '@/i18n': { useT: () => key => key },
    '../connector-metadata': metadata,
    './capability-groups': load('src/components/connectors/detail/capability-groups.ts'),
  });
  return capabilities => component.ConnectorCapabilityList({ capabilities });
});
const rendered = JSON.stringify(
  h.render([
    {
      id: 'wecom.documents.search',
      name: 'Search documents',
      usable: true,
      availability: 'conditional',
      description: 'DO_NOT_SHOW_INTERNAL_TOOL_INSTRUCTIONS',
    },
  ])
);
assert.match(rendered, /detail.checkAtUse/);
assert.doesNotMatch(rendered, /messaging.conditional|DO_NOT_SHOW_INTERNAL/);
assert.equal(nodes(h.render([])).filter(node => node.type === 'li').length, 0);
console.log('Connector setup/management copy boundaries and conditional read labeling passed.');

const status = load('src/components/connectors/setup/connector-setup-status.tsx', {
  'react/jsx-runtime': jsx,
  'next/link': { default: 'Link' },
  'lucide-react': ui,
  '@/components/ui/alert': ui,
  '@/components/ui/button': ui,
  '@/i18n': { useT: () => key => key },
});
const readyNodes = nodes(
  status.ConnectorSetupStatus({
    state: {
      status: 'ready',
      connection: { id: 'mail-account', external_display_name: 'user@example.com' },
    },
  })
);
assert.equal(
  readyNodes.filter(node => node.type === 'Button').length,
  1,
  'one next action after success'
);
assert.equal(
  readyNodes.find(node => node.type === 'Link').props.href,
  '/console/connectors/connections/mail-account'
);
