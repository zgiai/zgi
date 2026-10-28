import { createContext, useContext } from 'react';

// Lets a chat surface that owns an Agent's memory dialog (the WebApp, the
// builder's debug chat) be opened from a memory notice inside a reply.
export interface AgentMemoryOpener {
  open: () => void;
  /** Tells the surface that memory just changed, so it can refresh. */
  notifyChanged?: () => void;
}

export const AgentMemoryOpenerContext = createContext<AgentMemoryOpener | null>(null);

export function useAgentMemoryOpener(): AgentMemoryOpener | null {
  return useContext(AgentMemoryOpenerContext);
}
