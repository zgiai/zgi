'use client';

import * as React from 'react';
import { usePathname } from 'next/navigation';
import { useT } from '@/i18n';
import Link from 'next/link';
import { ENABLE_RUNTIME_WORKSPACE } from '@/lib/config';
import { cn } from '@/lib/utils';
import {
  KeyRound,
  LayoutDashboard,
  Loader2,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  ShieldAlert,
  Users,
} from 'lucide-react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  useCurrentWorkspace,
  useWorkspaceContextStatus,
  useHasHydrated,
} from '@/store/workspace-store';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import { WorkspaceRequiredState } from '@/components/common/workspace-required-state';

interface WorkspaceNavItem {
  id: string;
  label: string;
  desc: string;
  icon: React.ComponentType<{ className?: string }>;
  href: string;
  visible?: boolean;
  children?: Array<{
    id: string;
    label: string;
    href: string;
  }>;
}

function WorkspaceAccessDeniedState() {
  const t = useT();

  return (
    <div className="flex h-full w-full flex-col items-center justify-center p-4 text-center">
      <ShieldAlert className="mb-4 h-12 w-12 text-muted-foreground" />
      <h2 className="mb-2 text-xl font-semibold">{t('common.accessDenied')}</h2>
      <p className="max-w-md text-muted-foreground">{t('common.unauthorizedDescription')}</p>
    </div>
  );
}

const navCollapsedKey = 'zgi.workspace.nav.collapsed';

// Whether the workspace navigation is folded to icons, remembered per browser.
function useWorkspaceNavCollapsed() {
  const [collapsed, setCollapsed] = React.useState(false);
  React.useEffect(() => {
    try {
      setCollapsed(window.localStorage.getItem(navCollapsedKey) === '1');
    } catch {
      // Storage may be unavailable; the navigation stays expanded.
    }
  }, []);
  const toggle = React.useCallback(() => {
    setCollapsed(value => {
      try {
        window.localStorage.setItem(navCollapsedKey, value ? '0' : '1');
      } catch {
        // Storage may be unavailable; the choice lasts for this page only.
      }
      return !value;
    });
  }, []);
  return { collapsed, toggle };
}

