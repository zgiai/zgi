'use client';

import * as React from 'react';
import Link from 'next/link';
import {
  Settings,
  ArrowRightToLine,
  Home,
  Atom,
  BookText,
  FileText,
  FileSearch,
  Database,
  BookOpen,
  Users,
  MessageSquare,
  Image as ImageIcon,
  AppWindow,
  Clock3,
  ChevronDown,
  Workflow,
} from 'lucide-react';
import { usePathname, useSearchParams } from 'next/navigation';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { WorkspaceSwitcher } from './team-switcher';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useWorkspaceStore } from '@/store/workspace-store';
import {
  AGENT_VISIBLE_PERMISSION_CODES,
  DATABASE_VISIBLE_PERMISSION_CODES,
  FILE_VISIBLE_PERMISSION_CODES,
  KNOWLEDGE_BASE_VISIBLE_PERMISSION_CODES,
  WORKFLOW_VISIBLE_PERMISSION_CODES,
  type PermissionCode,
} from '@/constants/permissions';
import { ENABLE_THEME_SWITCH, withBasePathIfInternal } from '@/lib/config';
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { useWorkflowDebugFocusMode } from '@/components/workflow/hooks/use-debug-focus-mode';
import { usePersistentSidebarCollapse } from '@/hooks/use-persistent-sidebar-collapse';
import { getConsoleRouteAccess } from '@/routes/access';

interface NavItem {
  title: string;
  href: string;
  icon: React.ElementType;
  /** Required permission to show this nav item (when workspace selected) */
  permission?: PermissionCode;
  /** Any required permission to show this nav item (when workspace selected) */
  permissions?: readonly PermissionCode[];
}

interface NavGroup {
  key: string;
  title: string;
  items: NavItem[];
}

interface RootRouteItem {
  key: string;
  title: string;
  href: string;
  icon: React.ElementType;
  target?: '_self' | '_blank';
  activeMatchPaths?: string[];
}

const STORAGE_KEY = 'zgi:console:sidebar:groups';

function CollapsedNavTooltip({ label, children }: { label: string; children: React.ReactElement }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent side="right" sideOffset={8} className="px-2.5 py-1.5 text-xs">
        {label}
      </TooltipContent>
    </Tooltip>
  );
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

type HasPermission = (permission: PermissionCode) => boolean;
type HasAnyPermission = (permissions: readonly PermissionCode[]) => boolean;

function shouldShowConsoleNavItem(
  item: NavItem,
  isWorkspaceRequired: boolean,
  hasPermission: HasPermission,
  hasAnyPermission: HasAnyPermission
) {
  const routeAccess = getConsoleRouteAccess(item.href);

  if (isWorkspaceRequired) {
    return routeAccess.scope === 'organization';
  }

  if (routeAccess.scope === 'organization') {
    return true;
  }

  if (item.permissions?.length) {
    return hasAnyPermission(item.permissions);
  }

  if (!item.permission) {
    return true;
  }

  return hasPermission(item.permission);
}

function filterConsoleNavGroups(
  groups: NavGroup[],
  isWorkspaceRequired: boolean,
  hasPermission: HasPermission,
  hasAnyPermission: HasAnyPermission
) {
  return groups
    .map(group => {
      let items = group.items;

      if (!ENABLE_THEME_SWITCH) {
        items = items.filter(item => item.href !== '/console/settings');
      }

      items = items.filter(item =>
        shouldShowConsoleNavItem(item, isWorkspaceRequired, hasPermission, hasAnyPermission)
      );

      return { ...group, items };
    })
    .filter(group => group.items.length > 0);
}

