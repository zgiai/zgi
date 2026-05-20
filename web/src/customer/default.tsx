'use client';

import * as React from 'react';
import { usePathname } from 'next/navigation';
import { ConsoleHeader, ConsoleSidebar } from '@/components/console/console-shell-entry';
import { ConsoleMobileSidebar } from '@/components/console/console-sidebar';
import { DashboardMobileSidebar, DashboardSidebar } from '@/components/dashboard/sidebar';
import { useAvailableModels } from '@/hooks/model/use-model';
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
  const [mobileSidebarOpen, setMobileSidebarOpen] = React.useState(false);
  const hiddenHeaderPaths: string[] = [];
  const hiddenSidebarPaths = [] as string[];
  const lastPath = pathname.split('/').pop();
  const usesManagedViewport =
    pathname.startsWith('/console/work/app/') ||
    pathname === '/console/work/task' ||
    pathname.startsWith('/console/work/task/');

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
          {children}
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
      <DashboardMobileSidebar open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen} />
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
