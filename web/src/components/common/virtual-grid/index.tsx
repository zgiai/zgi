'use client';

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { cn } from '@/lib/utils';

// Generic virtualized grid component using row virtualization + computed columns
// - Fixed row height for predictable layout and optimal performance
// - Columns computed via ResizeObserver based on container width and minColumnWidth
// - Integrates with external scroll element (e.g., ScrollArea.Viewport or any overflow container)
// - Accessible roles set to grid/row/gridcell
// - Strict typing (no `any`)

export interface VirtualGridProps<T> {
  items: T[];
  itemKey: (item: T, index: number) => string;
  renderItem: (item: T, index: number) => React.ReactNode;
  // Reference to the scrollable element (required for best performance & correctness)
  scrollElementRef?: React.RefObject<HTMLElement | HTMLDivElement>;
  // Minimum column width in pixels, used to compute column count responsively
  minColumnWidth?: number; // default 280
  // Fixed columns override; when provided, computed columns will be ignored
  columns?: number;
  // Fixed row height in pixels (card height)
  rowHeight: number; // e.g. 160 for h-40
  // Spaces in pixels
  columnGap?: number; // default 16
  rowGap?: number; // default 16
  overscan?: number; // default 4 (rows)
  className?: string;
  // Fired when scrolled near the end (for infinite loading)
  onScrollEnd?: () => void;
}

export function VirtualGrid<T>({
  items,
  itemKey,
  renderItem,
  scrollElementRef,
  minColumnWidth = 280,
  columns: fixedColumns,
  rowHeight,
  columnGap = 16,
  rowGap = 16,
  overscan = 4,
  className,
  onScrollEnd,
}: VirtualGridProps<T>) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [containerWidth, setContainerWidth] = useState<number>(0);

  // Observe container width changes and update columns responsively
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver(entries => {
      const entry = entries[0];
      const width = entry.contentRect.width;
      setContainerWidth(width);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const columns = useMemo(() => {
    if (fixedColumns && fixedColumns >= 1) return fixedColumns;
    const safeWidth = Math.max(0, containerWidth);
    const minW = Math.max(80, minColumnWidth); // safety guard
    return Math.max(1, Math.floor(safeWidth / minW));
  }, [containerWidth, minColumnWidth, fixedColumns]);

  const rowsCount = useMemo(() => {
    return columns > 0 ? Math.ceil(items.length / columns) : 0;
  }, [items.length, columns]);

  // Unconditional hook call to satisfy React Hooks rules
  const rowVirtualizer = useVirtualizer({
    count: rowsCount,
    overscan,
    estimateSize: () => rowHeight + rowGap,
    getScrollElement: () => scrollElementRef?.current ?? containerRef.current,
  });

  // Optional: trigger onScrollEnd when close to bottom
  useEffect(() => {
    if (!onScrollEnd) return;
    const el = scrollElementRef?.current ?? containerRef.current;
    if (!el) return;
    const threshold = 64; // px
    const handler = () => {
      if (el.scrollTop + el.clientHeight >= el.scrollHeight - threshold) {
        onScrollEnd();
      }
    };
    el.addEventListener('scroll', handler);
    return () => el.removeEventListener('scroll', handler);
  }, [onScrollEnd, scrollElementRef]);

  // Total height computed by virtualizer
  const totalSize = rowVirtualizer.getTotalSize();
  const virtualRows = rowVirtualizer.getVirtualItems();

  return (
    <div
      ref={containerRef}
      className={cn('relative w-full', className)}
      role="grid"
      aria-rowcount={rowsCount}
    >
      {/* Spacer with total height for virtualization */}
      <div style={{ height: totalSize, position: 'relative' }}>
        {virtualRows.map(vRow => {
          // Absolute position row container at its start offset
          const top = vRow.start;
          const rowIndex = vRow.index;
          const startItemIndex = rowIndex * columns;
          const endItemIndex = Math.min(startItemIndex + columns, items.length);
          const rowItems = items.slice(startItemIndex, endItemIndex);

          return (
            <div
              key={vRow.key}
              role="row"
              aria-rowindex={rowIndex + 1}
              style={{
                position: 'absolute',
                top,
                left: 0,
                right: 0,
                height: rowHeight,
                // We use CSS grid to layout columns uniformly
                display: 'grid',
                gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
                columnGap: `${columnGap}px`,
              }}
            >
              {rowItems.map((item, i) => {
                const idx = startItemIndex + i;
                return (
                  <div
                    key={itemKey(item, idx)}
                    role="gridcell"
                    aria-colindex={(i % columns) + 1}
                    style={{
                      height: rowHeight,
                    }}
                  >
                    {renderItem(item, idx)}
                  </div>
                );
              })}
            </div>
          );
        })}
      </div>
    </div>
  );
}

export default VirtualGrid;
