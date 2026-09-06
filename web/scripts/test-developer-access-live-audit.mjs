import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import vm from 'node:vm';
import { URL, URLSearchParams } from 'node:url';
import test from 'node:test';
import ts from 'typescript';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { QueryClient, QueryObserver } from '@tanstack/react-query';

const require = createRequire(import.meta.url);
function load(relative, mocks, extra = '') {
  const filename = new URL(`../src/${relative}`, import.meta.url);
  const code = ts.transpileModule(readFileSync(filename, 'utf8') + extra, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.CommonJS,
      jsx: ts.JsxEmit.ReactJSX,
      esModuleInterop: true,
    },
    fileName: filename.pathname,
  }).outputText;
  const module = { exports: {} };
  vm.runInNewContext(code, {
    module,
    exports: module.exports,
    require: name => (Object.hasOwn(mocks, name) ? mocks[name] : require(name)),
    console,
    Date,
    Intl: globalThis.Intl,
  });
  return module.exports;
}

for (const manager of [false, true]) {
  test(`refresh respects workspace and enabled queries (manager=${manager})`, async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: Infinity } },
    });
    const calls = [];
    const stops = [];
    const observers = [];
    client.setQueryData(['developer-access', 'A', 'me'], { can_manage: manager });
    const service = new Proxy(
      {},
      {
        get: (_, method) => async (workspace, input) => {
          calls.push({ method, workspace, scope: input?.scope });
          return { data: method === 'getMe' ? { can_manage: manager } : { items: [] } };
        },
      }
    );
    const { useDeveloperAccess } = load('hooks/developer-access/use-developer-access.ts', {
      '@tanstack/react-query': {
        useQueryClient: () => client,
        useQuery: options => {
          const observer = new QueryObserver(client, options);
          observers.push(observer);
          stops.push(observer.subscribe(() => {}));
          return observer.getCurrentResult();
        },
      },
      '@/services/developer-access.service': { developerAccessService: service },
      '@/utils/ai-credits': new Proxy({}, { get: () => value => value }),
      '@/i18n': { useT: () => key => key },
      sonner: {},
    });
    try {
      const access = useDeveloperAccess('A');
      const other = new QueryObserver(client, {
        queryKey: ['developer-access', 'B', 'me'],
        queryFn: async () => {
          calls.push({ workspace: 'B' });
          return {};
        },
      });
      observers.push(other);
      stops.push(other.subscribe(() => {}));
      await client.refetchQueries({ type: 'active' });
      calls.length = 0;
      await access.refresh();
      assert.equal(calls.length, manager ? 6 : 4);
      assert.ok(calls.every(call => call.workspace === 'A'));
      assert.equal(calls.filter(call => call.scope === 'members').length, manager ? 2 : 0);
      assert.ok(calls.some(call => call.method === 'getMe'));
      assert.ok(calls.some(call => call.method === 'listAudit'));
      calls.length = 0;
      await useDeveloperAccess(undefined).refresh();
      assert.equal(calls.length, 0, 'no workspace must not issue a request');
    } finally {
      stops.forEach(stop => stop());
      observers.forEach(observer => observer.destroy());
      client.clear();
    }
  });
}

