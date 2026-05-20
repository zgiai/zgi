import type { ComponentType, ReactNode } from 'react';

export interface CustomerConsoleShellProps {
  children: ReactNode;
}

export interface CustomerDashboardShellProps {
  children: ReactNode;
}

export interface CustomerSessionBridgeProviderProps {
  children: ReactNode;
}

export interface CustomerAdapter {
  ConsoleShell: ComponentType<CustomerConsoleShellProps>;
  DashboardShell: ComponentType<CustomerDashboardShellProps>;
  SessionBridgeProvider: ComponentType<CustomerSessionBridgeProviderProps>;
}
