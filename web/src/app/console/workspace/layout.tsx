'use client';

import { usePathname } from 'next/navigation';
import { useT } from '@/i18n';
import Link from 'next/link';
import { cn } from '@/lib/utils';
import { LayoutDashboard, Loader2, Settings, ShieldAlert, Users } from 'lucide-react';
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
      <div className="w-60 shrink-0 border-r border-border/70 bg-muted/20">
        <div className="flex h-full flex-col">
          {/* Header */}
          <div className="border-b border-border/60 px-4 py-4">
            <div className="min-w-0">
              <h1 className="text-sm font-semibold text-foreground">{tWorkspace('pageTitle')}</h1>
              <p className="mt-1 truncate text-xs text-muted-foreground">{currentWorkspace.name}</p>
            </div>
          </div>

          {/* Workspace Navigation */}
          <div className="flex-1 overflow-y-auto px-3 py-3">
            <div className="space-y-0.5">
              {workspaceNavItems.map(item => {
                const Icon = item.icon;
                const isActive = isWorkspaceNavItemActive(pathname, item.href);

                return (
                  <Link
                    key={item.id}
                    href={item.href}
                    className={cn(
                      'group relative flex items-start gap-2 rounded-md px-2.5 py-2 text-sm transition-colors',
                      isActive
                        ? 'bg-background text-foreground shadow-sm ring-1 ring-border/70'
                        : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'
                    )}
                  >
                    {/* Active indicator */}
                    {isActive && (
                      <div className="absolute bottom-2 left-0 top-2 w-0.5 rounded-r-full bg-foreground/70" />
                    )}

                    <Icon
                      className={cn(
                        'mt-0.5 h-4 w-4 flex-shrink-0 transition-colors',
                        isActive ? 'text-foreground' : 'text-muted-foreground'
                      )}
                    />

                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span
                          className={cn(
                            'truncate font-medium',
                            isActive ? 'text-foreground' : 'text-muted-foreground'
                          )}
                        >
                          {item.label}
                        </span>
                      </div>
                      <p
                        className={cn(
                          'mt-0.5 line-clamp-1 text-[11px] leading-4',
                          isActive ? 'text-muted-foreground' : 'text-muted-foreground/80'
                        )}
                      >
                        {item.desc}
                      </p>
                    </div>
                  </Link>
                );
              })}
            </div>
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
