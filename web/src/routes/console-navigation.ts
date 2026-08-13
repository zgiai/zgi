import type { PermissionCode } from '@/constants/permissions';

export type ZGIConsoleRouteScope = 'organization' | 'workspace';
export type ZGIConsoleNavigationAccessStatus =
  | 'allowed'
  | 'unsupported'
  | 'workspace_required'
  | 'permissions_loading'
  | 'permission_denied';

export interface ZGIConsoleNavigationAccessContext {
  workspaceStatus: 'loading' | 'ready' | 'workspace_required';
  permissionsSettled: boolean;
  organizationRole?: 'owner' | 'admin' | 'normal' | null;
  workspaceRole?: string | null;
  permissions: readonly string[];
}

export interface ZGIConsoleRouteDefinition {
  href: string;
  label: string;
  purpose: string;
  scope: ZGIConsoleRouteScope;
  permissions: readonly PermissionCode[];
  workspaceManagerOnly?: boolean;
}

export interface ZGIConsoleNavigationAccessResult {
  status: ZGIConsoleNavigationAccessStatus;
  allowed: boolean;
  href: string | null;
  route: ZGIConsoleRouteDefinition | null;
  requiredPermissions: readonly PermissionCode[];
}

const AGENT_PAGE_PERMISSIONS = [
  'agent.view',
  'agent.create',
  'agent.logs.view',
  'agent.update',
  'agent.delete',
  'agent.move',
  'agent.publish',
  'agent.runtime_access.manage',
] as const satisfies readonly PermissionCode[];

const AGENT_EDITOR_PERMISSIONS = [
  'agent.create',
  'agent.update',
  'agent.publish',
  'agent.runtime_access.manage',
] as const satisfies readonly PermissionCode[];

const WORKFLOW_PAGE_PERMISSIONS = [
  'workflow.create',
  'workflow.import',
  'workflow.view',
  'workflow.logs.view',
  'workflow.update',
  'workflow.delete',
  'workflow.move',
  'workflow.run.draft',
  'workflow.publish',
  'workflow.runtime_access.manage',
] as const satisfies readonly PermissionCode[];

const WORKFLOW_EDITOR_PERMISSIONS = [
  'workflow.create',
  'workflow.import',
  'workflow.update',
  'workflow.run.draft',
  'workflow.publish',
  'workflow.runtime_access.manage',
] as const satisfies readonly PermissionCode[];

const AGENT_ASSET_EDITOR_PERMISSIONS = [
  ...AGENT_EDITOR_PERMISSIONS,
  ...WORKFLOW_EDITOR_PERMISSIONS,
] as const satisfies readonly PermissionCode[];

const KNOWLEDGE_BASE_PAGE_PERMISSIONS = [
  'knowledge_base.view',
  'knowledge_base.create',
  'knowledge_base.folder_manage',
  'knowledge_base.retrieval_test',
  'knowledge_base.document.view',
  'knowledge_base.graph.view',
  'knowledge_base.update',
  'knowledge_base.delete',
  'knowledge_base.move',
  'knowledge_base.document.create',
  'knowledge_base.document.update',
  'knowledge_base.document.delete',
  'knowledge_base.segment.update',
  'knowledge_base.segment.delete',
  'knowledge_base.index.manage',
  'knowledge_base.graph.manage',
] as const satisfies readonly PermissionCode[];

const DATABASE_PAGE_PERMISSIONS = [
  'database.view',
  'database.create',
  'database.update',
  'database.delete',
  'database.move',
  'database.schema.view',
  'database.schema.manage',
  'database.record.view',
  'database.record.create',
  'database.record.update',
  'database.record.delete',
  'database.import.analyze',
  'database.import.execute',
  'database.operation_logs.view',
  'database.sql_audit.view',
  'database.ai_query.read',
] as const satisfies readonly PermissionCode[];

const DATABASE_RECORD_PERMISSIONS = [
  'database.record.view',
  'database.record.create',
  'database.record.update',
  'database.record.delete',
] as const satisfies readonly PermissionCode[];

const DATABASE_TABLE_PERMISSIONS = [
  'database.schema.view',
  'database.schema.manage',
  ...DATABASE_RECORD_PERMISSIONS,
] as const satisfies readonly PermissionCode[];

const NO_PERMISSIONS = [] as const satisfies readonly PermissionCode[];

