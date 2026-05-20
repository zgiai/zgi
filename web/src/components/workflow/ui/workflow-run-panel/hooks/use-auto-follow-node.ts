import { useCallback } from 'react';
import type { ReactFlowInstance } from '@xyflow/react';
import { useWorkflowStore } from '@/components/workflow/store';
import { getNodeAbsolutePosition } from '@/components/workflow/store/helpers/graph';
import type { WorkflowNode } from '@/components/workflow/store/type';

interface ViewportLike {
  x?: number;
  y?: number;
  zoom?: number;
}

/**
 * Encapsulates auto-follow logic to center view on a running node while respecting panel offset.
 */
export function useAutoFollowNode(rf: ReactFlowInstance, viewport: ViewportLike) {
  return useCallback(
    (nodeId: string) => {
      try {
        const st = useWorkflowStore.getState() as unknown as {
          mode?: 'edit' | 'history';
          isAutoFollow?: boolean;
          setProgrammaticPan?: (enabled: boolean) => void;
        };
        const follow = Boolean(st?.isAutoFollow);
        const inHistory = st?.mode === 'history';
        if (!inHistory && follow) {
          const nodes = rf.getNodes();
          const node = rf.getNode(nodeId);
          const zoom = viewport.zoom ?? 1;
          if (node && node.position) {
            try {
              const setProgrammatic = st?.setProgrammaticPan;
              if (typeof setProgrammatic === 'function') setProgrammatic(true);
            } catch {
              /* no-op */
            }
            const absPos = getNodeAbsolutePosition(nodeId, nodes as WorkflowNode[]);
            const ax = absPos.x + (node.width ?? 0) / 2;
            const ay = absPos.y + (node.height ?? 0) / 2;
            // Compensate for right run panel width (500px): shift center RIGHT by half/zoom
            const RUN_PANEL_WIDTH = 500;
            const offsetFlowX = RUN_PANEL_WIDTH / 2 / (viewport.zoom || 1);
            rf.setCenter(ax + offsetFlowX, ay, { zoom, duration: 400 });
            window.setTimeout(() => {
              try {
                const setProgrammatic = st?.setProgrammaticPan;
                if (typeof setProgrammatic === 'function') setProgrammatic(false);
              } catch {
                /* no-op */
              }
            }, 450);
          }
        }
      } catch {
        // no-op
      }
    },
    [rf, viewport]
  );
}
