import React from 'react';
import { FloatingPanel } from '@/components/ui/floating-panel';

interface CreateNodeFloatingPickerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  anchorClientPosition: { x: number; y: number } | null;
  children: React.ReactNode;
}

function getFallbackAnchor() {
  if (typeof window === 'undefined') return { x: 0, y: 0 };
  return {
    x: Math.round(window.innerWidth / 2 - 144),
    y: Math.round(window.innerHeight / 2 - 220),
  };
}

function getAdaptiveMaxHeight(anchorY: number) {
  if (typeof window === 'undefined') return 440;

  const viewportPadding = 12;
  const anchorGap = 8;
  const preferredMaxHeight = 480;
  const minimumUsefulHeight = 300;
  const viewportHeight = window.innerHeight;
  const spaceAbove = Math.max(0, anchorY - viewportPadding - anchorGap);
  const spaceBelow = Math.max(0, viewportHeight - anchorY - viewportPadding - anchorGap);
  const bestAvailableSpace = Math.max(spaceAbove, spaceBelow);
  const viewportBoundedMax = Math.max(260, viewportHeight - viewportPadding * 2);

  if (bestAvailableSpace >= preferredMaxHeight) return preferredMaxHeight;
  if (bestAvailableSpace >= minimumUsefulHeight) return bestAvailableSpace;

  return Math.min(viewportBoundedMax, Math.max(bestAvailableSpace, 260));
}

/**
 * @component CreateNodeFloatingPicker
 * @category Workflow
 * @status Stable
 * @description Desktop floating shell for the workflow node creation picker
 * @usage Use for contextual add/insert node interactions on desktop
 * @example
 * <CreateNodeFloatingPicker open={open} onOpenChange={setOpen} anchorClientPosition={point} />
 */
export function CreateNodeFloatingPicker({
  open,
  onOpenChange,
  anchorClientPosition,
  children,
}: CreateNodeFloatingPickerProps) {
  const fallbackAnchor = getFallbackAnchor();
  const anchor = anchorClientPosition ?? fallbackAnchor;
  const maxHeight = React.useMemo(() => getAdaptiveMaxHeight(anchor.y), [anchor.y]);

  return (
    <FloatingPanel
      open={open}
      onOpenChange={onOpenChange}
      x={anchor.x}
      y={anchor.y}
      offsetSize={8}
      maxWidth={240}
      maxHeight={maxHeight}
      className="overflow-hidden rounded-xl border border-border/80 bg-popover p-0 text-popover-foreground shadow-lg shadow-black/10"
      style={{ zIndex: 80 }}
    >
      <div
        className="flex h-full min-h-0 flex-col"
        onPointerDown={event => event.stopPropagation()}
        onClick={event => event.stopPropagation()}
        onContextMenu={event => {
          event.preventDefault();
          event.stopPropagation();
        }}
      >
        {children}
      </div>
    </FloatingPanel>
  );
}
