'use client';

import React, { createContext, useContext, type ReactNode } from 'react';

interface ApiDocsContextValue {
  apibase: string;
}

const ApiDocsContext = createContext<ApiDocsContextValue>({ apibase: '' });

export function ApiDocsProvider({ children, apibase }: { children: ReactNode; apibase: string }) {
  return <ApiDocsContext.Provider value={{ apibase }}>{children}</ApiDocsContext.Provider>;
}

export function useApiDocs(): ApiDocsContextValue {
  return useContext(ApiDocsContext);
}
