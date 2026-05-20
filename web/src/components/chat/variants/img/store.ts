import { create } from 'zustand';

interface SysImageState {
  pendingPrompt: string;
  setPendingPrompt: (prompt: string) => void;
  clearPendingPrompt: () => void;
}

export const useSysImageStore = create<SysImageState>(set => ({
  pendingPrompt: '',
  setPendingPrompt: prompt => set({ pendingPrompt: prompt }),
  clearPendingPrompt: () => set({ pendingPrompt: '' }),
}));
