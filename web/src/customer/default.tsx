'use client';

import * as React from 'react';
import { usePathname } from 'next/navigation';
import { Loader2, ShieldAlert } from 'lucide-react';
import { ConsoleHeader, ConsoleSidebar } from '@/components/console/console-shell-entry';
import { ConsoleMobileSidebar } from '@/components/console/console-sidebar';
import { WorkspaceRequiredState } from '@/components/common/workspace-required-state';
import { DashboardMobileSidebar, DashboardSidebar } from '@/components/dashboard/sidebar';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import { useJoinedWorkspaces } from '@/hooks/workspace/use-joined-workspaces';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useT } from '@/i18n';
import { useCurrentWorkspace, useWorkspaceStore } from '@/store/workspace-store';
import { getConsoleRouteAccess } from '@/routes/access';
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
  const isCapabilityLoading = isCapabilitiesLoading || isCapabilitiesFetching;

  useJoinedWorkspaces({ syncToStore: true });

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
    <div className="flex h-screen min-h-0 flex-col bg-background overflow-hidden">
      <ConsoleHeader
        hidden={hiddenHeaderPaths.includes(lastPath || '_')}
        onToggleMobileSidebar={() => setMobileSidebarOpen(true)}
      />
      <div className="flex h-0 grow min-h-0 min-w-0">
        <ConsoleSidebar hidden={hiddenSidebarPaths.includes(lastPath || '_')} />
        <main
          className={
            usesManagedViewport
              ? 'h-full min-h-0 min-w-0 w-0 grow overflow-hidden'
              : 'h-full min-h-0 min-w-0 w-0 grow overflow-auto bg-bg-canvas'
          }
        >
          {content}
        </main>
      </div>
      <ConsoleMobileSidebar open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen} />
      <ConsoleModelsPreloader />
    </div>
  );
}

function DefaultDashboardShell({ children }: CustomerDashboardShellProps) {
  const [mobileSidebarOpen, setMobileSidebarOpen] = React.useState(false);

  return (
    <div className="flex min-h-screen min-w-0 flex-col bg-background overflow-hidden">
      <ConsoleHeader onToggleMobileSidebar={() => setMobileSidebarOpen(true)} />
      <div className="flex h-0 grow min-w-0">
        <DashboardSidebar />
        <div className="flex-1 min-w-0 overflow-auto">{children}</div>
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
