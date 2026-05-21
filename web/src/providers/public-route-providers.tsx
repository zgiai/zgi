'use client';

import type { PropsWithChildren } from 'react';
import { Toaster } from '@/components/ui/sonner';
import { QueryProvider } from './query-provider';
import { ThemeProvider } from './theme-provider';

export function PublicRouteProviders({ children }: PropsWithChildren) {
  return (
    <QueryProvider>
      <ThemeProvider defaultTheme="light" enableSystem>
        {children}
        <Toaster richColors position="top-right" />
      </ThemeProvider>
    </QueryProvider>
  );
}
