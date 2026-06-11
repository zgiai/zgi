'use client';

import * as React from 'react';
import { usePathname } from 'next/navigation';
import { ConsoleHeader, ConsoleSidebar } from '@/components/console/console-shell-entry';
import { ConsoleMobileSidebar } from '@/components/console/console-sidebar';
import { WorkspaceRequiredState } from '@/components/common/workspace-required-state';
import { DashboardMobileSidebar, DashboardSidebar } from '@/components/dashboard/sidebar';
import { useJoinedWorkspaces } from '@/hooks/workspace/use-joined-workspaces';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useCurrentWorkspace, useWorkspaceStore } from '@/store/workspace-store';
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

function DefaultConsoleShell({ children }: CustomerConsoleShellProps) {
  const pathname = usePathname();
  const currentWorkspace = useCurrentWorkspace();
  const workspaces = useWorkspaceStore.use.workspaces();
  const contextStatus = useWorkspaceStore.use.contextStatus();
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

  useJoinedWorkspaces({ syncToStore: true });

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
          {canUseWorkspaceContext ? children : <WorkspaceRequiredState />}
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
