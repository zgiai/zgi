import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import ts from 'typescript';
import Exa from 'modelicons/Exa';

const moduleURL = source => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`;
const transpile = source =>
  ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText;
const metadataURL = moduleURL(
  transpile(
    readFileSync(
      path.join(process.cwd(), 'src/components/connectors/connector-metadata.ts'),
      'utf8'
    )
  )
);
const setupURL = moduleURL(transpile(readFileSync(path.join(process.cwd(), 'src/components/connectors/setup/app-setup-state.ts'), 'utf8')));
const catalogModelSource = transpile(
  readFileSync(
    path.join(process.cwd(), 'src/components/connectors/catalog/catalog-model.ts'),
    'utf8'
  )
).replace("'@/components/connectors/connector-metadata'", JSON.stringify(metadataURL)).replace("'../setup/app-setup-state'", JSON.stringify(setupURL));
const { filterConnectorCatalog, getConnectorCatalogRowState, partitionConnectorCatalog } =
  await import(moduleURL(catalogModelSource));
const metadata = await import(metadataURL);

const catalog = [
  {
    id: 'github',
    plugin_id: 'github-plugin',
    name: 'GitHub',
    description: 'Work with repositories and issues',
    classification: 'connector',
    key_fields: [{ name: 'token', label: 'Personal access token', required: true, secret: true }],
    capabilities: [
      { id: 'issues.read', name: 'Issues', description: 'Search repository issues', usable: true },
      { id: 'repos.read', name: 'Repositories', description: 'Inspect source code', usable: true },
    ],
    examples: ['Summarize open issues'],
    connection_summary: {
      count: 1,
      primary_connection_id: '11111111-1111-4111-8111-111111111111',
      state: 'ready',
      external_display_name: '@octocat',
    },
  },
  {
    id: 'web-search',
    plugin_id: 'exa-plugin',
    name: 'Exa Web Search',
    name_i18n: { 'zh-Hans': 'Exa 网页搜索', 'en-US': 'Exa Web Search' },
    description: 'Search the live web',
    classification: 'connector',
    key_fields: [{ name: 'api_key', label: 'API Key', required: true, secret: true }],
    capabilities: [
      { id: 'web.search', name: 'Web search', description: 'Find current sources', usable: true },
      { id: 'web.fetch', name: 'Fetch page', description: 'Read a result page', usable: true },
    ],
    examples: ['Find recent research'],
    connection_summary: { count: 0 },
  },
  {
    id: 'github-attention',
    plugin_id: 'github-plugin',
    name: 'GitHub Archive',
    description: 'A connection that needs a new Key',
    classification: 'connector',
    key_fields: [{ name: 'token', label: 'Personal access token', required: true, secret: true }],
    capabilities: [
      { id: 'archive.read', name: 'Archive', description: 'Read archived issues', usable: true },
    ],
    connection_summary: {
      count: 1,
      primary_connection_id: '22222222-2222-4222-8222-222222222222',
      state: 'reconnect_required',
    },
  },
  {
    id: 'offline-source',
    plugin_id: 'offline-plugin',
    name: 'Offline Source',
    description: 'Temporarily unavailable',
    classification: 'data_source',
    key_fields: [{ name: 'api_key', label: 'API Key', required: true, secret: true }],
    capabilities: [
      {
        id: 'offline.search',
        name: 'Offline search',
        description: 'Search is unavailable',
        usable: false,
        unavailable_reason: 'plugin_unavailable',
      },
    ],
    connection_summary: { count: 0 },
  },
];

const partitioned = partitionConnectorCatalog(catalog);
assert.deepEqual(
  partitioned.connectors.map(item => item.id),
  ['github', 'web-search', 'github-attention'],
  'the external Exa service belongs to Connectors, not data sources'
);
for (const query of ['Exa', '网页搜索', 'websearch']) {
  assert.deepEqual(
    filterConnectorCatalog(catalog, { query, status: 'all' }).map(item => item.id),
    ['web-search'],
    `${query}: brand and localized aliases must find the same stable Connector ID`
  );
}
assert.deepEqual(
  partitioned.dataSources.map(item => item.id),
  ['offline-source'],
  'a non-Exa data source must remain in the data-source group'
);
assert.equal(
  partitioned.connectors.length + partitioned.dataSources.length,
  catalog.length,
  'every catalog item must appear in exactly one API-defined classification'
);

assert.deepEqual(
  filterConnectorCatalog(catalog, { query: 'repository issues', status: 'all' }).map(
    item => item.id
  ),
  ['github']
);
assert.deepEqual(
  filterConnectorCatalog(catalog, { query: 'recent research', status: 'all' }).map(item => item.id),
  ['web-search']
);
assert.deepEqual(
  filterConnectorCatalog(catalog, { query: '', status: 'connected' }).map(item => item.id),
  ['github']
);
assert.deepEqual(
  filterConnectorCatalog(catalog, { query: '', status: 'needs_attention' }).map(item => item.id),
  ['github-attention', 'offline-source']
);

assert.equal(getConnectorCatalogRowState(catalog[0]), 'connected');
assert.equal(getConnectorCatalogRowState(catalog[1]), 'not_connected');
assert.equal(getConnectorCatalogRowState(catalog[2]), 'reconnect_required');
assert.equal(getConnectorCatalogRowState(catalog[3]), 'unavailable');

const mixedConnections = {
  ...catalog[0],
  connection_summary: { ...catalog[0].connection_summary, count: 3, needs_attention_count: 2 },
};
assert.equal(getConnectorCatalogRowState(mixedConnections), 'connected');
assert.deepEqual(
  filterConnectorCatalog([mixedConnections], { query: '', status: 'needs_attention' }),
  [mixedConnections],
  'a ready primary must not hide other saved connections needing attention'
);
assert.deepEqual(
  filterConnectorCatalog([mixedConnections], { query: '', status: 'connected' }),
  [mixedConnections],
  'a mixed provider remains usable through its ready connection'
);
const rowSource = readFileSync(
  path.join(process.cwd(), 'src/components/connectors/catalog/connector-catalog-row.tsx'),
  'utf8'
);
assert.match(rowSource, /catalog\.needsAttentionCount/);

// Exercise the actual display components without mounting routes or making API requests.
const require = createRequire(import.meta.url);
const read = relativePath => readFileSync(path.join(process.cwd(), relativePath), 'utf8');
const loadComponent = (relativePath, dependencies) => {
  const code = ts.transpileModule(read(relativePath), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
      jsx: ts.JsxEmit.ReactJSX,
    },
  }).outputText;
  const module = { exports: {} };
  const localRequire = id => {
    if (Object.hasOwn(dependencies, id)) return dependencies[id];
    if (['react', 'react/jsx-runtime', 'lucide-react', 'modelicons/Exa'].includes(id)) return require(id);
    throw new Error(`Unexpected display-test dependency: ${id}`);
  };
  new Function('require', 'module', 'exports', code)(localRequire, module, module.exports);
  return module.exports;
};
const Container = ({ children }) => createElement('div', null, children);
const presentation = (...names) => Object.fromEntries(names.map(name => [name, Container]));
const utils = { cn: (...values) => values.filter(value => typeof value === 'string').join(' ') };
const icon = loadComponent('src/components/connectors/connector-icon.tsx', {
  'modelicons/Exa': { default: Exa },
  '@/components/ui/icons': loadComponent('src/components/ui/icons.tsx', {}),
  './feishu-icon': loadComponent('src/components/connectors/feishu-icon.tsx', {}),
  './messaging-icons': loadComponent('src/components/connectors/messaging-icons.tsx', {}),
  './connector-icon-model': loadComponent('src/components/connectors/connector-icon-model.ts', {}),
  '@/lib/utils': utils,
});
let activeLocale;
let activeMessages;
let activeItem;
const sharedDisplayDependencies = {
  '@/components/connectors/connector-icon': icon,
  '@/components/ui/badge': presentation('Badge'),
  '@/i18n': {
    useT:
      () =>
      (key, values = {}) =>
        key
          .split('.')
          .reduce((value, segment) => value?.[segment], activeMessages)
          .replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? name)),
  },
};
const { ConnectorDetailPage } = loadComponent(
  'src/components/connectors/detail/connector-detail-page.tsx',
  {
    ...sharedDisplayDependencies,
    'next/link': { default: Container },
    'next-intl': { useLocale: () => activeLocale },
    '@/components/connectors/connector-page-state': presentation(
      'ConnectorPageLoading',
      'ConnectorPageMessage'
    ),
    '@/components/connectors/connector-metadata': metadata,
    '@/components/ui/alert': presentation('Alert', 'AlertDescription', 'AlertTitle'),
    '@/components/ui/breadcrumb': presentation(
      'Breadcrumb',
      'BreadcrumbItem',
      'BreadcrumbLink',
      'BreadcrumbList',
      'BreadcrumbPage',
      'BreadcrumbSeparator'
    ),
    '@/hooks/connectors/use-connectors': {
      useConnector: () => ({ data: { data: activeItem }, isLoading: false, isError: false }),
      usePersonalConnections: () => ({ data: { data: { items: [] } } }),
      personalConnectionItems: data => data.items,
    },
    './connector-capability-list': presentation('ConnectorCapabilityList'),
    './connector-connections-panel': presentation('ConnectorConnectionsPanel'),
    './connector-example-list': presentation('ConnectorExampleList'),
    './connector-key-requirements': presentation('ConnectorKeyRequirements'),
    './connector-app-permissions': presentation('ConnectorAppPermissions'),
    '@/components/connectors/setup/app-setup-state': {
      supportsAppSetup: item => item.setup_method === 'app_credentials',
    },
  }
);
const { ConnectorPickerRow } = loadComponent(
  'src/components/connectors/picker/connector-picker-row.tsx',
  {
    ...sharedDisplayDependencies,
    '@/components/ui/button': presentation('Button'),
    '@/lib/utils': utils,
  }
);
for (const locale of ['en-US', 'zh-Hans']) {
  activeLocale = locale;
  activeMessages = (
    await import(moduleURL(transpile(read(`src/i18n/modules/connectors/${locale}.ts`))))
  ).default;
  const localizedCatalog = catalog.map(item => metadata.localizeConnectorCatalogItem(item, locale));
  assert.deepEqual(
    partitionConnectorCatalog(localizedCatalog).dataSources.map(item => item.id),
    ['offline-source'],
    'changing language cannot move Exa back into data sources'
  );
  for (const item of [catalog[1], catalog[3]]) {
    activeItem = item;
    const localized = metadata.localizeConnectorCatalogItem(item, locale);
    const detail = renderToStaticMarkup(
      createElement(ConnectorDetailPage, { connectorId: item.id, enabled: true })
    );
    const picker = renderToStaticMarkup(
      createElement(ConnectorPickerRow, {
        row: { connector: localized, state: 'not_connected' },
        onAction: () => {},
      })
    );
    const dataSource = item.classification === 'data_source';
    const badge = activeMessages.classification[dataSource ? 'dataSource' : 'connector'];
    assert.ok(detail.includes(`>${badge}</div>`), `${locale}: detail badge follows ${item.id} DTO`);
    for (const html of [detail, picker]) {
      assert.ok(html.includes(localized.name), `${locale}: provider name is localized`);
      assert.match(html, dataSource ? /lucide-database/ : /<svg/);
      if (!dataSource) assert.doesNotMatch(html, /lucide-plug/, 'Exa uses its service mark, not a generic plug');
      assert.doesNotMatch(html, dataSource ? /lucide-plug/ : /lucide-database/);
    }
  }
}
console.log('Connector catalog classification and filter checks passed.');
