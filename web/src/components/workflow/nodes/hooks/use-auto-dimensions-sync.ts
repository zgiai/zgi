import * as React from 'react';
import { useUpdateNodeInternals } from '@xyflow/react';
import { useWorkflowStore } from '../../store/store';

/**
 * Observe element size changes and request React Flow to re-measure node dimensions.
 * This ensures DOM-driven auto-height reflects into node.height via onNodesChange('dimensions').
 */
export default function useAutoDimensionsSync(
  nodeId: string | null | undefined,
  element: HTMLElement | null
): void {
  const updateNodeInternals = useUpdateNodeInternals();
  const mode = useWorkflowStore.use.mode();
  const nodes = useWorkflowStore.use.nodes();

  const lastHeightRef = React.useRef<number | null>(null);
  const frameRef = React.useRef<number | null>(null);
  const pendingRef = React.useRef<boolean>(false);
  const mountedRef = React.useRef<boolean>(false);

  React.useEffect(() => {
    if (!nodeId || !element) return;
    if (mode === 'history') return; // skip in read-only mode

    // Check if the node already has valid dimensions (from stored layout)
    // If so, skip the initial updateNodeInternals call to reduce mount-time overhead
    const existingNode = nodes.find(n => n.id === nodeId);
    const hasValidDimensions =
      existingNode?.width != null &&
      existingNode?.height != null &&
      existingNode.width > 0 &&
      existingNode.height > 0;

    let isUnmounted = false;

    const handle = () => {
      if (isUnmounted) return;
      pendingRef.current = false;
      try {
        updateNodeInternals(nodeId);
      } catch {
        // no-op
      }
    };

    const ro = new ResizeObserver(entries => {
      if (!entries || entries.length === 0) return;
      const entry = entries[0];
      const nextH = entry.contentRect?.height ?? element.getBoundingClientRect().height;
      const lastH = lastHeightRef.current;
      // Ignore minuscule fluctuations
      if (lastH !== null && Math.abs(nextH - lastH) < 0.5) return;
      lastHeightRef.current = nextH;

      // Skip the first resize event on mount if the node already has valid dimensions
      // This prevents a storm of updateNodeInternals calls during initial render
      if (!mountedRef.current && hasValidDimensions) {
        mountedRef.current = true;
        return;
      }
      mountedRef.current = true;

      if (!pendingRef.current) {
        pendingRef.current = true;
        if (typeof window !== 'undefined' && typeof window.requestAnimationFrame === 'function') {
          frameRef.current = window.requestAnimationFrame(handle);
        } else {
          handle();
        }
      }
    });

    try {
      ro.observe(element);
    } catch {
      // ignore
    }

    return () => {
      isUnmounted = true;
      try {
        ro.disconnect();
      } catch {
        // ignore
      }
      if (frameRef.current !== null && typeof window !== 'undefined') {
        try {
          window.cancelAnimationFrame(frameRef.current);
        } catch {
          // ignore
        }
      }
      frameRef.current = null;
      pendingRef.current = false;
    };
  }, [nodeId, element, mode, updateNodeInternals]);
}