export const ZGI_CONSOLE_SITE_MAP = [
  {
    href: '/console',
    label: 'Home',
    purpose: 'workspace overview and entry point',
    scope: 'organization',
    permissions: NO_PERMISSIONS,
  },
  {
    href: '/console/work/chat',
    label: 'Conversations',
    purpose: 'chat workbench',
    scope: 'organization',
    permissions: NO_PERMISSIONS,
  },
  {
    href: '/console/work/image',
    label: 'Images',
    purpose: 'image/drawing workbench',
    scope: 'organization',
    permissions: NO_PERMISSIONS,
  },
  {
    href: '/console/work/video',
    label: 'Videos',
    purpose: 'video generation workbench',
    scope: 'organization',
    permissions: NO_PERMISSIONS,
  },
  {
    href: '/console/work/app',
    label: 'Apps',
    purpose: 'web app workbench',
    scope: 'organization',
    permissions: NO_PERMISSIONS,
  },
  {
    href: '/console/work/task',
    label: 'Scheduled Tasks',
    purpose: 'scheduled task management',
    scope: 'workspace',
    permissions: NO_PERMISSIONS,
  },
  {
    href: '/console/agents',
    label: 'Agents',
    purpose: 'create, configure, debug, and publish agents',
    scope: 'workspace',
    permissions: AGENT_PAGE_PERMISSIONS,
  },
  {
    href: '/console/workflows',
    label: 'Workflows',
    purpose: 'create, configure, debug, and publish workflows',
    scope: 'workspace',
    permissions: WORKFLOW_PAGE_PERMISSIONS,
  },
  {
    href: '/console/dataset',
    label: 'Knowledge Bases',
    purpose: 'knowledge base and document assets',
    scope: 'workspace',
    permissions: KNOWLEDGE_BASE_PAGE_PERMISSIONS,
  },
  {
    href: '/console/db',
    label: 'Databases',
    purpose: 'database sources and table records',
    scope: 'workspace',
    permissions: DATABASE_PAGE_PERMISSIONS,
  },
  {
    href: '/console/files',
    label: 'Files',
    purpose: 'uploaded files and managed workspace files',
    scope: 'workspace',
    permissions: NO_PERMISSIONS,
  },
  {
    href: '/console/skills',
    label: 'Skills',
    purpose: 'organization skill management',
    scope: 'organization',
    permissions: NO_PERMISSIONS,
  },
  {
    href: '/console/prompts',
    label: 'Prompts',
    purpose: 'prompt library',
    scope: 'workspace',
    permissions: NO_PERMISSIONS,
  },
  {
    href: '/console/developer/content-parse',
    label: 'File Recognition',
    purpose: 'content parsing and recognition tools',
    scope: 'workspace',
    permissions: NO_PERMISSIONS,
  },
  {
    href: '/console/workspace',
    label: 'Workspace',
    purpose: 'workspace administration',
    scope: 'workspace',
    permissions: NO_PERMISSIONS,
  },
  {
    href: '/console/workspace/members',
    label: 'Workspace Members',
    purpose: 'workspace member management',
    scope: 'workspace',
    permissions: NO_PERMISSIONS,
    workspaceManagerOnly: true,
  },
  {
    href: '/console/workspace/settings',
    label: 'Workspace Settings',
    purpose: 'workspace configuration',
    scope: 'workspace',
    permissions: NO_PERMISSIONS,
  },
] as const satisfies readonly ZGIConsoleRouteDefinition[];

interface ZGIConsoleDynamicRouteDefinition {
  pattern: RegExp;
  route: Omit<ZGIConsoleRouteDefinition, 'href'>;
}

const ZGI_CONSOLE_EXACT_ROUTES: ReadonlyMap<string, ZGIConsoleRouteDefinition> = new Map(
  ZGI_CONSOLE_SITE_MAP.map(route => [route.href, route])
);

