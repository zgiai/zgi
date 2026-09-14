'use client';

import * as React from 'react';
import Link from 'next/link';
import {
  ArrowRightToLine,
  Atom,
  Bot,
  BookText,
  FileText,
  Database,
  BookOpen,
  Users,
  MessageSquare,
  Image as ImageIcon,
  Video,
  Music2,
  AppWindow,
  Clock3,
  ChevronDown,
  PlugZap,
  Workflow,
  LayoutGrid,
  House,
  CircleAlert,
  LoaderCircle,
  LockKeyhole,
  KeyRound,
} from 'lucide-react';
import { usePathname, useSearchParams } from 'next/navigation';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { WorkspaceSwitcher } from './team-switcher';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useWorkspaceStore } from '@/store/workspace-store';
import { withBasePathIfInternal } from '@/lib/config';
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { useWorkflowDebugFocusMode } from '@/components/workflow/hooks/use-debug-focus-mode';
import { usePersistentSidebarCollapse } from '@/hooks/use-persistent-sidebar-collapse';
import { useSystemFeatures } from '@/hooks/auth/use-system-features';
import {
  getZGIConsoleNavigationAccess,
  getZGIConsoleNavigationDisplayState,
  getZGIConsoleNavigationTarget,
  type ZGIConsoleNavigationDisplayState,
  type ZGIConsoleNavigationAccessContext,
} from '@/routes/console-navigation';

type NavAccessState = ZGIConsoleNavigationDisplayState;

interface NavItem {
  title: string;
  href: string;
  icon: React.ElementType;
  forcedAccessState?: NavAccessState;
  blockedReason?: string;
}

interface ResolvedNavItem extends NavItem {
  accessState: NavAccessState;
}

interface NavGroup {
  key: string;
  title: string;
  items: NavItem[];
}

interface ResolvedNavGroup extends Omit<NavGroup, 'items'> {
  items: ResolvedNavItem[];
}

interface RootRouteItem {
  key: string;
  title: string;
  href: string;
  icon: React.ElementType;
  target?: '_self' | '_blank';
  activeMatchPaths?: string[];
}

const ROOT_ROUTES = [
  { key: 'model-plaza', titleKey: 'modelPlaza', href: '/console/model', icon: LayoutGrid },
  { key: 'api-keys', titleKey: 'apiKeys', href: '/console/api-keys', icon: KeyRound },
] as const;

const STORAGE_KEY = 'zgi:console:sidebar:groups';

function CollapsedNavTooltip({ label, children }: { label: string; children: React.ReactElement }) {
  const child = children as React.ReactElement<{
    title?: string;
    'aria-label'?: string;
  }>;
  return React.cloneElement(child, {
    title: child.props.title ?? label,
    'aria-label': child.props['aria-label'] ?? label,
  });
}

function getDatasetReturnTo(value: string | null): string | null {
  if (!value) return null;
  if (!value.startsWith('/console/dataset/')) return null;
  if (value.startsWith('//') || value.includes('://')) return null;
  return value;
}

function isItemActive(pathname: string, href: string): boolean {
  return pathname === href || pathname.startsWith(`${href}/`);
}

function isRootRouteItemActive(pathname: string, item: RootRouteItem): boolean {
  const matchPaths = item.activeMatchPaths?.length ? item.activeMatchPaths : [item.href];

  return matchPaths.some(matchPath => {
    if (!matchPath.startsWith('/')) return false;
    return isItemActive(pathname, withBasePathIfInternal(matchPath));
  });
}

function resolveConsoleNavGroups(
  groups: NavGroup[],
  context: ZGIConsoleNavigationAccessContext,
  permissionsFailed: boolean
): ResolvedNavGroup[] {
  return groups
    .map(group => {
      const items = group.items.flatMap(item => {
        const access = getZGIConsoleNavigationAccess(item.href, context);
        if (access.status === 'unsupported') return [];

        const accessState: NavAccessState =
          item.forcedAccessState ?? getZGIConsoleNavigationDisplayState(access, permissionsFailed);

        return [{ ...item, accessState }];
      });

      return { ...group, items };
    })
    .filter(group => group.items.length > 0);
}

