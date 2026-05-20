'use client';

import type { PropsWithChildren } from 'react';
import { ThemeProvider } from './theme-provider';
import { QueryProvider } from './query-provider';
import { AuthProvider } from './auth-provider';
import { DomMutationGuard } from '@/components/common/dom-mutation-guard';
import { Toaster } from '@/components/ui/sonner';
import { TooltipProvider } from '@/components/ui/tooltip';
import { customerAdapter } from '@/customer';

export function Providers({ children }: PropsWithChildren) {
  const SessionBridgeProvider = customerAdapter.SessionBridgeProvider;

  return (
    <QueryProvider>
      <ThemeProvider defaultTheme="light" enableSystem>
        <AuthProvider>
          <SessionBridgeProvider>
            <TooltipProvider delayDuration={300}>
              <DomMutationGuard />
              {children}
              <Toaster richColors position="top-right" />
            </TooltipProvider>
          </SessionBridgeProvider>
        </AuthProvider>
      </ThemeProvider>
    </QueryProvider>
  );
}
