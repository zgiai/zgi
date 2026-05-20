import { useNodeId, useUpdateNodeInternals } from '@xyflow/react';
import { useWorkflowStore } from '../../store';
import React from 'react';
import { PAD_X, PAD_Y } from '../../ui/create-node-modal/constants/iteration-layout';
import { cn } from '@/lib/utils';

/**
 * Bottom-right manual resize handle
 * Uses existing SVG and updates node.width/height in store during drag.
 * Calls updateNodeInternals once on release to emit dimensions change.
 */
export default function ManualResizeHandle({
  minWidth = 300,
  minHeight = 200,
}: {
  minWidth?: number;
  minHeight?: number;
}) {
  const nodeId = useNodeId();
  const mode = useWorkflowStore.use.mode();
  const viewport = useWorkflowStore.use.viewport();
  const updateNode = useWorkflowStore.use.updateNode();
  const updateNodeInternals = useUpdateNodeInternals();

  const dragRafRef = React.useRef<number | null>(null);

  const onMouseDown = React.useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (!nodeId) return;
      if (mode === 'history') return; // disabled in read-only mode

      e.preventDefault();
      e.stopPropagation();

      const startX = e.clientX;
      const startY = e.clientY;
      const state = useWorkflowStore.getState();
      const node = state.nodes.find(n => n.id === nodeId);
      const startW = (node?.width ?? 280) as number;
      const startH = (node?.height ?? 120) as number;

      const onMouseMove = (ev: MouseEvent) => {
        if (dragRafRef.current) cancelAnimationFrame(dragRafRef.current);
        dragRafRef.current = requestAnimationFrame(() => {
          // Convert screen-space delta to flow-space using current zoom
          const zoom = (viewport?.zoom ?? 1) as number;
          const dw = (ev.clientX - startX) / zoom;
          const dh = (ev.clientY - startY) / zoom;
          // Enforce min-size for iteration parent to avoid squeezing child nodes
          let minAllowedW = minWidth;
          let minAllowedH = minHeight;
          try {
            const st = useWorkflowStore.getState();
            const cur = st.nodes.find(n => n.id === (nodeId as string));
            const t = (cur?.data as { type?: string } | undefined)?.type;
            if (t === 'iteration') {
              let maxRight = 0;
              let maxBottom = 0;
              for (const child of st.nodes) {
                const pid = (child as unknown as { parentId?: string })?.parentId;
                if (pid !== (nodeId as string)) continue;
                const cw = (child.width ?? 240) as number;
                const ch = (child.height ?? 120) as number;
                const pos = child.position || { x: 0, y: 0 };
                const right = pos.x + cw;
                const bottom = pos.y + ch;
                if (right > maxRight) maxRight = right;
                if (bottom > maxBottom) maxBottom = bottom;
              }
              minAllowedW = Math.max(minAllowedW, maxRight + PAD_X * 2);
              minAllowedH = Math.max(minAllowedH, maxBottom + PAD_Y * 2);
            }
          } catch {
            // ignore and fall back to provided minWidth/minHeight
          }
          const nextW = Math.max(minAllowedW, startW + dw);
          const nextH = Math.max(minAllowedH, startH + dh);
          updateNode(nodeId as string, { width: nextW, height: nextH });
        });
      };

      const onMouseUp = () => {
        if (dragRafRef.current) {
          cancelAnimationFrame(dragRafRef.current);
          dragRafRef.current = null;
        }
        updateNodeInternals(nodeId as string);
        window.removeEventListener('mousemove', onMouseMove);
        window.removeEventListener('mouseup', onMouseUp);
      };

      window.addEventListener('mousemove', onMouseMove);
      window.addEventListener('mouseup', onMouseUp);
    },
    [nodeId, mode, viewport?.zoom, updateNode, updateNodeInternals, minWidth, minHeight]
  );

  return (
    <div
      onMouseDown={onMouseDown}
      className={cn('nodrag nowheel group')}
      style={{
        position: 'absolute',
        right: 0,
        bottom: 0,
        width: 18,
        height: 18,
        cursor: mode === 'history' ? 'not-allowed' : 'se-resize',
        boxSizing: 'border-box',
      }}
      aria-label="Resize"
    >
      <svg
        className="absolute bottom-1 right-1 text-highlight/50 group-hover:text-highlight"
        viewBox="0 0 1024 1024"
        version="1.1"
        xmlns="http://www.w3.org/2000/svg"
        p-id="2645"
        width="16"
        height="16"
      >
        <path
          d="M27.149838 1016.887351h437.146522c305.13355 0 552.517189-247.328288 552.590991-552.461837v-437.275676a19.345297 19.345297 0 0 0-19.243819-19.24382H863.204324a19.345297 19.345297 0 0 0-19.234594 19.24382v437.275676c0 209.615568-169.928649 379.553441-379.553442 379.553441h-437.275675A19.26227 19.26227 0 0 0 7.906018 863.204324v134.429982c0 10.544432 8.570234 19.24382 19.24382 19.24382z"
          p-id="2646"
          data-spm-anchor-id="a313x.search_index.0.i2.483e3a81uNnrXf"
          fill="currentColor"
        />
      </svg>
    </div>
  );
}