export function ConsoleSidebar({
  hidden,
  temporarilyCollapsed = false,
  autoCollapse = false,
}: {
  hidden?: boolean;
  temporarilyCollapsed?: boolean;
  autoCollapse?: boolean;
}) {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const t = useT('navigation');
  const datasetReturnTo = getDatasetReturnTo(searchParams.get('returnTo'));
  const activePathname = datasetReturnTo ? '/console/dataset' : pathname;

  // Permission checking
  const {
    permissions,
    organizationRole,
    workspaceRole,
    isLoading: isPermissionsLoading,
    error: permissionsError,
  } = useAccountPermissions();
  const contextStatus = useWorkspaceStore.use.contextStatus();
  const systemFeatures = useSystemFeatures();
  const externalIntegrationsAccessState: NavAccessState | undefined =
    systemFeatures.data !== undefined
      ? systemFeatures.data?.enable_external_integrations
        ? undefined
        : 'forbidden'
      : systemFeatures.isLoading
        ? 'loading'
        : systemFeatures.error
          ? 'error'
          : 'forbidden';
  const isDebugFocusMode = useWorkflowDebugFocusMode();

  // Collapsed state persisted via ui-local helpers
  const [persistedIsCollapsed, setIsCollapsed] = usePersistentSidebarCollapse(
    'console',
    autoCollapse,
    isDebugFocusMode || temporarilyCollapsed
  );
  const isTemporarilyCollapsed = isDebugFocusMode || temporarilyCollapsed;
  const layoutIsCollapsed = isTemporarilyCollapsed || persistedIsCollapsed;
  const isCollapsed = layoutIsCollapsed;
  const wasAutoCollapseActiveRef = React.useRef(false);

  React.useEffect(() => {
    if (autoCollapse && !wasAutoCollapseActiveRef.current && !persistedIsCollapsed) {
      setIsCollapsed(true);
    }
    wasAutoCollapseActiveRef.current = autoCollapse;
  }, [autoCollapse, persistedIsCollapsed, setIsCollapsed]);

  const toggleCollapse = () => setIsCollapsed(prev => !prev);

  // Group open state
  const [openGroups, setOpenGroups] = React.useState<Record<string, boolean>>(() => {
    if (typeof window === 'undefined') return {};
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      return raw
        ? (JSON.parse(raw) as Record<string, boolean>)
        : { work: true, resources: true, tools: true, management: true };
    } catch {
      return { work: true, resources: true, tools: true, management: true };
    }
  });

  React.useEffect(() => {
    if (typeof window === 'undefined') return;
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(openGroups));
    } catch {
      // ignore storage errors
    }
  }, [openGroups]);

  const toggleGroup = (key: string) => setOpenGroups(prev => ({ ...prev, [key]: !prev[key] }));

  // Define all nav groups
  const allNavGroups: NavGroup[] = React.useMemo(
    () => [
      {
        key: 'work',
        title: t('work'),
        items: [
          {
            title: t('image'),
            href: '/console/work/image',
            icon: ImageIcon,
          },
          {
            title: t('music'),
            href: '/console/work/music',
            icon: Music2,
          },
          {
            title: t('video'),
            href: '/console/work/video',
            icon: Video,
          },
          {
            title: t('app'),
            href: '/console/work/app',
            icon: AppWindow,
          },
          {
            title: t('task'),
            href: '/console/work/task',
            icon: Clock3,
          },
        ],
      },
      {
        key: 'resources',
        title: t('resources'),
        items: [
          {
            title: t('agents'),
            href: '/console/agents',
            icon: Atom,
          },
          {
            title: t('workflowAgents'),
            href: '/console/workflows',
            icon: Workflow,
          },
          {
            title: t('prompts'),
            href: '/console/prompts',
            icon: BookText,
          },
          {
            title: t('files'),
            href: '/console/files',
            icon: FileText,
          },
          {
            title: t('datasets'),
            href: '/console/dataset',
            icon: BookOpen,
          },
          {
            title: t('dbs'),
            href: '/console/db',
            icon: Database,
          },
        ],
      },
      {
        key: 'tools',
        title: t('tools'),
        items: [
          {
            title: t('integrations'),
            href: '/console/integrations',
            icon: PlugZap,
            forcedAccessState: externalIntegrationsAccessState,
            blockedReason:
              externalIntegrationsAccessState === 'forbidden' ? t('featureDisabled') : undefined,
          },
          {
            title: t('skills'),
            href: '/console/skills',
            icon: Bot,
          },
        ],
      },
      {
        key: 'management',
        title: t('management'),
        items: [
          {
            title: t('workspaceManagement'),
            href: '/console/workspace',
            icon: Users,
          },
        ],
      },
    ],
    [externalIntegrationsAccessState, t]
  );

  const navigationAccessContext = React.useMemo<ZGIConsoleNavigationAccessContext>(
    () => ({
      workspaceStatus: contextStatus,
      permissionsSettled: !isPermissionsLoading && permissionsError === null,
      organizationRole,
      workspaceRole,
      permissions,
    }),
    [
      contextStatus,
      isPermissionsLoading,
      organizationRole,
      permissions,
      permissionsError,
      workspaceRole,
    ]
  );

  // Keep the product map discoverable and represent blocked states explicitly.
  const navGroups = React.useMemo(() => {
    return resolveConsoleNavGroups(
      allNavGroups,
      navigationAccessContext,
      permissionsError !== null
    );
  }, [allNavGroups, navigationAccessContext, permissionsError]);

  const rootRouteItems = React.useMemo(
    (): RootRouteItem[] => ROOT_ROUTES.map(item => ({ ...item, title: t(item.titleKey) })),
    [t]
  );

  const workspaceHomeNavLink = (
    <Link
      href="/console"
      className={cn(
        'flex items-center gap-2 rounded-md py-1.5 text-[13px] transition-colors shrink-0 w-full',
        isCollapsed ? 'justify-center px-0 w-8' : 'justify-start px-2',
        'text-foreground/70 hover:bg-muted/70 hover:text-foreground',
        pathname === '/console' && 'bg-muted/80 text-foreground'
      )}
    >
      <House
        size={16}
        className={cn('shrink-0 text-foreground/65', pathname === '/console' && 'text-foreground')}
      />
      <span
        className={cn(
          'truncate transition-all duration-300 opacity-100 font-normal',
          isCollapsed && 'ml-0 opacity-0 w-0 hidden'
        )}
      >
        {t('home')}
      </span>
    </Link>
  );

  const chatNavLink = (
    <Link
      href="/console/work/chat"
      className={cn(
        'flex items-center gap-2 rounded-md py-1.5 text-[13px] transition-colors shrink-0 w-full',
        isCollapsed ? 'justify-center px-0 w-8' : 'justify-start px-2',
        'text-foreground/70 hover:bg-muted/70 hover:text-foreground',
        pathname.startsWith('/console/work/chat') && 'bg-muted/80 text-foreground'
      )}
    >
      <MessageSquare
        size={16}
        className={cn(
          'shrink-0 text-foreground/65',
          pathname.startsWith('/console/work/chat') && 'text-foreground'
        )}
      />
      <span
        className={cn(
          'truncate transition-all duration-300 opacity-100 font-normal',
          isCollapsed && 'ml-0 opacity-0 w-0 hidden'
        )}
      >
        {t('chat')}
      </span>
    </Link>
  );

  const sidebarContent = (
    <div className="flex flex-col flex-1 h-full overflow-hidden">
      {/* Workspace Switcher */}
      <div className="shrink-0 px-2 py-2">
        <WorkspaceSwitcher
          isCollapsed={isCollapsed}
          onOpen={() => {
            if (!isTemporarilyCollapsed) {
              setIsCollapsed(false);
            }
          }}
        />
      </div>
      {/* Navigation Items */}
      <nav
        className={cn(
          'flex flex-col gap-1 px-2 py-1 flex-1 overflow-y-auto overflow-x-hidden scrollbar-none transition-all duration-300',
          isCollapsed ? 'items-center' : 'items-start'
        )}
      >
        {isCollapsed ? (
          <CollapsedNavTooltip label={t('home')}>{workspaceHomeNavLink}</CollapsedNavTooltip>
        ) : (
          workspaceHomeNavLink
        )}
        {isCollapsed ? (
          <CollapsedNavTooltip label={t('chat')}>{chatNavLink}</CollapsedNavTooltip>
        ) : (
          chatNavLink
        )}

        {navGroups.map(group => {
          const isExpanded = openGroups[group.key] ?? true;

          return (
            <div
              key={group.key}
              className={cn(
                'w-full transition-[padding] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none',
                isCollapsed ? 'pt-0' : 'pt-2'
              )}
            >
              <button
                type="button"
                onClick={() => toggleGroup(group.key)}
                className={cn(
                  'flex w-full origin-left items-center justify-between overflow-hidden rounded-md px-2 text-[12px]',
                  'font-medium text-foreground/55 hover:bg-muted/60 hover:text-foreground/80',
                  'transition-[max-height,opacity,transform,padding] duration-200 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none',
                  isCollapsed
                    ? 'pointer-events-none max-h-0 -translate-x-1 py-0 opacity-0'
                    : 'max-h-7 translate-x-0 py-1 opacity-100'
                )}
                tabIndex={isCollapsed ? -1 : 0}
                aria-hidden={isCollapsed}
              >
                <span className="truncate">{group.title}</span>
                <ChevronDown
                  className={cn(
                    'h-3.5 w-3.5 shrink-0 text-foreground/45 transition-transform duration-200',
                    !isExpanded && '-rotate-90'
                  )}
                />
              </button>

              {(isCollapsed || isExpanded) && (
                <div className={cn('space-y-0.5', isCollapsed ? 'mt-0' : 'mt-1')}>
                  {group.items.map(item => {
                    const Icon = item.icon;
                    const isForbidden = item.accessState === 'forbidden';
                    const isLoading = item.accessState === 'loading';
                    const hasAccessError = item.accessState === 'error';
                    const isBlocked = isForbidden || isLoading || hasAccessError;
                    const needsSetup = item.accessState === 'setup_required';
                    const navigationTarget = getZGIConsoleNavigationTarget(
                      item.href,
                      item.accessState
                    );
                    const isActive = !isBlocked && isItemActive(activePathname, item.href);
                    const accessLabel = isForbidden
                      ? item.blockedReason || t('permissionRequired')
                      : isLoading
                        ? t('checkingAccess')
                        : hasAccessError
                          ? t('accessCheckFailed')
                          : needsSetup
                            ? t('setupRequired')
                            : item.title;
                    const navContent = (
                      <>
                        <Icon
                          size={16}
                          className={cn(
                            'shrink-0 text-foreground/60',
                            isActive && 'text-foreground',
                            isBlocked && 'text-foreground/30'
                          )}
                        />
                        <span
                          className={cn(
                            'truncate font-medium transition-[max-width,margin,opacity,transform] duration-200 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none',
                            isCollapsed
                              ? 'ml-0 max-w-0 -translate-x-1 opacity-0'
                              : 'ml-2 max-w-32 translate-x-0 opacity-100 delay-75'
                          )}
                        >
                          {item.title}
                        </span>
                        {!isCollapsed && isForbidden ? (
                          <LockKeyhole className="ml-auto size-3 shrink-0 text-muted-foreground/60" />
                        ) : null}
                        {!isCollapsed && isLoading ? (
                          <LoaderCircle className="ml-auto size-3 shrink-0 animate-spin text-muted-foreground/60" />
                        ) : null}
                        {!isCollapsed && hasAccessError ? (
                          <CircleAlert className="ml-auto size-3 shrink-0 text-muted-foreground/60" />
                        ) : null}
                        {!isCollapsed && needsSetup ? (
                          <span className="ml-auto size-1.5 shrink-0 rounded-full bg-warning" />
                        ) : null}
                      </>
                    );
                    const navClassName = cn(
                      'flex h-8 items-center rounded-md py-1.5 text-[13px]',
                      'transition-[width,padding,background-color,color,transform] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none',
                      !isBlocked && 'active:scale-[0.98]',
                      isCollapsed ? 'w-8 justify-center px-0' : 'w-full justify-start px-2',
                      isBlocked
                        ? 'cursor-not-allowed text-foreground/35'
                        : isActive
                          ? 'bg-muted/80 text-foreground'
                          : 'text-foreground/70 hover:bg-muted/70 hover:text-foreground'
                    );
                    const navLink = isBlocked ? (
                      <button
                        type="button"
                        className={navClassName}
                        aria-disabled="true"
                        title={`${item.title} · ${accessLabel}`}
                      >
                        {navContent}
                      </button>
                    ) : (
                      <Link
                        href={navigationTarget}
                        className={navClassName}
                        title={needsSetup ? `${item.title} · ${accessLabel}` : undefined}
                      >
                        {navContent}
                      </Link>
                    );

                    return isCollapsed ? (
                      <CollapsedNavTooltip
                        key={item.href}
                        label={
                          item.accessState === 'available'
                            ? item.title
                            : `${item.title} · ${accessLabel}`
                        }
                      >
                        {navLink}
                      </CollapsedNavTooltip>
                    ) : (
                      <React.Fragment key={item.href}>{navLink}</React.Fragment>
                    );
                  })}
                </div>
              )}
            </div>
          );
        })}

        {rootRouteItems.length > 0 ? (
          <div
            className={cn(
              'w-full mt-2 pt-2 border-t border-border',
              isCollapsed ? 'space-y-1' : 'space-y-1'
            )}
          >
            {rootRouteItems.map(item => {
              const Icon = item.icon;
              const isActive = isRootRouteItemActive(activePathname, item);

              const rootLink = (
                <Link
                  href={item.href}
                  target={item.target}
                  aria-current={isActive ? 'page' : undefined}
                  rel={item.target === '_blank' ? 'noreferrer' : undefined}
                  className={cn(
                    'flex items-center rounded-md py-1.5 text-[13px] transition-colors shrink-0 w-full',
                    isCollapsed ? 'justify-center px-0 w-8' : 'justify-start px-2',
                    'text-foreground/70 hover:bg-muted/70 hover:text-foreground',
                    isActive && 'bg-muted/80 text-foreground'
                  )}
                >
                  <Icon
                    size={16}
                    className={cn('shrink-0 text-foreground/65', isActive && 'text-foreground')}
                  />
                  <span
                    className={cn(
                      'truncate transition-all duration-300 opacity-100 font-normal',
                      isCollapsed && 'ml-0 opacity-0 w-0 hidden',
                      !isCollapsed && 'ml-2'
                    )}
                  >
                    {item.title}
                  </span>
                </Link>
              );

              return isCollapsed ? (
                <CollapsedNavTooltip key={item.key} label={item.title}>
                  {rootLink}
                </CollapsedNavTooltip>
              ) : (
                <React.Fragment key={item.key}>{rootLink}</React.Fragment>
              );
            })}
          </div>
        ) : null}
      </nav>
    </div>
  );

  if (hidden) {
    return null;
  }
  return (
    <aside className={cn('relative hidden shrink-0 md:block', layoutIsCollapsed ? 'w-12' : 'w-44')}>
      <div
        className={cn(
          'absolute inset-y-0 left-0 z-40 flex flex-col overflow-hidden border-r border-border/60 bg-background text-sidebar-foreground',
          'will-change-[width] transition-[width,box-shadow] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none',
          isCollapsed ? 'w-12' : 'w-44'
        )}
      >
        {sidebarContent}
        {!isTemporarilyCollapsed ? (
          <div className={cn('shrink-0 flex p-2 pt-1', isCollapsed && 'justify-center')}>
            <Button
              onClick={toggleCollapse}
              variant="ghost"
              size="xs"
              aria-label={isCollapsed ? t('expand') : t('collapse')}
              className={cn(
                'flex h-7 items-center rounded-md py-0 text-[13px] font-medium transition-colors gap-0',
                isCollapsed ? 'justify-center w-8 px-0' : 'justify-start w-full px-2',
                'text-foreground/70 hover:bg-muted/70 hover:text-foreground'
              )}
            >
              <ArrowRightToLine
                size={16}
                className={cn(
                  'shrink-0 transition-transform duration-300',
                  !isCollapsed && 'rotate-180'
                )}
              />
              <span
                className={cn(
                  'truncate transition-all duration-300 ml-2 opacity-100 font-normal',
                  isCollapsed && 'ml-0 opacity-0 w-0 hidden'
                )}
              >
                {isCollapsed ? t('expand') : t('collapse')}
              </span>
            </Button>
          </div>
        ) : null}
      </div>
    </aside>
  );
}

