// Viewport slice: viewport state and setter with layout-change tracking
// Strict typing, no any

import type { Viewport } from '@xyflow/react';

export interface ViewportSlice {
  viewport: Viewport;
  creationOffsetIndex: number;
  setViewport: (viewport: Viewport, options?: { markLayoutDirty?: boolean }) => void;
  incrementCreationOffsetIndex: () => void;
  resetCreationOffsetIndex: () => void;
}

interface ViewportGet {
  viewport: Viewport;
  creationOffsetIndex: number;
  // Flag from run-status slice to distinguish programmatic panning (e.g., auto-follow)
  isProgrammaticPan?: boolean;
  suppressNextViewportDirty?: boolean;
}

export type StoreSet = (partial: unknown, replace?: boolean, action?: string) => void;

export function createViewportSlice(set: StoreSet, get: () => ViewportGet): ViewportSlice {
  return {
    viewport: { x: 0, y: 0, zoom: 1 },
    creationOffsetIndex: 0,
    setViewport(viewport: Viewport, options?: { markLayoutDirty?: boolean }) {
      const prev = get().viewport;
      const same = prev.x === viewport.x && prev.y === viewport.y && prev.zoom === viewport.zoom;
      if (same) return;

      // Reset staggering offset when viewport moves substantially
      const isShift = Math.abs(prev.x - viewport.x) > 1 || Math.abs(prev.y - viewport.y) > 1;

      // When viewport changes are programmatic (auto-follow during runs),
      // do not mark hasLayoutChanges to keep semantic dirty separate from run-time panning.
      const programmatic = Boolean(get().isProgrammaticPan);
      const suppressViewportDirty = Boolean(get().suppressNextViewportDirty);
      const markLayoutDirty = options?.markLayoutDirty ?? true;
      const shouldMarkLayoutDirty = markLayoutDirty && !programmatic && !suppressViewportDirty;
      if (!shouldMarkLayoutDirty) {
        set(
          {
            viewport,
            ...(suppressViewportDirty ? { suppressNextViewportDirty: false } : {}),
          },
          false,
          'setViewport'
        );
      } else {
        // Direct set without callback to avoid implicit any on state parameter
        set(
          {
            viewport,
            hasLayoutChanges: true,
            ...(isShift ? { creationOffsetIndex: 0 } : {}),
          },
          false,
          'setViewport'
        );
      }
    },
    incrementCreationOffsetIndex() {
      set(
        { creationOffsetIndex: get().creationOffsetIndex + 1 },
        false,
        'incrementCreationOffsetIndex'
      );
    },
    resetCreationOffsetIndex() {
      set({ creationOffsetIndex: 0 }, false, 'resetCreationOffsetIndex');
    },
  };
}
