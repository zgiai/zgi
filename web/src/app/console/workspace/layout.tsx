'use client';

import { useEffect } from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { useT } from '@/i18n';
import Link from 'next/link';
import { cn } from '@/lib/utils';
import { Users, Settings } from 'lucide-react';
import {
  useCurrentWorkspace,
  useWorkspaceContextStatus,
  useHasHydrated,
} from '@/store/workspace-store';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { Loader2 } from 'lucide-react';

interface WorkspaceNavItem {
  id: string;
  label: string;
  desc: string;
  icon: React.ComponentType<{ className?: string }>;
  href: string;
}

export default function WorkspaceLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const t = useT();
  const currentWorkspace = useCurrentWorkspace();
  const contextStatus = useWorkspaceContextStatus();
  const hasHydrated = useHasHydrated();
  const { hasPermission, isLoading: isLoadingPermissions } = useAccountPermissions();

  const workspaceNavItems: WorkspaceNavItem[] = [
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

  // Access guard: redirect if not in workspace context or missing group.view permission
  // Only execute AFTER hydration and permission loading are complete
  useEffect(() => {
    if (!hasHydrated || isLoadingPermissions) return;

    if (contextStatus !== 'ready' || !currentWorkspace || !hasPermission('workspace.view')) {
      router.replace('/console');
    }
  }, [
    hasHydrated,
    isLoadingPermissions,
    contextStatus,
    currentWorkspace,
    hasPermission,
    router,
  ]);

  // Loading state
  if (!hasHydrated || isLoadingPermissions) {
    return (
      <div className="flex h-full items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  // Don't render if not in workspace context or missing permission (safety check)
  if (contextStatus !== 'ready' || !currentWorkspace || !hasPermission('workspace.view')) {
    return null;
  }

  return (
    <div className="flex h-full bg-background">
      {/* Workspace Sidebar */}
      <div className="w-60 shrink-0 border-r border-border/70 bg-muted/20">
        <div className="flex h-full flex-col">
          {/* Header */}
          <div className="border-b border-border/60 px-4 py-4">
            <div className="min-w-0">
              <h1 className="text-sm font-semibold text-foreground">{t.workspace('pageTitle')}</h1>
              <p className="mt-1 truncate text-xs text-muted-foreground">{currentWorkspace.name}</p>
            </div>
          </div>

          {/* Workspace Navigation */}
          <div className="flex-1 overflow-y-auto px-3 py-3">
            <div className="space-y-0.5">
              {workspaceNavItems.map(item => {
                const Icon = item.icon;
                const isActive = pathname.includes(item.href);

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
