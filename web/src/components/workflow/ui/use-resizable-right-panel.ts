import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { MouseEvent as ReactMouseEvent } from 'react';

const DEFAULT_WIDTH = 420;
const MIN_WIDTH = 360;
const MAX_WIDTH = 560;
const MAX_VIEWPORT_RATIO = 0.45;

interface UseResizableRightPanelOptions {
  cssVar: `--${string}`;
  defaultWidth?: number;
  minWidth?: number;
  maxWidth?: number;
  maxViewportRatio?: number;
}

function getMaxWidth(minWidth: number, maxWidth: number, maxViewportRatio: number) {
  if (typeof window === 'undefined') return maxWidth;
  return Math.max(minWidth, Math.min(maxWidth, window.innerWidth * maxViewportRatio));
}

function clampPanelWidth(
  width: number,
  minWidth: number,
  maxWidth: number,
  maxViewportRatio: number
) {
  return Math.max(minWidth, Math.min(getMaxWidth(minWidth, maxWidth, maxViewportRatio), width));
}

export function useResizableRightPanel({
  cssVar,
  defaultWidth = DEFAULT_WIDTH,
  minWidth = MIN_WIDTH,
  maxWidth = MAX_WIDTH,
  maxViewportRatio = MAX_VIEWPORT_RATIO,
}: UseResizableRightPanelOptions) {
  const initialWidth = clampPanelWidth(defaultWidth, minWidth, maxWidth, maxViewportRatio);
  const [panelWidth, setPanelWidth] = useState(initialWidth);
  const [isResizing, setIsResizing] = useState(false);
  const resizeRaf = useRef<number | null>(null);
  const dragCleanupRef = useRef<(() => void) | null>(null);
  const dragWidthRef = useRef(initialWidth);

  const clampWidth = useCallback(
    (width: number) => clampPanelWidth(width, minWidth, maxWidth, maxViewportRatio),
    [maxViewportRatio, maxWidth, minWidth]
  );

  useEffect(() => {
    document.documentElement.style.setProperty(cssVar, `${panelWidth}px`);
    dragWidthRef.current = panelWidth;
  }, [cssVar, panelWidth]);

  useEffect(() => {
    const onResize = () => {
      setPanelWidth(width => clampWidth(width));
    };

    window.addEventListener('resize', onResize);
    return () => {
      window.removeEventListener('resize', onResize);
    };
  }, [clampWidth]);

  useEffect(() => {
    return () => {
      dragCleanupRef.current?.();
      dragCleanupRef.current = null;
      if (resizeRaf.current) {
        cancelAnimationFrame(resizeRaf.current);
        resizeRaf.current = null;
      }
    };
  }, []);

  const onMouseDown = useCallback(
    (event: ReactMouseEvent<HTMLDivElement>) => {
      event.preventDefault();
      event.stopPropagation();
      dragCleanupRef.current?.();
      setIsResizing(true);
      const startX = event.clientX;
      const startWidth = panelWidth;

      function onMouseMove(moveEvent: MouseEvent) {
        if (resizeRaf.current) cancelAnimationFrame(resizeRaf.current);
        const next = clampWidth(startWidth + startX - moveEvent.clientX);
        dragWidthRef.current = next;
        resizeRaf.current = requestAnimationFrame(() => {
          document.documentElement.style.setProperty(cssVar, `${next}px`);
          resizeRaf.current = null;
        });
      }

      function cleanupDrag() {
        window.removeEventListener('mousemove', onMouseMove);
        window.removeEventListener('mouseup', onMouseUp);
        if (resizeRaf.current) {
          cancelAnimationFrame(resizeRaf.current);
          resizeRaf.current = null;
        }
      }

      function onMouseUp() {
        cleanupDrag();
        dragCleanupRef.current = null;
        setIsResizing(false);
        setPanelWidth(clampWidth(dragWidthRef.current));
      }

      window.addEventListener('mousemove', onMouseMove);
      window.addEventListener('mouseup', onMouseUp);
      dragCleanupRef.current = cleanupDrag;
    },
    [clampWidth, cssVar, panelWidth]
  );

  const panelWidthStyle = useMemo(
    () => ({
      width: `min(var(${cssVar}, ${defaultWidth}px), calc(100vw - 32px))`,
    }),
    [cssVar, defaultWidth]
  );

  return {
    panelWidth,
    isResizing,
    panelWidthStyle,
    resizeHandleProps: { onMouseDown },
  };
}