const translate = key => key;
const passthrough = ({ children }) => React.createElement('div', null, children);
const ui = new Proxy({}, { get: () => passthrough });
test('audit renders exact request ID and input/output tokens without exposing a secret', () => {
  const mocks = {
    './load-error': {},
    '@/i18n': { useT: () => (key, values) => (values ? `${key} ${JSON.stringify(values)}` : key) },
    '@/lib/config': {},
    '@/utils/ai-credits': {},
    '@/store/workspace-store': {},
    '@/hooks/developer-access/use-developer-access': {},
  };
  for (const name of [
    'badge',
    'button',
    'card',
    'dialog',
    'dropdown-menu',
    'input',
    'label',
    'skeleton',
    'tabs',
    'textarea',
    'table',
  ]) {
    mocks[`@/components/ui/${name}`] = ui;
  }
  const { AuditList } = load(
    'features/developer-access/developer-access-page.tsx',
    mocks,
    '\nexports.AuditList = AuditList;'
  );
  const markup = renderToStaticMarkup(
    React.createElement(AuditList, {
      items: [
        {
          attempt_id: 'attempt-1',
          request_id: 'request-123',
          principal_id: 'user-1',
          api_key_id: 'key-1',
          api_key_name: 'Example',
          api_key_masked: 'zgi_abcd••••1234',
          model_name: 'model-1',
          provider_name: 'provider-1',
          status: 'success',
          prompt_tokens: 10,
          completion_tokens: 2,
          total_tokens: 12,
          total_points: 0.045,
          quota_overage_points: 0,
          quota_charged_points: 0.045,
          created_at: '2026-09-06T14:38:00Z',
          secret: 'must-not-render',
        },
      ],
      loading: false,
      error: false,
      showPrincipal: false,
      total: 1,
      page: 1,
      pageSize: 50,
      onPageChange() {},
    })
  );
  assert.match(markup, /request-123/);
  assert.match(markup, /audit.tokenBreakdown.*10.*2/);
  assert.match(markup, /0\.045/);
  assert.doesNotMatch(markup, /must-not-render/);
});

test('desktop and mobile expose the same root links; mobile closes on navigation', () => {
  const links = [];
  const closed = [];
  const mocks = {
    'next/link': ({ children, ...props }) => {
      links.push(props);
      return React.createElement(
        'a',
        { href: props.href, 'aria-current': props['aria-current'] },
        children
      );
    },
    'next/navigation': {
      usePathname: () => '/console/api-keys',
      useSearchParams: () => new URLSearchParams(),
    },
    '@/i18n': { useT: () => translate },
    '@/lib/utils': {
      cn: (...values) => values.filter(value => typeof value === 'string').join(' '),
    },
    '@/lib/config': { withBasePathIfInternal: value => value },
    '@/components/ui/button': ui,
    '@/components/ui/sheet': ui,
    './team-switcher': { WorkspaceSwitcher: () => null },
    '@/hooks/organization/use-account-permissions': {
      useAccountPermissions: () => ({
        permissions: [],
        isLoading: false,
        error: new Error('offline'),
      }),
    },
    '@/store/workspace-store': { useWorkspaceStore: { use: { contextStatus: () => 'ready' } } },
    '@/components/workflow/hooks/use-debug-focus-mode': { useWorkflowDebugFocusMode: () => false },
    '@/hooks/use-persistent-sidebar-collapse': {
      usePersistentSidebarCollapse: () => [false, () => {}],
    },
    '@/routes/console-navigation': {
      getZGIConsoleNavigationAccess: () => ({ status: 'forbidden' }),
      getZGIConsoleNavigationDisplayState: () => 'error',
      getZGIConsoleNavigationTarget: path => path,
    },
  };
  const { ConsoleSidebar, ConsoleMobileSidebar } = load(
    'components/console/console-sidebar.tsx',
    mocks
  );
  for (const [component, props] of [
    [ConsoleSidebar, {}],
    [ConsoleMobileSidebar, { open: true, onOpenChange: value => closed.push(value) }],
  ]) {
    links.length = 0;
    const markup = renderToStaticMarkup(React.createElement(component, props));
    for (const href of ['/console/model', '/console/api-keys']) {
      assert.equal(links.filter(link => link.href === href).length, 1);
      if (component === ConsoleMobileSidebar) links.find(link => link.href === href).onClick();
    }
    assert.match(markup, /href="\/console\/api-keys" aria-current="page"/);
  }
  assert.deepEqual(closed, [false, false]);
});

test('manual refresh and tab changes are wired to the scoped query refresh', () => {
  const source = readFileSync(
    new URL('../src/features/developer-access/developer-access-page.tsx', import.meta.url),
    'utf8'
  );
  assert.match(source, /disabled=\{isRefreshing\} onClick=\{\(\) => void refresh\(\)\}/);
  assert.match(
    source,
    /<Tabs key=\{workspaceId\} defaultValue="keys" onValueChange=\{\(\) => void refresh\(\)\}/
  );
});