const ZGI_CONSOLE_DYNAMIC_ROUTES: readonly ZGIConsoleDynamicRouteDefinition[] = [
  {
    pattern: /^\/console\/agents\/[A-Za-z0-9_-]+\/logs$/,
    route: {
      label: 'Agent Logs',
      purpose: 'agent runtime logs',
      scope: 'workspace',
      permissions: ['agent.logs.view', 'workflow.logs.view'],
    },
  },
  {
    pattern: /^\/console\/agents\/[A-Za-z0-9_-]+\/api\/(keys|docs)$/,
    route: {
      label: 'Agent API',
      purpose: 'agent API access management',
      scope: 'workspace',
      permissions: ['agent.runtime_access.manage', 'workflow.runtime_access.manage'],
    },
  },
  {
    pattern: /^\/console\/agents\/[A-Za-z0-9_-]+\/batch-test$/,
    route: {
      label: 'Agent Batch Test',
      purpose: 'workflow batch testing',
      scope: 'workspace',
      permissions: ['workflow.view'],
    },
  },
  {
    pattern: /^\/console\/agents\/[A-Za-z0-9_-]+\/agent$/,
    route: {
      label: 'Agent Detail',
      purpose: 'agent editor',
      scope: 'workspace',
      permissions: AGENT_EDITOR_PERMISSIONS,
    },
  },
  {
    pattern: /^\/console\/agents\/[A-Za-z0-9_-]+\/workflow$/,
    route: {
      label: 'Workflow Detail',
      purpose: 'legacy workflow editor route',
      scope: 'workspace',
      permissions: WORKFLOW_EDITOR_PERMISSIONS,
    },
  },
  {
    pattern: /^\/console\/workflows\/[A-Za-z0-9_-]+\/logs$/,
    route: {
      label: 'Workflow Logs',
      purpose: 'workflow runtime logs',
      scope: 'workspace',
      permissions: ['workflow.logs.view'],
    },
  },
  {
    pattern: /^\/console\/workflows\/[A-Za-z0-9_-]+\/api\/(keys|docs)$/,
    route: {
      label: 'Workflow API',
      purpose: 'workflow API access management',
      scope: 'workspace',
      permissions: ['workflow.runtime_access.manage'],
    },
  },
  {
    pattern: /^\/console\/workflows\/[A-Za-z0-9_-]+\/batch-test$/,
    route: {
      label: 'Workflow Batch Test',
      purpose: 'workflow batch testing',
      scope: 'workspace',
      permissions: ['workflow.view'],
    },
  },
  {
    pattern: /^\/console\/workflows\/[A-Za-z0-9_-]+$/,
    route: {
      label: 'Workflow Detail',
      purpose: 'workflow editor',
      scope: 'workspace',
      permissions: WORKFLOW_EDITOR_PERMISSIONS,
    },
  },
  {
    pattern:
      /^\/console\/dataset\/[A-Za-z0-9_-]+(\/(documents|graph|hit-testing|batch-testing|settings))?$/,
    route: {
      label: 'Knowledge Base Detail',
      purpose: 'knowledge base detail',
      scope: 'workspace',
      permissions: KNOWLEDGE_BASE_PAGE_PERMISSIONS,
    },
  },
  {
    pattern: /^\/console\/db\/[A-Za-z0-9_-]+\/record$/,
    route: {
      label: 'Database Records',
      purpose: 'database table records',
      scope: 'workspace',
      permissions: DATABASE_RECORD_PERMISSIONS,
    },
  },
  {
    pattern: /^\/console\/db\/[A-Za-z0-9_-]+\/search$/,
    route: {
      label: 'Database Search',
      purpose: 'AI database query',
      scope: 'workspace',
      permissions: ['database.ai_query.read'],
    },
  },
  {
    pattern: /^\/console\/db\/[A-Za-z0-9_-]+\/import-excel$/,
    route: {
      label: 'Database Import',
      purpose: 'database import',
      scope: 'workspace',
      permissions: ['database.import.analyze', 'database.import.execute'],
    },
  },
  {
    pattern: /^\/console\/db\/[A-Za-z0-9_-]+\/table\/[A-Za-z0-9_-]+$/,
    route: {
      label: 'Database Table',
      purpose: 'database table detail',
      scope: 'workspace',
      permissions: DATABASE_TABLE_PERMISSIONS,
    },
  },
  {
    pattern: /^\/console\/db\/[A-Za-z0-9_-]+$/,
    route: {
      label: 'Database Detail',
      purpose: 'database detail',
      scope: 'workspace',
      permissions: DATABASE_PAGE_PERMISSIONS,
    },
  },
  {
    pattern: /^\/console\/prompts\/[A-Za-z0-9_-]+$/,
    route: {
      label: 'Prompt Detail',
      purpose: 'prompt detail',
      scope: 'workspace',
      permissions: NO_PERMISSIONS,
    },
  },
  {
    pattern: /^\/console\/work\/app\/[A-Za-z0-9_-]+$/,
    route: {
      label: 'App Detail',
      purpose: 'web app detail',
      scope: 'organization',
      permissions: NO_PERMISSIONS,
    },
  },
];

