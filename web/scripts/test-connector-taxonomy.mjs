import assert from 'node:assert/strict';
import { load } from './connector-fixture.mjs';

const metadata = load('src/components/connectors/connector-metadata.ts');
const { connectorCategory, connectorTags, connectorCatalogSearchValues } = metadata;
assert.equal(connectorCategory({ category: 'developer' }), 'developer');
assert.equal(connectorCategory({ category: 'unsupported' }), 'other');
assert.deepEqual(
  connectorTags({ tags: ['email', 'email', 'unsupported', 'collaboration'] }),
  ['email', 'collaboration']
);
assert.ok(
  connectorCatalogSearchValues({
    id: 'github',
    name: 'GitHub',
    description: 'Repositories',
    category: 'developer',
    tags: ['code', 'repository'],
    key_fields: [],
    capabilities: [],
    connection_summary: { count: 0 },
  }).includes('developer')
);

const catalogModel = load('src/components/connectors/catalog/catalog-model.ts', {
  '@/components/connectors/connector-metadata': metadata,
  '../setup/app-setup-state': { supportsConnectorSetup: () => true },
});
const items = [
  {
    id: 'github',
    name: 'GitHub',
    description: 'Repositories and issues',
    classification: 'connector',
    category: 'developer',
    tags: ['code', 'repository', 'issues'],
    key_fields: [],
    capabilities: [{ id: 'issues', name: 'Issues', description: '', usable: true }],
    connection_summary: { count: 0 },
  },
  {
    id: 'web-search',
    name: 'Exa Web Search',
    description: 'Search the web',
    classification: 'connector',
    category: 'information',
    tags: ['search', 'web'],
    key_fields: [],
    capabilities: [{ id: 'search', name: 'Search', description: '', usable: true }],
    connection_summary: { count: 0 },
  },
];
assert.equal(
  catalogModel.filterConnectorCatalog(items, {
    query: '代码',
    status: 'all',
    category: 'developer',
    tag: 'code',
  }).length,
  1
);
assert.equal(
  catalogModel.filterConnectorCatalog(items, {
    query: 'search',
    status: 'all',
    category: 'information',
    tag: 'web',
  }).length,
  1
);
assert.equal(
  catalogModel.filterConnectorCatalog(items, {
    query: '',
    status: 'all',
    category: 'information',
    tag: 'code',
  }).length,
  0
);
console.log('Connector taxonomy mapping, search and filters passed');
