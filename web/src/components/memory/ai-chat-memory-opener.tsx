'use client';

import { useMemo, type ReactNode } from 'react';
import { useRouter } from 'next/navigation';
import { AgentMemoryOpenerContext } from '@/components/memory/agent-memory-opener';

// AI chat keeps one memory per person (ADR 0021); its relationship has this
// fixed consumer ID.
export const AI_CHAT_MEMORY_CONSUMER_ID = '00000000-0000-0000-0000-00000000a1c0';

/** Opens the person's AI chat memory in "My memory". */
export const AI_CHAT_MEMORY_HREF = `/profile/memory?consumer_id=${AI_CHAT_MEMORY_CONSUMER_ID}`;

/** Lets memory notices in AI chat replies open the person's AI chat memory. */
export function AIChatMemoryOpenerProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const opener = useMemo(() => ({ open: () => router.push(AI_CHAT_MEMORY_HREF) }), [router]);
  return (
    <AgentMemoryOpenerContext.Provider value={opener}>{children}</AgentMemoryOpenerContext.Provider>
  );
}
