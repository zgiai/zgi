import assert from 'node:assert/strict';
import { harness, jsx, load, nodes, ui } from './connector-fixture.mjs';
let connection, connector;
const page = harness(react => {
  const { ConnectionManagementPage } = load(
    'src/components/connectors/connections/connection-management-page.tsx',
    {
      react,
      'react/jsx-runtime': jsx,
      'next/link': { default: 'Link' },
      'next-intl': { useLocale: () => 'en-US' },
      'next/navigation': { useRouter: () => ({ replace() {} }) },
      'lucide-react': ui,
      sonner: { toast: {} },
      '@/components/ui/collapsible': ui,
      '@/components/ui/button': ui,
      '@/components/ui/breadcrumb': ui,
      '@/components/connectors/connector-icon': ui,
      '@/components/connectors/connector-metadata': { localizeConnectorCatalogItem: item => item },
      '@/components/connectors/connector-page-state': ui,
      '@/components/connectors/connector-status-badge': ui,
      '@/hooks/connectors/use-connectors': {
        usePersonalConnection: () => ({ data: { data: connection } }),
        useConnector: () => ({ data: { data: connector } }),
        useDiagnosePersonalConnection: () => ({}),
      },
      '@/i18n': { useT: () => key => key },
      './connection-account-card': ui,
      './connection-management-model': { getConnectionManagementPageState: () => 'ready' },
      './connection-reconnect-dialog': ui,
      './connection-removal-dialog': ui,
      './connection-usage-list': ui,
      './personal-connections-model': { personalConnectionDisplayName: item => item.name },
      '@/components/connectors/setup/app-setup-state': {
        supportsAppSetup: item => item.id === 'feishu',
      },
      '@/components/connectors/setup/messaging-setup-state': {
        supportsMessagingContract: () => false,
      },
      '@/components/connectors/detail/connector-app-permissions': ui,
      '@/components/connectors/detail/connector-capability-list': ui,
      './connection-app-dialog': ui,
      '../setup/mail-setup-state': {
        supportsMailSetup: item => item.id === 'gmail',
        connectionMailUsage: () => 'read_send',
        mailFailureMessage: () => '',
      },
      './connection-operation-policy': ui,
      '../connector-taxonomy-badges': ui,
    }
  );
  return () => ConnectionManagementPage({ connectionId: 'connection', enabled: true });
});
// The catalog can aggregate another account's abilities: only display this
// connection's evidence, retaining unavailable operations and readiness state.
for (const id of [
  'x-social',
  'linkedin-social',
  'facebook-social',
  'github',
  'github-mcp',
  'feishu',
  'gmail',
]) {
  for (const state of ['ready', 'reconnect_required']) {
    connection = {
      id: 'connection',
      connector_id: id,
      name: 'Example',
      state,
      capabilities: [
        { id: `${id}.read`, name: 'Read', usable: state === 'ready' },
        { id: `${id}.write`, name: 'Write', usable: false },
      ],
      bindings: [],
    };
    connector = {
      id,
      name: id,
      key_fields: [{ name: 'token' }],
      capabilities: [{ id: 'other-account-tool', usable: true }],
    };
    const lists = nodes(page.render()).filter(node => node.type === 'ConnectorCapabilityList');
    assert.equal(lists.length, 1, `${id}/${state} must show one tool list`);
    assert.strictEqual(
      lists[0].props.capabilities,
      connection.capabilities,
      'use the current connection evidence'
    );
  }
}
connector = undefined;
assert.equal(
  nodes(page.render()).filter(node => node.type === 'ConnectorCapabilityList').length,
  1,
  'saved tools remain visible while catalog metadata loads'
);
page.unmount();
console.log('Connected account tools render across auth methods and readiness states.');
