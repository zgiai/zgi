import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import vm from 'node:vm';
import test from 'node:test';
import ts from 'typescript';

const require = createRequire(import.meta.url);
const pagePath = '../src/app/dashboard/organization/audit-logs/page.tsx';
const pageCode = ts.transpileModule(readFileSync(new URL(pagePath, import.meta.url), 'utf8'), {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.CommonJS,
    jsx: ts.JsxEmit.ReactJSX,
  },
  fileName: pagePath,
}).outputText;

// Renders the page once with stubbed hooks and returns its element tree and
// what the audit-log query was asked to do.
function renderPage() {
  const queryCalls = [];
  const setters = [];
  let refetches = 0;
  const component = name => {
    const stub = () => null;
    stub.displayName = name;
    return stub;
  };
  const stubs = names => Object.fromEntries(names.map(name => [name, component(name)]));
  const mocks = {
    'react/jsx-runtime': require('react/jsx-runtime'),
    react: {
      useState: initial => {
        const value = typeof initial === 'function' ? initial() : initial;
        const setter = next => setters.push(next);
        return [value, setter];
      },
      useMemo: factory => factory(),
    },
    'date-fns': require('date-fns'),
    axios: { isAxiosError: () => false },
    'lucide-react': stubs(['RefreshCw', 'X']),
    '@/components/ui/badge': stubs(['Badge']),
    '@/components/ui/button': stubs(['Button']),
    '@/components/ui/card': stubs(['Card', 'CardContent']),
    '@/components/ui/select': stubs([
      'Select',
      'SelectContent',
      'SelectItem',
      'SelectTrigger',
      'SelectValue',
    ]),
    '@/components/ui/sheet': stubs([
      'Sheet',
      'SheetContent',
      'SheetDescription',
      'SheetHeader',
      'SheetTitle',
    ]),
    '@/components/ui/skeleton': stubs(['Skeleton']),
    '@/components/ui/table': stubs([
      'Table',
      'TableBody',
      'TableCell',
      'TableHead',
      'TableHeader',
      'TableRow',
    ]),
    '@/hooks/organization/use-organization-audit-logs': {
      useOrganizationAuditLogs: (organizationId, params, enabled) => {
        queryCalls.push({ organizationId, params, enabled });
        return {
          data: { items: [], total: 0, page: 1, limit: 20, has_more: false },
          isLoading: false,
          isFetching: false,
          isError: false,
          error: null,
          refetch: async () => {
            refetches += 1;
          },
        };
      },
    },
    '@/hooks/organization/use-organizations': {
      useOrganizations: () => ({
        currentOrganization: { id: 'org-1', organization_role: 'owner' },
      }),
    },
    '@/i18n': { useT: () => key => key },
    '@/store/auth-store': {
      useAuthStore: { use: { user: () => ({ organization_role: 'owner' }) } },
    },
  };
  const module = { exports: {} };
  vm.runInNewContext(pageCode, {
    module,
    exports: module.exports,
    require: name => {
      assert.ok(Object.hasOwn(mocks, name), `Unexpected dependency: ${name}`);
      return mocks[name];
    },
  });
  const tree = module.exports.default();
  return { tree, queryCalls, setters, refetches: () => refetches };
}

function findElement(node, predicate) {
  if (!node || typeof node !== 'object') return null;
  if (Array.isArray(node)) {
    for (const child of node) {
      const found = findElement(child, predicate);
      if (found) return found;
    }
    return null;
  }
  if (predicate(node)) return node;
  return findElement(node.props?.children, predicate);
}

function hasText(node, text) {
  if (node === text) return true;
  if (!node || typeof node !== 'object') return false;
  if (Array.isArray(node)) return node.some(child => hasText(child, text));
  return hasText(node.props?.children, text);
}

test('Refresh refetches even when the query key does not change', () => {
  const page = renderPage();
  const refresh = findElement(
    page.tree,
    node => node.type?.displayName === 'Button' && hasText(node.props?.children, 'refresh')
  );
  assert.ok(refresh, 'Refresh button not found');

  refresh.props.onClick();

  assert.equal(page.refetches(), 1, 'Refresh must refetch the audit log');
  // The refresh time still advances so the date range starts from today.
  assert.equal(page.setters.length, 1);
  assert.equal(typeof page.setters[0], 'number');
});

test('query params carry only API filters', () => {
  const page = renderPage();
  assert.equal(page.queryCalls.length, 1);
  const { organizationId, params, enabled } = page.queryCalls[0];
  assert.equal(organizationId, 'org-1');
  assert.equal(enabled, true);
  assert.deepEqual(
    Object.keys(params).sort(),
    ['action', 'actor_account_id', 'from', 'limit', 'page', 'result', 'target_account_id'].sort()
  );
  assert.equal(typeof params.from, 'number');
});
