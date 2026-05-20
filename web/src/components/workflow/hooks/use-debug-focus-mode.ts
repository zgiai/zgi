'use client';

import { usePathname } from 'next/navigation';
import { useActivePanel, type WorkflowActivePanel } from './use-active-panel';

export function isWorkflowDebugPanelActive(panel: WorkflowActivePanel) {
  return panel === 'run' || panel === 'chat';
}

export function useWorkflowDebugFocusMode() {
  const pathname = usePathname();
  const activePanel = useActivePanel(state => state.active);
  const isAgentWorkflowPage =
    pathname.includes('/console/agents/') && pathname.endsWith('/workflow');

  return isAgentWorkflowPage && isWorkflowDebugPanelActive(activePanel);
}
