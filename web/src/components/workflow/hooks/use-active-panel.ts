'use client';

import { create } from 'zustand';

export type WorkflowActivePanel =
  | 'run'
  | 'chat'
  | 'conversation-history'
  | 'conversation-variables'
  | 'environment-variables'
  | 'features'
  | null;

interface ActivePanelState {
  active: WorkflowActivePanel;
  /** Set the active panel, or null to close all */
  setActive: (panel: WorkflowActivePanel) => void;
  /** Toggle a panel open/closed */
  toggle: (panel: Exclude<WorkflowActivePanel, null>) => void;
  /** Close all panels */
  closeAll: () => void;
}

export const useActivePanel = create<ActivePanelState>((set, get) => ({
  active: null,
  setActive: (panel: WorkflowActivePanel) => set({ active: panel }),
  toggle: (panel: Exclude<WorkflowActivePanel, null>) =>
    set({ active: get().active === panel ? null : panel }),
  closeAll: () => set({ active: null }),
}));
