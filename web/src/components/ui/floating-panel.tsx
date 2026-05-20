'use client';

import React, { useEffect, useMemo, useRef } from 'react';
import {
  FloatingPortal,
  useDismiss,
  useFloating,
  useInteractions,
  useRole,
  offset,
  flip,
  shift,
  size,
  autoUpdate,
} from '@floating-ui/react';
import { cn } from '@/lib/utils';

type FloatingPortalRoot = React.ComponentProps<typeof FloatingPortal>['root'];

export interface FloatingPanelProps {
  // Control open state
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // Viewport client coordinates to anchor to
  x: number;
  y: number;
  // Optional styling
  className?: string;
  style?: React.CSSProperties;
  // Accessibility role (limited to supported roles)
  role?: 'dialog' | 'tooltip';
  // Collision protection padding
  collisionPadding?: number;
  // Distance between the virtual anchor and floating panel
  offsetSize?: number;
  // Max dimensions
  maxWidth?: number | string;
  maxHeight?: number | string;
  // Dismiss behaviors
  closeOnEscape?: boolean;
  closeOnPointerDownOutside?: boolean;
  // Optional portal root for modal-safe overlay rendering
  portalRoot?: FloatingPortalRoot;
  // Content
  children: React.ReactNode;
}

// Virtual element from a viewport point
function createVirtualFromPoint(x: number, y: number): { getBoundingClientRect: () => DOMRect } {
  return {
    getBoundingClientRect: () => new DOMRect(x, y, 0, 0),
  };
}

function toCssSize(value: number | string) {
  return typeof value === 'number' ? `${value}px` : value;
}

/**
 * FloatingPanel - Portal-based floating container that can open at any viewport point.
 * - Uses @floating-ui/react for robust positioning and overflow prevention.
 * - Renders into document.body via FloatingPortal to avoid clipping and layout shift.
 * - Provides built-in outside press and Escape dismissal.
 */
export function FloatingPanel({
  open,
  onOpenChange,
  x,
  y,
  className,
  style,
  role = 'dialog',
  collisionPadding = 8,
  offsetSize = 4,
  maxWidth = 360,
  maxHeight = 260,
  closeOnEscape = true,
  closeOnPointerDownOutside = true,
  portalRoot,
  children,
}: FloatingPanelProps) {
  const virtualRef = useRef(createVirtualFromPoint(x, y));
  useEffect(() => {
    virtualRef.current = createVirtualFromPoint(x, y);
  }, [x, y]);

  const { refs, floatingStyles, context, update } = useFloating({
    placement: 'bottom-start',
    open,
    onOpenChange,
    whileElementsMounted: autoUpdate,
    strategy: 'fixed',
    middleware: [
      offset(offsetSize),
      flip({ padding: collisionPadding }),
      shift({ padding: collisionPadding }),
      size({
        apply({ availableWidth, availableHeight, elements }) {
          const el = elements.floating as HTMLElement;
          // Constrain panel to available viewport size while respecting props
          const constrainedMaxWidth =
            typeof maxWidth === 'number' ? `${Math.min(maxWidth, availableWidth)}px` : maxWidth;
          el.style.width = constrainedMaxWidth;
          el.style.maxWidth = constrainedMaxWidth;
          el.style.maxHeight =
            typeof maxHeight === 'number'
              ? `${Math.min(maxHeight, availableHeight)}px`
              : String(maxHeight);
          el.style.height = el.style.maxHeight;
        },
        padding: collisionPadding,
      }),
    ],
  });

  // Attach virtual reference and update position on open/coords change
  useEffect(() => {
    refs.setReference(virtualRef.current as unknown as Element);
    void update?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refs, open, x, y]);

  const dismiss = useDismiss(context, {
    escapeKey: closeOnEscape,
    outsidePress: closeOnPointerDownOutside,
  });
  const roleInt = useRole(context, { role });
  const { getFloatingProps } = useInteractions([dismiss, roleInt]);

  const styles = useMemo<React.CSSProperties>(() => {
    return {
      ...floatingStyles,
      width: toCssSize(maxWidth),
      maxWidth: toCssSize(maxWidth),
      height: toCssSize(maxHeight),
      maxHeight: toCssSize(maxHeight),
      zIndex: 50,
      ...style,
    };
  }, [floatingStyles, maxWidth, maxHeight, style]);

  if (!open) return null;

  return (
    <FloatingPortal root={portalRoot}>
      <div
        ref={refs.setFloating}
        data-slot="floating-panel"
        className={cn(
          'z-50 overflow-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md outline-none',
          className
        )}
        style={styles}
        {...getFloatingProps()}
      >
        {children}
      </div>
    </FloatingPortal>
  );
}

export default FloatingPanel;
