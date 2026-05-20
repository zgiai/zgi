import { QueryClient } from '@tanstack/react-query';

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      enabled: false,
      refetchOnWindowFocus: false,
      retry: false,
      staleTime: 5 * 60 * 1000,
    },
    mutations: {
      retry: false,
    },
  },
});

export function setQueryClientQueriesEnabled(enabled: boolean): void {
  const defaults = queryClient.getDefaultOptions();
  const queryDefaults = defaults.queries ?? {};
  queryClient.setDefaultOptions({
    ...defaults,
    queries: {
      ...queryDefaults,
      enabled,
    },
  });
}

export default queryClient;
