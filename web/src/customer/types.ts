import type { ComponentType, ReactNode } from 'react';

export type CustomerAuthPageType =
  | 'login'
  | 'register'
  | 'forgot-password'
  | 'reset-password'
  | 'init'
  | 'invite'
  | 'activate'
  | 'sso-callback'
  | 'auth';

export interface CustomerAuthPageConfig {
  appName: string;
  brandName: string;
  titleLine1: string;
  titleLine2: string;
  description: string;
  backgroundImageUrl?: string;
  privacyUrl?: string;
  termsUrl?: string;
  supportUrl?: string;
}

export interface CustomerAuthSlots {
  Logo: ReactNode;
  LanguageSwitcher: ReactNode;
  BrandPanel: ReactNode;
  Form: ReactNode;
  Footer: ReactNode;
}

export interface CustomerAuthShellProps {
  page: CustomerAuthPageType;
  config: CustomerAuthPageConfig;
  slots: CustomerAuthSlots;
}

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
  AuthShell?: ComponentType<CustomerAuthShellProps>;
  ConsoleShell: ComponentType<CustomerConsoleShellProps>;
  DashboardShell: ComponentType<CustomerDashboardShellProps>;
  SessionBridgeProvider: ComponentType<CustomerSessionBridgeProviderProps>;
}