export function ConsoleSidebar({
  hidden,
  temporarilyCollapsed = false,
}: {
  hidden?: boolean;
  temporarilyCollapsed?: boolean;
}) {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const t = useT('navigation');
  const datasetReturnTo = getDatasetReturnTo(searchParams.get('returnTo'));
  const activePathname = datasetReturnTo ? '/console/dataset' : pathname;

  // Permission checking
  const { hasPermission, hasAnyPermission } = useAccountPermissions();
  const contextStatus = useWorkspaceStore.use.contextStatus();
  const isWorkspaceRequired = contextStatus === 'workspace_required';
  const isDebugFocusMode = useWorkflowDebugFocusMode();

  // Collapsed state persisted via ui-local helpers
  const [persistedIsCollapsed, setIsCollapsed] = usePersistentSidebarCollapse(
    'console',
    false,
    isDebugFocusMode || temporarilyCollapsed
  );
  const isTemporarilyCollapsed = isDebugFocusMode || temporarilyCollapsed;
  const isCollapsed = isTemporarilyCollapsed || persistedIsCollapsed;

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
            title: t('chat'),
            href: '/console/work/chat',
            icon: MessageSquare,
          },
          {
            title: t('image'),
            href: '/console/work/image',
            icon: ImageIcon,
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
            permissions: AGENT_VISIBLE_PERMISSION_CODES,
          },
          {
            title: t('workflowAgents'),
            href: '/console/workflows',
            icon: Workflow,
            permissions: WORKFLOW_VISIBLE_PERMISSION_CODES,
          },
          {
            title: t('datasets'),
            href: '/console/dataset',
            icon: BookOpen,
            permissions: KNOWLEDGE_BASE_VISIBLE_PERMISSION_CODES,
          },
          {
            title: t('files'),
            href: '/console/files',
            icon: FileText,
            permissions: FILE_VISIBLE_PERMISSION_CODES,
          },
          {
            title: t('dbs'),
            href: '/console/db',
            icon: Database,
            permissions: DATABASE_VISIBLE_PERMISSION_CODES,
          },
        ],
      },
      {
        key: 'tools',
        title: t('tools'),
        items: [
          {
            title: t('prompts'),
            href: '/console/prompts',
            icon: BookText,
          },
          {
            title: t('fileRecognition'),
            href: '/console/developer/content-parse',
            icon: FileSearch,
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
          { title: t('systemSettings'), href: '/console/settings', icon: Settings },
        ],
      },
    ],
    [t]
  );

  // Filter groups and items
  const navGroups = React.useMemo(() => {
    return filterConsoleNavGroups(
      allNavGroups,
      isWorkspaceRequired,
      hasPermission,
      hasAnyPermission
    );
  }, [isWorkspaceRequired, hasPermission, hasAnyPermission, allNavGroups]);

  const rootRouteItems = React.useMemo(
    (): RootRouteItem[] => [
      // Add branch-specific root route items here when needed.
      // Example:
      // {
      //   key: 'model-square',
      //   title: 'Model Square',
      //   href: 'https://example.com/modelsquare',
      //   icon: LayoutGrid,
      //   target: '_blank',
      // },
    ],
    []
  );

  const homeNavLink = (
    <Link
      href="/console"
      className={cn(
        'flex items-center gap-2 rounded-md py-1.5 text-[13px] transition-colors shrink-0 w-full',
        isCollapsed ? 'justify-center px-0 w-8' : 'justify-start px-2',
        'text-foreground/70 hover:bg-muted/70 hover:text-foreground',
        pathname === '/console' && 'bg-muted/80 text-foreground'
      )}
    >
      <Home
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

  const sidebarContent = (
    <div className="flex flex-col flex-1 h-full overflow-hidden">
      {/* Workspace Switcher */}
      <div className="shrink-0 px-2 py-2">
        <WorkspaceSwitcher isCollapsed={isCollapsed} />
      </div>
      {/* Navigation Items */}
      <nav
        className={cn(
          'flex flex-col gap-1 px-2 py-1 flex-1 overflow-y-auto overflow-x-hidden scrollbar-none transition-all duration-300',
          isCollapsed ? 'items-center' : 'items-start'
        )}
      >
        {isCollapsed ? (
          <CollapsedNavTooltip label={t('home')}>{homeNavLink}</CollapsedNavTooltip>
        ) : (
          homeNavLink
        )}

        {navGroups.map(group => {
          // If collapsed, we flatten the structure visually (hide headers, show items)
          if (isCollapsed) {
            return group.items.map(item => {
              const Icon = item.icon;
              const isActive = isItemActive(activePathname, item.href);
              return (
                <CollapsedNavTooltip key={item.href} label={item.title}>
                  <Link
                    href={item.href}
                    className={cn(
                      'flex w-8 items-center justify-center rounded-md py-1.5 text-[13px] font-medium transition-colors',
                      'text-foreground/70 hover:bg-muted/70 hover:text-foreground',
                      isActive && 'bg-muted/80 text-foreground'
                    )}
                  >
                    <Icon
                      size={16}
                      className={cn('shrink-0 text-foreground/65', isActive && 'text-foreground')}
                    />
                  </Link>
                </CollapsedNavTooltip>
              );
            });
          }

          const isExpanded = openGroups[group.key] ?? true;

          return (
            <div key={group.key} className="w-full pt-2">
              <button
                type="button"
                onClick={() => toggleGroup(group.key)}
                className={cn(
                  'w-full flex items-center justify-between rounded-md px-2 py-1 text-[12px]',
                  'font-medium text-foreground/55 hover:bg-muted/60 hover:text-foreground/80'
                )}
              >
                <span className="truncate">{group.title}</span>
                <ChevronDown
                  className={cn(
                    'h-3.5 w-3.5 shrink-0 text-foreground/45 transition-transform duration-200',
                    !isExpanded && '-rotate-90'
                  )}
                />
              </button>

              {isExpanded && (
                <div className="mt-1 space-y-0.5">
                  {group.items.map(item => {
                    const Icon = item.icon;
                    const isActive = isItemActive(activePathname, item.href);
                    return (
                      <Link
                        key={item.href}
                        href={item.href}
                        className={cn(
                          'flex items-center gap-2 rounded-md px-2 py-1.5 text-[13px] transition-colors',
                          isActive
                            ? 'bg-muted/80 text-foreground'
                            : 'text-foreground/70 hover:bg-muted/70 hover:text-foreground'
                        )}
                      >
                        <Icon
                          size={16}
                          className={cn(
                            'shrink-0 text-foreground/60',
                            isActive && 'text-foreground'
                          )}
                        />
                        <span className="truncate font-medium">{item.title}</span>
                      </Link>
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
    <aside
      className={cn(
        'hidden md:flex md:flex-col shrink-0 border-r border-border/60 bg-background text-sidebar-foreground transition-[width] duration-300 ease-in-out',
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
  const { hasPermission, hasAnyPermission } = useAccountPermissions();
  const contextStatus = useWorkspaceStore.use.contextStatus();
  const isWorkspaceRequired = contextStatus === 'workspace_required';
  const [openGroups, setOpenGroups] = React.useState<Record<string, boolean>>({
    work: true,
    resources: true,
    tools: true,
    management: true,
  });

  const navGroups = React.useMemo<NavGroup[]>(() => {
    const groups: NavGroup[] = [
      {
        key: 'work',
        title: t('work'),
        items: [
          {
            title: t('chat'),
            href: '/console/work/chat',
            icon: MessageSquare,
          },
          {
            title: t('image'),
            href: '/console/work/image',
            icon: ImageIcon,
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
            permissions: AGENT_VISIBLE_PERMISSION_CODES,
          },
          {
            title: t('workflowAgents'),
            href: '/console/workflows',
            icon: Workflow,
            permissions: WORKFLOW_VISIBLE_PERMISSION_CODES,
          },
          {
            title: t('datasets'),
            href: '/console/dataset',
            icon: BookOpen,
            permissions: KNOWLEDGE_BASE_VISIBLE_PERMISSION_CODES,
          },
          {
            title: t('files'),
            href: '/console/files',
            icon: FileText,
            permissions: FILE_VISIBLE_PERMISSION_CODES,
          },
          {
            title: t('dbs'),
            href: '/console/db',
            icon: Database,
            permissions: DATABASE_VISIBLE_PERMISSION_CODES,
          },
        ],
      },
      {
        key: 'tools',
        title: t('tools'),
        items: [
          {
            title: t('prompts'),
            href: '/console/prompts',
            icon: BookText,
          },
          {
            title: t('fileRecognition'),
            href: '/console/developer/content-parse',
            icon: FileSearch,
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
          { title: t('systemSettings'), href: '/console/settings', icon: Settings },
        ],
      },
    ];

    return filterConsoleNavGroups(groups, isWorkspaceRequired, hasPermission, hasAnyPermission);
  }, [hasPermission, hasAnyPermission, isWorkspaceRequired, t]);

  const closeSidebar = () => onOpenChange(false);
  const toggleGroup = (key: string) => setOpenGroups(prev => ({ ...prev, [key]: !prev[key] }));

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="left" className="w-[86vw] max-w-80 p-0">
        <SheetTitle className="sr-only">{t('home')}</SheetTitle>
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
              <Home
                size={16}
                className={cn(
                  'shrink-0 text-foreground/60',
                  pathname === '/console' && 'text-foreground'
                )}
              />
              <span className="truncate font-medium">{t('home')}</span>
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
                        const isActive = isItemActive(activePathname, item.href);
                        return (
                          <Link
                            key={item.href}
                            href={item.href}
                            onClick={closeSidebar}
                            className={cn(
                              'flex items-center gap-2 rounded-md px-2 py-2 text-[13px] transition-colors',
                              isActive
                                ? 'bg-muted/80 text-foreground'
                                : 'text-foreground/70 hover:bg-muted/70 hover:text-foreground'
                            )}
                          >
                            <Icon
                              size={16}
                              className={cn(
                                'shrink-0 text-foreground/60',
                                isActive && 'text-foreground'
                              )}
                            />
                            <span className="truncate font-medium">{item.title}</span>
                          </Link>
                        );
                      })}
                    </div>
                  ) : null}
                </div>
              );
            })}
          </nav>
        </div>
      </SheetContent>
    </Sheet>
  );
}