export function ConsoleMobileSidebar({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const t = useT('navigation');
  const datasetReturnTo = getDatasetReturnTo(searchParams.get('returnTo'));
  const activePathname = datasetReturnTo ? '/console/dataset' : pathname;
  const {
    permissions,
    organizationRole,
    workspaceRole,
    isLoading: isPermissionsLoading,
    error: permissionsError,
  } = useAccountPermissions();
  const contextStatus = useWorkspaceStore.use.contextStatus();
  const systemFeatures = useSystemFeatures();
  const externalIntegrationsAccessState: NavAccessState | undefined =
    systemFeatures.data !== undefined
      ? systemFeatures.data?.enable_external_integrations
        ? undefined
        : 'forbidden'
      : systemFeatures.isLoading
        ? 'loading'
        : systemFeatures.error
          ? 'error'
          : 'forbidden';
  const [openGroups, setOpenGroups] = React.useState<Record<string, boolean>>({
    work: true,
    resources: true,
    tools: true,
    management: true,
  });

  const navigationAccessContext = React.useMemo<ZGIConsoleNavigationAccessContext>(
    () => ({
      workspaceStatus: contextStatus,
      permissionsSettled: !isPermissionsLoading && permissionsError === null,
      organizationRole,
      workspaceRole,
      permissions,
    }),
    [
      contextStatus,
      isPermissionsLoading,
      organizationRole,
      permissions,
      permissionsError,
      workspaceRole,
    ]
  );

  const navGroups = React.useMemo<ResolvedNavGroup[]>(() => {
    const groups: NavGroup[] = [
      {
        key: 'work',
        title: t('work'),
        items: [
          {
            title: t('image'),
            href: '/console/work/image',
            icon: ImageIcon,
          },
          {
            title: t('music'),
            href: '/console/work/music',
            icon: Music2,
          },
          {
            title: t('video'),
            href: '/console/work/video',
            icon: Video,
          },
          {
            title: t('app'),
            href: '/console/work/app',
            icon: AppWindow,
          },
          {
            title: t('task'),
            href: '/console/work/task',
            icon: Clock3,
          },
        ],
      },
      {
        key: 'resources',
        title: t('resources'),
        items: [
          {
            title: t('agents'),
            href: '/console/agents',
            icon: Atom,
          },
          {
            title: t('workflowAgents'),
            href: '/console/workflows',
            icon: Workflow,
          },
          {
            title: t('prompts'),
            href: '/console/prompts',
            icon: BookText,
          },
          {
            title: t('files'),
            href: '/console/files',
            icon: FileText,
          },
          {
            title: t('datasets'),
            href: '/console/dataset',
            icon: BookOpen,
          },
          {
            title: t('dbs'),
            href: '/console/db',
            icon: Database,
          },
        ],
      },
      {
        key: 'tools',
        title: t('tools'),
        items: [
          {
            title: t('integrations'),
            href: '/console/integrations',
            icon: PlugZap,
            forcedAccessState: externalIntegrationsAccessState,
            blockedReason:
              externalIntegrationsAccessState === 'forbidden' ? t('featureDisabled') : undefined,
          },
          {
            title: t('skills'),
            href: '/console/skills',
            icon: Bot,
          },
        ],
      },
      {
        key: 'management',
        title: t('management'),
        items: [
          {
            title: t('workspaceManagement'),
            href: '/console/workspace',
            icon: Users,
          },
        ],
      },
    ];

    return resolveConsoleNavGroups(groups, navigationAccessContext, permissionsError !== null);
  }, [externalIntegrationsAccessState, navigationAccessContext, permissionsError, t]);

  const closeSidebar = () => onOpenChange(false);
  const toggleGroup = (key: string) => setOpenGroups(prev => ({ ...prev, [key]: !prev[key] }));

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="left" className="w-[86vw] max-w-80 p-0">
        <SheetTitle className="sr-only">{t('chat')}</SheetTitle>
        <div className="flex h-full flex-col overflow-hidden bg-background">
          <div className="border-b border-border/60 px-4 py-3">
            <WorkspaceSwitcher isCollapsed={false} />
          </div>

          <nav className="flex-1 space-y-3 overflow-y-auto px-3 py-3">
            <Link
              href="/console"
              onClick={closeSidebar}
              className={cn(
                'flex items-center gap-2 rounded-md px-2 py-2 text-[13px] transition-colors',
                pathname === '/console'
                  ? 'bg-muted/80 text-foreground'
                  : 'text-foreground/70 hover:bg-muted/70 hover:text-foreground'
              )}
            >
              <House
                size={16}
                className={cn(
                  'shrink-0 text-foreground/60',
                  pathname === '/console' && 'text-foreground'
                )}
              />
              <span className="truncate font-medium">{t('home')}</span>
            </Link>
            <Link
              href="/console/work/chat"
              onClick={closeSidebar}
              className={cn(
                'flex items-center gap-2 rounded-md px-2 py-2 text-[13px] transition-colors',
                pathname.startsWith('/console/work/chat')
                  ? 'bg-muted/80 text-foreground'
                  : 'text-foreground/70 hover:bg-muted/70 hover:text-foreground'
              )}
            >
              <MessageSquare
                size={16}
                className={cn(
                  'shrink-0 text-foreground/60',
                  pathname.startsWith('/console/work/chat') && 'text-foreground'
                )}
              />
              <span className="truncate font-medium">{t('chat')}</span>
            </Link>

            {navGroups.map(group => {
              const isExpanded = openGroups[group.key] ?? true;
              return (
                <div key={group.key}>
                  <button
                    type="button"
                    onClick={() => toggleGroup(group.key)}
                    className="flex w-full items-center justify-between rounded-md px-2 py-1 text-[12px] font-medium text-foreground/55 hover:bg-muted/60 hover:text-foreground/80"
                  >
                    <span className="truncate">{group.title}</span>
                    <ChevronDown
                      className={cn(
                        'h-3.5 w-3.5 shrink-0 text-foreground/45 transition-transform duration-200',
                        !isExpanded && '-rotate-90'
                      )}
                    />
                  </button>

                  {isExpanded ? (
                    <div className="mt-1 space-y-0.5">
                      {group.items.map(item => {
                        const Icon = item.icon;
                        const isForbidden = item.accessState === 'forbidden';
                        const isLoading = item.accessState === 'loading';
                        const hasAccessError = item.accessState === 'error';
                        const isBlocked = isForbidden || isLoading || hasAccessError;
                        const needsSetup = item.accessState === 'setup_required';
                        const navigationTarget = getZGIConsoleNavigationTarget(
                          item.href,
                          item.accessState
                        );
                        const isActive = !isBlocked && isItemActive(activePathname, item.href);
                        const itemContent = (
                          <>
                            <Icon
                              size={16}
                              className={cn(
                                'shrink-0 text-foreground/60',
                                isActive && 'text-foreground',
                                isBlocked && 'text-foreground/30'
                              )}
                            />
                            <span className="truncate font-medium">{item.title}</span>
                            {isForbidden ? (
                              <span className="ml-auto flex items-center gap-1 text-[10px] text-muted-foreground">
                                <LockKeyhole className="size-3" />
                                {item.blockedReason || t('permissionRequired')}
                              </span>
                            ) : null}
                            {isLoading ? (
                              <span className="ml-auto flex items-center gap-1 text-[10px] text-muted-foreground">
                                <LoaderCircle className="size-3 animate-spin" />
                                {t('checkingAccess')}
                              </span>
                            ) : null}
                            {hasAccessError ? (
                              <span className="ml-auto flex items-center gap-1 text-[10px] text-muted-foreground">
                                <CircleAlert className="size-3" />
                                {t('accessCheckFailed')}
                              </span>
                            ) : null}
                            {needsSetup ? (
                              <span className="ml-auto text-[10px] text-warning">
                                {t('setupRequired')}
                              </span>
                            ) : null}
                          </>
                        );
                        const itemClassName = cn(
                          'flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-[13px] transition-colors',
                          isBlocked
                            ? 'cursor-not-allowed text-foreground/35'
                            : isActive
                              ? 'bg-muted/80 text-foreground'
                              : 'text-foreground/70 hover:bg-muted/70 hover:text-foreground'
                        );

                        return isBlocked ? (
                          <button
                            key={item.href}
                            type="button"
                            className={itemClassName}
                            aria-disabled="true"
                          >
                            {itemContent}
                          </button>
                        ) : (
                          <Link
                            key={item.href}
                            href={navigationTarget}
                            onClick={closeSidebar}
                            className={itemClassName}
                            title={needsSetup ? `${item.title} · ${t('setupRequired')}` : undefined}
                          >
                            {itemContent}
                          </Link>
                        );
                      })}
                    </div>
                  ) : null}
                </div>
              );
            })}
            <div className="space-y-0.5 border-t border-border pt-2">
              {ROOT_ROUTES.map(item => {
                const Icon = item.icon;
                const title = t(item.titleKey);
                const isActive = isRootRouteItemActive(activePathname, { ...item, title });
                return (
                  <Link
                    key={item.key}
                    href={item.href}
                    onClick={closeSidebar}
                    aria-current={isActive ? 'page' : undefined}
                    className={cn(
                      'flex w-full items-center gap-2 rounded-md px-2 py-2 text-[13px] transition-colors',
                      isActive
                        ? 'bg-muted/80 text-foreground'
                        : 'text-foreground/70 hover:bg-muted/70 hover:text-foreground'
                    )}
                  >
                    <Icon size={16} className="shrink-0" />
                    <span className="truncate font-medium">{title}</span>
                  </Link>
                );
              })}
            </div>
          </nav>
        </div>
      </SheetContent>
    </Sheet>
  );
}
