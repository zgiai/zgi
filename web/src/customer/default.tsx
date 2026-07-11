'use client';

import * as React from 'react';
import { usePathname } from 'next/navigation';
import { Loader2, ShieldAlert } from 'lucide-react';
import {
  ContextualAIChatDock,
  ContextualAIChatProvider,
  useAIChatContextRegistration,
  type AIChatContextItem,
} from '@/components/aichat/contextual';
import { WorkspaceRequiredState } from '@/components/common/workspace-required-state';
import { ConsoleHeader, ConsoleSidebar } from '@/components/console/console-shell-entry';
import { ConsoleMobileSidebar } from '@/components/console/console-sidebar';
import { DashboardMobileSidebar, DashboardSidebar } from '@/components/dashboard/sidebar';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useT } from '@/i18n';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import { useJoinedWorkspaces } from '@/hooks/workspace/use-joined-workspaces';
import {
  useCurrentWorkspace,
  useIsOrganizationMode,
  useWorkspaceStore,
} from '@/store/workspace-store';
import { getConsoleRouteAccess } from '@/routes/access';
import {
  createAIChatTraceInstanceId,
  logAIChatSessionTrace,
} from '@/components/chat/controllers/aichat/session-trace';
import type {
  CustomerAdapter,
  CustomerConsoleShellProps,
  CustomerDashboardShellProps,
  CustomerSessionBridgeProviderProps,
} from './types';

function ConsoleModelsPreloader() {
  useAvailableModels();
  return null;
}

function ConsolePageContextRegistration() {
  const pathname = usePathname();
  const currentWorkspace = useCurrentWorkspace();
  const isOrganizationMode = useIsOrganizationMode();
  const items = React.useMemo<AIChatContextItem[]>(
    () => [
      {
        id: pathname || '/console',
        type: 'page',
        title: pathname || '/console',
        subtitle: 'Console page',
        metadata: {
          route: pathname,
          workspace_id: currentWorkspace?.id,
          workspace_name: currentWorkspace?.name,
          organization_mode: isOrganizationMode,
        },
      },
    ],
    [currentWorkspace?.id, currentWorkspace?.name, isOrganizationMode, pathname]
  );

  useAIChatContextRegistration(items, { scopeId: 'console-page' });
  return null;
}
function ConsoleAccessDeniedState() {
  const t = useT();

  return (
    <div className="flex h-full w-full flex-col items-center justify-center p-4 text-center">
      <ShieldAlert className="mb-4 h-12 w-12 text-muted-foreground" />
      <h2 className="mb-2 text-xl font-semibold">{t('common.accessDenied')}</h2>
      <p className="max-w-md text-muted-foreground">{t('common.unauthorizedDescription')}</p>
    </div>
  );
}

