'use client';

import { QueryClientProvider } from '@tanstack/react-query';
// Comment out DevTools to avoid client-side only errors
// import { ReactQueryDevtools } from '@tanstack/react-query-devtools';
import type { PropsWithChildren } from 'react';
import { useEffect } from 'react';
import { useIsInitialized, useIsAuthenticated } from '@/store/auth-store';
import { queryClient, setQueryClientQueriesEnabled } from '@/lib/query-client';

/**
 * React Query Provider
 * Provide React Query functionality for the entire app
 */
export function QueryProvider({ children }: PropsWithChildren) {
  const isAuthReady = useIsInitialized();
  const isAuthenticated = useIsAuthenticated();

  // Once auth ready, enable queries and refetch disabled ones
  useEffect(() => {
    if (!isAuthReady) return;

    setQueryClientQueriesEnabled(true);

    // Only trigger global refetch/preload when authenticated
    if (isAuthenticated) {
      queryClient.invalidateQueries();
    }
  }, [isAuthReady, isAuthenticated]);

  return (
    <QueryClientProvider client={queryClient}>
      {children}
      {/* Temporarily disable DevTools to avoid issues */}
      {/* <ReactQueryDevtools initialIsOpen={false} /> */}
    </QueryClientProvider>
  );
}
