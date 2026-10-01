import assert from 'node:assert/strict';
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import path from 'node:path';
import { getConsoleRouteAccess, isOrganizationScopedConsoleRoute } from '../src/routes/access.ts';
import {
  ZGI_CONSOLE_SITE_MAP,
  getZGIConsoleNavigationAccess,
  resolveZGIConsoleNavigationRoute,
} from '../src/routes/console-navigation.ts';

const root = process.cwd();
const connectorRoot = path.join(root, 'src', 'components', 'connectors');
const routes = {
  center: '/console/connectors',
  connections: '/console/connectors/connections',
  detail: '/console/connectors/github',
  connection: '/console/connectors/connections/11111111-1111-4111-8111-111111111111',
};

for (const [name, href] of Object.entries(routes)) {
  assert.equal(
    isOrganizationScopedConsoleRoute(href),
    true,
    `${name} Connector route must be organization-scoped`
  );
  assert.deepEqual(getConsoleRouteAccess(href), {
    scope: 'organization',
    requiresWorkspace: false,
  });
}

const noWorkspaceContext = {
  workspaceStatus: 'workspace_required',
  permissionsSettled: true,
  organizationRole: 'normal',
  workspaceRole: null,
  permissions: [],
};

for (const href of Object.values(routes)) {
  assert.equal(
    getZGIConsoleNavigationAccess(href, noWorkspaceContext).status,
    'allowed',
    `${href} must remain usable without a workspace`
  );
}

assert.equal(
  resolveZGIConsoleNavigationRoute(routes.connection)?.label,
  'Personal Connection',
  'the static connections segment must resolve before generic Connector detail'
);
assert.equal(resolveZGIConsoleNavigationRoute(routes.detail)?.label, 'Connector Detail');
assert.equal(
  resolveZGIConsoleNavigationRoute(routes.connections)?.label,
  'My Connections',
  'the connections index must resolve as an instance list, never as Connector detail'
);

const connectorSiteMapEntry = ZGI_CONSOLE_SITE_MAP.find(route => route.href === routes.center);
assert.equal(connectorSiteMapEntry?.scope, 'organization');
assert.deepEqual(connectorSiteMapEntry?.permissions, []);

const consoleLayoutSource = readFileSync(
  path.join(root, 'src', 'app', 'console', 'layout.tsx'),
  'utf8'
);
assert.match(
  consoleLayoutSource,
  /<ProtectedRoute>\s*<OrganizationContextRequired>[\s\S]*<ConsoleShell>\{children\}<\/ConsoleShell>/,
  'always-on Connector routes must remain inside authentication and organization boundaries'
);
assert.match(consoleLayoutSource, /user\?\.current_organization_id === null/);
assert.match(consoleLayoutSource, /router\.replace\('\/onboarding\/organization'\)/);

const requiredPages = [
  path.join(root, 'src', 'app', 'console', 'connectors', 'page.tsx'),
  path.join(root, 'src', 'app', 'console', 'connectors', '[connectorId]', 'page.tsx'),
  path.join(root, 'src', 'app', 'console', 'connectors', 'connections', 'page.tsx'),
  path.join(
    root,
    'src',
    'app',
    'console',
    'connectors',
    'connections',
    '[connectionId]',
    'page.tsx'
  ),
];
for (const page of requiredPages) {
  assert.equal(existsSync(page), true, `${path.relative(root, page)} must exist`);
  const source = readFileSync(page, 'utf8');
  assert.doesNotMatch(
    source,
    /useSystemFeatures|enable_connectors|featureLoading/,
    'Connector routes must stay accessible when legacy feature metadata is missing or false'
  );
  assert.match(
    source,
    /<(?:ConnectorCenter|ConnectorDetailPage|ConnectionManagementPage|PersonalConnectionsPage)\b/
  );
  if (!source.includes('<PersonalConnectionsPage')) {
    assert.match(source, /\benabled\b[^>]*\/>/);
  }
}

const sidebarSource = readFileSync(
  path.join(root, 'src', 'components', 'console', 'console-sidebar.tsx'),
  'utf8'
);
assert.doesNotMatch(sidebarSource, /enable_connectors|connectorsEnabled/);
assert.equal(
  [
    ...sidebarSource.matchAll(
      /items:\s*\[\s*\{\s*title:\s*t\('connectors'\),\s*href:\s*'\/console\/connectors'/g
    ),
  ].length,
  2,
  'desktop and mobile sidebars must register Connector without a feature flag'
);
assert.match(sidebarSource, /resolveConsoleNavGroups/);
assert.doesNotMatch(sidebarSource, /enable_external_integrations/);

const headerSource = readFileSync(
  path.join(root, 'src', 'components', 'console', 'console-header.tsx'),
  'utf8'
);
assert.match(headerSource, /startsWith\(['"]\/console\/connectors['"]\)/);
assert.match(headerSource, /tNav\(['"]connectors['"]\)/);

const centerSource = readFileSync(
  path.join(connectorRoot, 'catalog', 'connector-center.tsx'),
  'utf8'
);
assert.match(centerSource, /<Breadcrumb/);
assert.match(centerSource, /tNavigation\(['"]tools['"]\)/);

const detailSource = readFileSync(
  path.join(connectorRoot, 'detail', 'connector-detail-page.tsx'),
  'utf8'
);
const detailContentPosition = detailSource.indexOf('<ConnectorCapabilityList');
const setupPanelPosition = detailSource.indexOf('<ConnectorConnectionsPanel');
assert.ok(detailContentPosition >= 0 && setupPanelPosition >= 0);
assert.ok(
  detailContentPosition < setupPanelPosition,
  'narrow Connector detail layouts must place capabilities before instance selection and add-Key entry'
);

const connectorIconSource = readFileSync(path.join(connectorRoot, 'connector-icon.tsx'), 'utf8');
const connectorStatusSource = readFileSync(
  path.join(connectorRoot, 'connector-status-badge.tsx'),
  'utf8'
);
const connectorPageStateSource = readFileSync(
  path.join(connectorRoot, 'connector-page-state.tsx'),
  'utf8'
);
assert.match(connectorIconSource, /role="img"/);
assert.match(connectorIconSource, /aria-label=\{name\}/);
assert.match(connectorIconSource, /size\?: 'sm' \| 'md' \| 'lg'/);
assert.match(connectorStatusSource, /className\?: string/);
assert.match(connectorPageStateSource, /'catalog' \| 'detail' \| 'connection'/);
assert.match(connectorPageStateSource, /kind: 'error' \| 'empty' \| 'disabled' \| 'not_found'/);
assert.match(connectorPageStateSource, /export function ConnectorReadinessBanner/);

function sourceFiles(directory) {
  return readdirSync(directory).flatMap(name => {
    const candidate = path.join(directory, name);
    return statSync(candidate).isDirectory()
      ? sourceFiles(candidate)
      : /\.(?:ts|tsx)$/.test(candidate)
        ? [candidate]
        : [];
  });
}

for (const file of sourceFiles(connectorRoot)) {
  const source = readFileSync(file, 'utf8');
  assert.doesNotMatch(
    source,
    /@\/components\/integrations|@\/services\/types\/integration|@\/services\/integration/i,
    `${path.relative(root, file)} must stay independent from legacy integration modules`
  );
  assert.doesNotMatch(
    source,
    /credentialSource|credential_source|allowedActionIds|allowed_action_ids|actionPolicies|grantIds|scopeIds/,
    `${path.relative(root, file)} must not reintroduce member-configured permission concepts`
  );
}

console.log('Connector always-on route, access, and module-boundary checks passed.');