function isWorkspaceNavItemActive(pathname: string, href: string) {
  if (href === '/console/workspace') {
    return pathname === href;
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}

export default function WorkspaceLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const t = useT();
  const tWorkspace = useT('workspace');
  const currentWorkspace = useCurrentWorkspace();
  const contextStatus = useWorkspaceContextStatus();
  const hasHydrated = useHasHydrated();
  const {
    hasWorkspaceAccess,
    isWorkspaceManager,
    isLoading: isLoadingPermissions,
    isFetching: isFetchingPermissions,
  } = useAccountPermissions();
  const {
    isLoading: isCapabilitiesLoading,
    isFetching: isCapabilitiesFetching,
    canUseWorkspaceScope,
    isWorkspaceRequired,
  } = useAccountCapabilities();
  const isAccessLoading =
    isLoadingPermissions ||
    isFetchingPermissions ||
    isCapabilitiesLoading ||
    isCapabilitiesFetching;
  const hasWorkspaceContext = contextStatus === 'ready' && !!currentWorkspace;
  const canViewWorkspace = hasWorkspaceAccess();
  const canManageWorkspace = isWorkspaceManager();
  const { collapsed, toggle: toggleCollapsed } = useWorkspaceNavCollapsed();
  // Shown only while the navigation is wide: by choice and by room.
  const wide = collapsed ? 'hidden' : 'hidden @5xl/console:block';
  const wideFlex = collapsed ? 'hidden' : 'hidden @5xl/console:flex';
  const narrowFlex = collapsed ? 'flex' : 'flex @5xl/console:hidden';

  const workspaceNavItems: WorkspaceNavItem[] = [
    {
      id: 'overview',
      label: t('workspace.navigation.overview'),
      desc: t('workspace.navigation.overviewDesc'),
      icon: LayoutDashboard,
      href: '/console/workspace',
    },
    {
      id: 'members',
      label: t('navigation.member'),
      desc: t('navigation.memberDesc'),
      icon: Users,
      href: '/console/workspace/members',
      visible: canManageWorkspace,
    },
    {
      id: 'runtime',
      label: t('workspace.navigation.runtime'),
      desc: t('workspace.navigation.runtimeDesc'),
      icon: KeyRound,
      href: '/console/workspace/runtime-workspaces',
      visible: ENABLE_RUNTIME_WORKSPACE && canManageWorkspace,
      children: [
        { id: 'runtime-overview', label: t('workspace.runtime.tabs.overview'), href: '/console/workspace/runtime-workspaces' },
        { id: 'runtime-agents', label: t('workspace.runtime.tabs.apps'), href: '/console/workspace/runtime-workspaces/agents' },
        { id: 'runtime-end-users', label: t('workspace.runtime.tabs.endUsers'), href: '/console/workspace/runtime-workspaces/end-users' },
        { id: 'runtime-quota', label: t('workspace.runtime.tabs.quota'), href: '/console/workspace/runtime-workspaces/quota' },
        { id: 'runtime-integration', label: t('workspace.runtime.tabs.integration'), href: '/console/workspace/runtime-workspaces/integration' },
        { id: 'runtime-docs', label: t('workspace.runtime.tabs.docs'), href: '/console/workspace/runtime-workspaces/docs' },
      ],
    },
    {
      id: 'settings',
      label: t('navigation.settings'),
      desc: t('navigation.settingsDesc'),
      icon: Settings,
      href: '/console/workspace/settings',
    },
  ];

  // Loading state
  if (!hasHydrated || isAccessLoading) {
    return (
      <div className="flex h-full items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (isWorkspaceRequired || !hasWorkspaceContext) {
    return <WorkspaceRequiredState />;
  }

  if (!canUseWorkspaceScope || !canViewWorkspace) {
    return <WorkspaceAccessDeniedState />;
  }

  return (
    <div className="flex h-full bg-background">
      {/* Workspace Sidebar */}
      <div
        className={cn(
          'shrink-0 border-r border-border/70 bg-muted/20 transition-[width]',
          collapsed ? 'w-14' : 'w-14 @5xl/console:w-60'
        )}
      >
        <div className="flex h-full flex-col">
          {/* Header */}
          <div className={cn('border-b border-border/60 px-4 py-4', wide)}>
            <div className="min-w-0">
              <h1 className="text-sm font-semibold text-foreground">{tWorkspace('pageTitle')}</h1>
              <p className="mt-1 truncate text-xs text-muted-foreground">{currentWorkspace.name}</p>
            </div>
          </div>

          {/* Workspace Navigation */}
          <div className={cn('flex-1 overflow-y-auto px-2 py-3', !collapsed && '@5xl/console:px-3')}>
            <div className="space-y-0.5">
              {workspaceNavItems
                .filter(item => item.visible !== false)
                .map(item => {
                  const Icon = item.icon;
                  const isActive = isWorkspaceNavItemActive(pathname, item.href);
                  const iconLink = (
                    <Link
                      href={item.href}
                      title={item.label}
                      aria-label={item.label}
                      className={cn(
                        'relative items-center justify-center rounded-md px-2 py-2 transition-colors',
                        narrowFlex,
                        isActive
                          ? 'bg-primary/[0.08] text-primary'
                          : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'
                      )}
                    >
                      {isActive && <div className="absolute bottom-2 left-0 top-2 w-0.5 rounded-r-full bg-primary" />}
                      <Icon className="h-4 w-4" />
                    </Link>
                  );

                  return (
                    <div key={item.id}>
                      <Link
                        href={item.href}
                        title={item.label}
                        className={cn(
                          'group relative items-start justify-start gap-2 rounded-md px-2.5 py-2 text-sm transition-colors',
                          wideFlex,
                          isActive
                            ? item.children
                              ? 'bg-primary/[0.06] text-foreground'
                              : 'bg-primary/[0.08] text-primary'
                            : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'
                        )}
                      >
                        {isActive && <div className="absolute bottom-2 left-0 top-2 w-0.5 rounded-r-full bg-primary" />}
                        <Icon className={cn('mt-0.5 h-4 w-4 flex-shrink-0 transition-colors', isActive ? 'text-primary' : 'text-muted-foreground')} />
                        <div className="min-w-0 flex-1">
                          <span className={cn('truncate font-medium', isActive ? 'text-foreground' : 'text-muted-foreground')}>{item.label}</span>
                          {!(isActive && item.children) ? <p className={cn('mt-0.5 line-clamp-1 text-[11px] leading-4', isActive ? 'text-muted-foreground' : 'text-muted-foreground/80')}>{item.desc}</p> : null}
                        </div>
                      </Link>
                      {item.children ? (
                        // Icon-only, a section's pages open from a menu on its icon.
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <button
                              type="button"
                              title={item.label}
                              aria-label={item.label}
                              className={cn(
                                'relative w-full items-center justify-center rounded-md px-2 py-2 transition-colors',
                                narrowFlex,
                                isActive
                                  ? 'bg-primary/[0.08] text-primary'
                                  : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'
                              )}
                            >
                              {isActive && <div className="absolute bottom-2 left-0 top-2 w-0.5 rounded-r-full bg-primary" />}
                              <Icon className="h-4 w-4" />
                            </button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent side="right" align="start" className="min-w-48">
                            <DropdownMenuLabel>{item.label}</DropdownMenuLabel>
                            <DropdownMenuSeparator />
                            {item.children.map(child => (
                              <DropdownMenuItem key={child.id} asChild>
                                <Link
                                  href={child.href}
                                  className={cn(pathname === child.href && 'font-medium text-primary')}
                                >
                                  {child.label}
                                </Link>
                              </DropdownMenuItem>
                            ))}
                          </DropdownMenuContent>
                        </DropdownMenu>
                      ) : (
                        iconLink
                      )}
                      {isActive && item.children ? (
                        <div className={cn('mb-2 ml-4 mt-1 space-y-0.5 border-l border-primary/20 pl-3', wide)}>
                          {item.children.map(child => {
                            const childActive = pathname === child.href;
                            return (
                              <Link
                                key={child.id}
                                href={child.href}
                                className={cn(
                                  'relative block rounded-md px-3 py-2 text-xs transition-colors',
                                  childActive ? 'bg-primary/[0.08] font-medium text-primary' : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'
                                )}
                              >
                                {childActive ? <span className="absolute -left-[13px] top-1/2 h-4 w-0.5 -translate-y-1/2 rounded-full bg-primary" /> : null}
                                <span className="truncate">{child.label}</span>
                              </Link>
                            );
                          })}
                        </div>
                      ) : null}
                    </div>
                  );
                })}
            </div>
          </div>

          {/* Collapse: only offered where the navigation has room to expand */}
          <div className="hidden border-t border-border/60 p-2 @5xl/console:block">
            <button
              type="button"
              onClick={toggleCollapsed}
              title={collapsed ? tWorkspace('navigation.expand') : tWorkspace('navigation.collapse')}
              aria-label={collapsed ? tWorkspace('navigation.expand') : tWorkspace('navigation.collapse')}
              className={cn(
                'flex w-full items-center gap-2 rounded-md px-2 py-2 text-xs text-muted-foreground transition-colors hover:bg-background/70 hover:text-foreground',
                collapsed ? 'justify-center' : 'justify-start px-2.5'
              )}
            >
              {collapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
              {collapsed ? null : <span>{tWorkspace('navigation.collapse')}</span>}
            </button>
          </div>
        </div>
      </div>

      {/* Workspace Content */}
      <div className="min-w-0 flex-1 bg-background">
        <div className="h-full overflow-y-auto">{children}</div>
      </div>
    </div>
  );
}
