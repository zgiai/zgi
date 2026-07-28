'use client';

import * as React from 'react';
import Link from 'next/link';
import {
  ArrowRightToLine,
  Atom,
  Bot,
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
import { WorkspaceSwitcher } from './team-switcher';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useWorkspaceStore } from '@/store/workspace-store';
import type { PermissionCode } from '@/constants/permissions';
import { withBasePathIfInternal } from '@/lib/config';
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { useWorkflowDebugFocusMode } from '@/components/workflow/hooks/use-debug-focus-mode';
import { usePersistentSidebarCollapse } from '@/hooks/use-persistent-sidebar-collapse';
import { resolveZGIConsoleNavigationRoute } from '@/routes/console-navigation';

interface NavItem {
  title: string;
  href: string;
  icon: React.ElementType;
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

type HasAnyPermission = (permissions: readonly PermissionCode[]) => boolean;

function shouldShowConsoleNavItem(
  item: NavItem,
  isWorkspaceRequired: boolean,
  hasAnyPermission: HasAnyPermission
) {
  const navigationRoute = resolveZGIConsoleNavigationRoute(item.href);
  if (!navigationRoute) return false;
  if (isWorkspaceRequired) return navigationRoute.scope === 'organization';
  if (navigationRoute.scope === 'organization' || navigationRoute.permissions.length === 0) {
    return true;
  }
  return hasAnyPermission(navigationRoute.permissions);
}

function filterConsoleNavGroups(
  groups: NavGroup[],
  isWorkspaceRequired: boolean,
  hasAnyPermission: HasAnyPermission
) {
  return groups
    .map(group => {
      const items = group.items.filter(item =>
        shouldShowConsoleNavItem(item, isWorkspaceRequired, hasAnyPermission)
      );

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
  const { hasAnyPermission } = useAccountPermissions();
  const contextStatus = useWorkspaceStore.use.contextStatus();
  const isWorkspaceRequired = contextStatus === 'workspace_required';
  const isDebugFocusMode = useWorkflowDebugFocusMode();

  // Collapsed state persisted via ui-local helpers
  const [persistedIsCollapsed, setIsCollapsed] = usePersistentSidebarCollapse(
    'console',
    autoCollapse,
    isDebugFocusMode || temporarilyCollapsed
  );
  const isTemporarilyCollapsed = isDebugFocusMode || temporarilyCollapsed;
  const [isHoverExpanded, setIsHoverExpanded] = React.useState(false);
  const hoverTimerRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);
  const layoutIsCollapsed = isTemporarilyCollapsed || persistedIsCollapsed;
  const isCollapsed = layoutIsCollapsed && !isHoverExpanded;
  const wasAutoCollapseActiveRef = React.useRef(false);

  React.useEffect(() => {
    if (autoCollapse && !wasAutoCollapseActiveRef.current && !persistedIsCollapsed) {
      setIsCollapsed(true);
    }
    wasAutoCollapseActiveRef.current = autoCollapse;
  }, [autoCollapse, persistedIsCollapsed, setIsCollapsed]);

  const toggleCollapse = () => setIsCollapsed(prev => !prev);

  React.useEffect(() => {
    if (!layoutIsCollapsed) {
      if (hoverTimerRef.current) {
        clearTimeout(hoverTimerRef.current);
        hoverTimerRef.current = null;
      }
      setIsHoverExpanded(false);
    }
  }, [layoutIsCollapsed]);

  React.useEffect(
    () => () => {
      if (hoverTimerRef.current) clearTimeout(hoverTimerRef.current);
    },
    []
  );

  const scheduleHoverExpanded = React.useCallback((expanded: boolean) => {
    if (hoverTimerRef.current) clearTimeout(hoverTimerRef.current);
    hoverTimerRef.current = setTimeout(
      () => {
        setIsHoverExpanded(expanded);
        hoverTimerRef.current = null;
      },
      expanded ? 60 : 220
    );
  }, []);

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
            title: t('skills'),
            href: '/console/skills',
            icon: Bot,
          },
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
        ],
      },
    ],
    [t]
  );

  // Filter groups and items
  const navGroups = React.useMemo(() => {
    return filterConsoleNavGroups(allNavGroups, isWorkspaceRequired, hasAnyPermission);
  }, [isWorkspaceRequired, hasAnyPermission, allNavGroups]);

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
          <CollapsedNavTooltip label={t('chat')}>{homeNavLink}</CollapsedNavTooltip>
        ) : (
          homeNavLink
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
                    const isActive = isItemActive(activePathname, item.href);
                    const navLink = (
                      <Link
                        key={item.href}
                        href={item.href}
                        className={cn(
                          'flex h-8 items-center rounded-md py-1.5 text-[13px]',
                          'transition-[width,padding,background-color,color,transform] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none',
                          'active:scale-[0.98]',
                          isCollapsed ? 'w-8 justify-center px-0' : 'w-full justify-start px-2',
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
                      </Link>
                    );

                    return isCollapsed ? (
                      <CollapsedNavTooltip key={item.href} label={item.title}>
                        {navLink}
                      </CollapsedNavTooltip>
                    ) : (
                      navLink
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
      onPointerEnter={event => {
        if (event.pointerType !== 'mouse') return;
        if (layoutIsCollapsed) {
          scheduleHoverExpanded(true);
        }
      }}
      onPointerLeave={event => {
        if (event.pointerType !== 'mouse') return;
        scheduleHoverExpanded(false);
      }}
      className={cn('relative hidden shrink-0 md:block', layoutIsCollapsed ? 'w-12' : 'w-44')}
    >
      <div
        className={cn(
          'absolute inset-y-0 left-0 z-40 flex flex-col overflow-hidden border-r border-border/60 bg-background text-sidebar-foreground',
          'will-change-[width] transition-[width,box-shadow] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none',
          isCollapsed ? 'w-12' : 'w-44',
          layoutIsCollapsed && !isCollapsed && 'shadow-[12px_0_28px_-18px_rgba(15,23,42,0.45)]'
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
  const { hasAnyPermission } = useAccountPermissions();
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
          },
          {
            title: t('workflowAgents'),
            href: '/console/workflows',
            icon: Workflow,
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
            title: t('skills'),
            href: '/console/skills',
            icon: Bot,
          },
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
        ],
      },
    ];

    return filterConsoleNavGroups(groups, isWorkspaceRequired, hasAnyPermission);
  }, [hasAnyPermission, isWorkspaceRequired, t]);

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
