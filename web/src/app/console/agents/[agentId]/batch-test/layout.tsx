import type { ReactNode } from 'react';
import { notFound } from 'next/navigation';
import { ENABLE_AGENT_BATCH_TEST_PAGE } from '@/lib/config';

export default function AgentBatchTestLayout({ children }: { children: ReactNode }) {
  if (!ENABLE_AGENT_BATCH_TEST_PAGE) {
    notFound();
  }

  return children;
}
