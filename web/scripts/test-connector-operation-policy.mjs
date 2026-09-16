import assert from 'node:assert/strict';
import { load, harness, flush, nodes, jsx, ui, read } from './connector-fixture.mjs';

const calls = [];
let reject, invalidations = 0;
const connection = {
  id: 'connection', connector_id: 'wecom', revision: 3, state: 'ready',
  allowed_operation_ids: ['wecom.identity.get'],
  usable_operation_ids: ['wecom.identity.get', 'wecom.messages.send'],
};
const connector = { capabilities: [
  { id: 'wecom.identity.get', name: 'Identity' },
  { id: 'wecom.messages.send', name: 'Send' },
  { id: 'wecom.documents.search', name: 'Documents' },
] };
const h = harness(react => {
  const { ConnectionOperationPolicy } = load('src/components/connectors/connections/connection-operation-policy.tsx', {
    react, 'react/jsx-runtime': jsx, 'next-intl': { useLocale: () => 'en-US' },
    '@tanstack/react-query': { useQueryClient: () => ({}) },
    '@/i18n': { useT: () => key => key }, '@/components/ui/button': ui,
    '@/components/ui/checkbox': ui, '@/components/ui/label': ui,
    '@/components/ui/collapsible': ui, 'lucide-react': ui,
    '@/services/connector.service': { connectorService: { updateOperationPolicy: (...args) => {
      calls.push(args); return new Promise((_, no) => { reject = no; });
    } } },
    '@/hooks/connectors/connector-request-error': load('src/hooks/connectors/connector-request-error.ts'),
    '@/hooks/connectors/connection-detail-query': { invalidatePersonalConnectionQueries: () => invalidations++ },
    '../connector-metadata': { resolveConnectorLocalizedText: (_, __, fallback) => fallback },
  });
  return (current = connection) => {
    const element = ConnectionOperationPolicy({ connection: current, connector });
    return element && element.type(element.props);
  };
});
const render = () => h.render();
const checkboxes = tree => nodes(tree).filter(n => n.type === 'Checkbox');
const save = tree => nodes(tree).find(n => n.type === 'Button' && n.props.children === 'operationPolicy.save');
let tree = render();
assert.equal(checkboxes(tree)[2].props.disabled, true, 'new operations require separate connection acceptance');
assert.equal(save(tree).props.disabled, true);
checkboxes(tree)[0].props.onCheckedChange(false);
tree = render();
assert.ok(nodes(tree).some(n => n.props.children === 'operationPolicy.paused'));
const handler = save(tree).props.onClick;
handler(); handler();
assert.equal(calls.length, 1, 'same-frame duplicate save must be suppressed');
assert.deepEqual(calls[0].slice(0, 2), ['connection', { expected_revision: 3, operation_ids: [] }]);
reject(new Error('network outcome unknown'));
await flush();
tree = render();
save(tree).props.onClick();
assert.equal(calls[1][2], calls[0][2], 'identical retry reuses the receipt key');
reject({ response: { status: 409, data: { code: 'connector_conflict' } } });
await flush();
tree = render();
assert.equal(save(tree).props.disabled, true, 'stale revision must not auto-resubmit');
assert.ok(nodes(tree).some(n => n.props.children === 'operationPolicy.conflict'));
nodes(tree).find(n => n.type === 'Button' && n.props.children === 'actions.retry').props.onClick();
assert.equal(invalidations, 1);
assert.equal(h.render({ ...connection, connector_id: 'gmail' }), null);
assert.equal(h.render({ ...connection, allowed_operation_ids: undefined }), null);
h.unmount();

const groups = load('src/components/connectors/detail/capability-groups.ts');
const definition = JSON.parse(read('../api/internal/modules/connectors/catalog/definitions/feishu.json'));
const names = new Set(['messaging', 'documents', 'people', 'resources']);
assert.equal(definition.operations.length, 38);
for (const op of definition.operations) assert.ok(names.has(groups.capabilityGroup(op.id)), op.id);
console.log('Connector operation-policy concurrency, empty selection, retry and Feishu grouping checks passed.');