function ConsoleCapabilityLoadingState() {
  return (
    <div className="flex h-full w-full items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  );
}
function DefaultConsoleShell({ children }: CustomerConsoleShellProps) {
  const pathname = usePathname();
  const routeAccess = getConsoleRouteAccess(pathname);
  const currentWorkspace = useCurrentWorkspace();
  const workspaces = useWorkspaceStore.use.workspaces();
  const contextStatus = useWorkspaceStore.use.contextStatus();
  const {
    isLoading: isCapabilitiesLoading,
    isFetching: isCapabilitiesFetching,
    canUseOrganizationScope,
    canUseWorkspaceScope,
    isWorkspaceRequired,
  } = useAccountCapabilities();
  const [mobileSidebarOpen, setMobileSidebarOpen] = React.useState(false);
  const shellInstanceIdRef = React.useRef<string | null>(null);
  if (!shellInstanceIdRef.current) {
    shellInstanceIdRef.current = createAIChatTraceInstanceId('console-shell');
  }
  const shellInstanceId = shellInstanceIdRef.current;
  const hiddenHeaderPaths: string[] = [];
  const hiddenSidebarPaths = [] as string[];
  const lastPath = pathname.split('/').pop();
  const usesManagedViewport =
    pathname.startsWith('/console/work/app/') ||
    pathname === '/console/work/task' ||
    pathname.startsWith('/console/work/task/');
  const hasActiveWorkspace = currentWorkspace
    ? workspaces.some(workspace => workspace.id === currentWorkspace.id)
    : false;
  const canUseWorkspaceContext = contextStatus === 'ready' && hasActiveWorkspace;
  const canRenderOrganizationRoute =
    routeAccess.scope === 'organization' && canUseOrganizationScope;
  const canRenderWorkspaceRoute =
    routeAccess.scope === 'workspace' &&
    canUseWorkspaceContext &&
    !isWorkspaceRequired &&
    canUseWorkspaceScope;
  const shouldShowWorkspaceRequired =
    routeAccess.scope === 'workspace' && (!canUseWorkspaceContext || isWorkspaceRequired);
  const shouldShowAccessDenied =
    routeAccess.scope === 'organization'
      ? !canUseOrganizationScope
      : canUseWorkspaceContext && !isWorkspaceRequired && !canUseWorkspaceScope;
  // Keep the active route mounted during background capability refetches. Chat message
  // components also observe this query, so treating `isFetching` as initial loading
  // would abort an in-flight conversation whenever a stale query refreshes.
  const isCapabilityLoading = isCapabilitiesLoading;

  useJoinedWorkspaces({ syncToStore: true });

  React.useEffect(() => {
    logAIChatSessionTrace('console_shell_mounted', {
      shellInstanceId,
      pathname,
    });
    return () => {
      logAIChatSessionTrace('console_shell_unmounted', {
        shellInstanceId,
        pathname,
        unmountStack: new Error('DefaultConsoleShell unmount observer').stack,
      });
    };
  }, [pathname, shellInstanceId]);

  React.useEffect(() => {
    const contentBranch = isCapabilityLoading
      ? 'capability_loading'
      : shouldShowWorkspaceRequired
        ? 'workspace_required'
        : shouldShowAccessDenied || (!canRenderOrganizationRoute && !canRenderWorkspaceRoute)
          ? 'access_denied'
          : 'children';
    logAIChatSessionTrace('console_shell_state', {
      shellInstanceId,
      pathname,
      contentBranch,
      routeScope: routeAccess.scope,
      contextStatus,
      currentWorkspaceId: currentWorkspace?.id ?? null,
      hasActiveWorkspace,
      isCapabilitiesLoading,
      isCapabilitiesFetching,
      isCapabilityLoading,
      canUseOrganizationScope,
      canUseWorkspaceScope,
      isWorkspaceRequired,
      canUseWorkspaceContext,
      canRenderOrganizationRoute,
      canRenderWorkspaceRoute,
      shouldShowWorkspaceRequired,
      shouldShowAccessDenied,
    });
  }, [
    canRenderOrganizationRoute,
    canRenderWorkspaceRoute,
    canUseOrganizationScope,
    canUseWorkspaceContext,
    canUseWorkspaceScope,
    contextStatus,
    currentWorkspace?.id,
    hasActiveWorkspace,
    isCapabilitiesFetching,
    isCapabilitiesLoading,
    isCapabilityLoading,
    isWorkspaceRequired,
    pathname,
    routeAccess.scope,
    shellInstanceId,
    shouldShowAccessDenied,
    shouldShowWorkspaceRequired,
  ]);

  let content = children;
  if (isCapabilityLoading) {
    content = <ConsoleCapabilityLoadingState />;
  } else if (shouldShowWorkspaceRequired) {
    content = <WorkspaceRequiredState />;
  } else if (shouldShowAccessDenied) {
    content = <ConsoleAccessDeniedState />;
  } else if (!canRenderOrganizationRoute && !canRenderWorkspaceRoute) {
    content = <ConsoleAccessDeniedState />;
  }

  return (
    <ContextualAIChatProvider>
      <div className="flex h-screen min-h-0 flex-col overflow-hidden bg-background">
        <ConsoleHeader
          hidden={hiddenHeaderPaths.includes(lastPath || '_')}
          onToggleMobileSidebar={() => setMobileSidebarOpen(true)}
        />
        <div className="flex h-0 min-h-0 min-w-0 grow">
          <ConsoleSidebar hidden={hiddenSidebarPaths.includes(lastPath || '_')} />
          <main
            className={
              usesManagedViewport
                ? 'h-full min-h-0 w-0 min-w-0 grow overflow-hidden'
                : 'h-full min-h-0 w-0 min-w-0 grow overflow-auto bg-bg-canvas'
            }
          >
            {content}
          </main>
          <ContextualAIChatDock />
        </div>
        <ConsoleMobileSidebar open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen} />
        <ConsoleModelsPreloader />
        <ConsolePageContextRegistration />
      </div>
    </ContextualAIChatProvider>
  );
}

function DefaultDashboardShell({ children }: CustomerDashboardShellProps) {
  const [mobileSidebarOpen, setMobileSidebarOpen] = React.useState(false);

  return (
    <div className="flex min-h-screen min-w-0 flex-col overflow-hidden bg-background">
      <ConsoleHeader onToggleMobileSidebar={() => setMobileSidebarOpen(true)} />
      <div className="flex h-0 min-w-0 grow">
        <DashboardSidebar />
        <div className="min-w-0 flex-1 overflow-auto">{children}</div>
      </div>
      {mobileSidebarOpen ? (
        <DashboardMobileSidebar open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen} />
      ) : null}
    </div>
  );
}

function DefaultSessionBridgeProvider({ children }: CustomerSessionBridgeProviderProps) {
  return <>{children}</>;
}

export const defaultCustomerAdapter: CustomerAdapter = {
  ConsoleShell: DefaultConsoleShell,
  DashboardShell: DefaultDashboardShell,
  SessionBridgeProvider: DefaultSessionBridgeProvider,
};