function normalizeRoutePath(rawHref: string | undefined): string | null {
  const text = rawHref?.trim();
  if (!text || text.includes('..') || /^https?:\/\//i.test(text) || text.startsWith('//')) {
    return null;
  }

  const [rawPath] = text.split(/[?#]/, 1);
  const path = `/${rawPath.replace(/^\/+/, '')}`.replace(/\/+$/, '') || '/';
  return path;
}

function canonicalizeApiIndexRoute(href: string): string {
  if (/^\/console\/(agents|workflows)\/[A-Za-z0-9_-]+\/api$/.test(href)) {
    return `${href}/keys`;
  }
  return href;
}

export function resolveZGIConsoleNavigationRoute(
  rawHref: string | undefined
): ZGIConsoleRouteDefinition | null {
  const normalizedHref = normalizeRoutePath(rawHref);
  if (!normalizedHref) return null;
  const href = canonicalizeApiIndexRoute(normalizedHref);

  const exactRoute = ZGI_CONSOLE_EXACT_ROUTES.get(href);
  if (exactRoute) return exactRoute;

  if (/^\/console\/agents\/[A-Za-z0-9_-]+$/.test(href)) {
    return {
      href: `${href}/agent`,
      label: 'Agent Detail',
      purpose: 'agent or legacy workflow detail',
      scope: 'workspace',
      permissions: AGENT_ASSET_EDITOR_PERMISSIONS,
    };
  }

  const dynamicRoute = ZGI_CONSOLE_DYNAMIC_ROUTES.find(candidate => candidate.pattern.test(href));
  if (!dynamicRoute) return null;
  return { href, ...dynamicRoute.route };
}

export function normalizeZGIConsoleNavigationHref(rawHref: string | undefined): string | null {
  return resolveZGIConsoleNavigationRoute(rawHref)?.href ?? null;
}

function isOrganizationAdmin(context: ZGIConsoleNavigationAccessContext) {
  return context.organizationRole === 'owner' || context.organizationRole === 'admin';
}

function isWorkspaceManager(context: ZGIConsoleNavigationAccessContext) {
  const workspaceRole = context.workspaceRole?.trim().toLowerCase();
  return isOrganizationAdmin(context) || workspaceRole === 'owner' || workspaceRole === 'admin';
}

function accessResult(
  status: ZGIConsoleNavigationAccessStatus,
  route: ZGIConsoleRouteDefinition | null
): ZGIConsoleNavigationAccessResult {
  return {
    status,
    allowed: status === 'allowed',
    href: route?.href ?? null,
    route,
    requiredPermissions: route?.permissions ?? NO_PERMISSIONS,
  };
}

export function getZGIConsoleNavigationAccess(
  rawHref: string | undefined,
  context: ZGIConsoleNavigationAccessContext
): ZGIConsoleNavigationAccessResult {
  const route = resolveZGIConsoleNavigationRoute(rawHref);
  if (!route) return accessResult('unsupported', null);
  if (route.scope === 'organization') return accessResult('allowed', route);
  if (context.workspaceStatus !== 'ready') {
    return accessResult(
      context.workspaceStatus === 'loading' ? 'permissions_loading' : 'workspace_required',
      route
    );
  }
  if (!context.permissionsSettled) return accessResult('permissions_loading', route);
  if (route.workspaceManagerOnly && !isWorkspaceManager(context)) {
    return accessResult('permission_denied', route);
  }
  if (route.permissions.length === 0 || isOrganizationAdmin(context)) {
    return accessResult('allowed', route);
  }

  const grantedPermissions = new Set(context.permissions);
  return accessResult(
    route.permissions.some(permission => grantedPermissions.has(permission))
      ? 'allowed'
      : 'permission_denied',
    route
  );
}

export function getAccessibleZGIConsoleSiteMap(
  context: ZGIConsoleNavigationAccessContext
): readonly ZGIConsoleRouteDefinition[] {
  return ZGI_CONSOLE_SITE_MAP.filter(
    route => getZGIConsoleNavigationAccess(route.href, context).allowed
  );
}
