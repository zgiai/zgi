import type { ReactNode } from 'react';
import { notFound } from 'next/navigation';
import { ENABLE_AGENT_RUNTIME_LOGS_PAGE } from '@/lib/config';

export default function AgentRuntimeLogsLayout({ children }: { children: ReactNode }) {
  if (!ENABLE_AGENT_RUNTIME_LOGS_PAGE) {
    notFound();
  }

  return children;
}
