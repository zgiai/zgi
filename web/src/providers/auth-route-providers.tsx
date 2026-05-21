'use client';

import type { PropsWithChildren } from 'react';
import { Toaster } from '@/components/ui/sonner';
import { customerAdapter } from '@/customer';
import { AuthProvider } from './auth-provider';
import { QueryProvider } from './query-provider';
import { ThemeProvider } from './theme-provider';

export function AuthRouteProviders({ children }: PropsWithChildren) {
  const SessionBridgeProvider = customerAdapter.SessionBridgeProvider;

  return (
    <QueryProvider>
      <ThemeProvider defaultTheme="light" enableSystem>
        <AuthProvider>
          <SessionBridgeProvider>
            {children}
            <Toaster richColors position="top-right" />
          </SessionBridgeProvider>
        </AuthProvider>
      </ThemeProvider>
    </QueryProvider>
  );
}
