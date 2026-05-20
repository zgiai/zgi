'use client';

import React, { useEffect } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';

// Strictly typed, generic virtual list component
export interface VirtualListProps<T> {
  items: T[];
  itemKey: (item: T, index: number) => string;
  renderItem: (item: T, index: number) => React.ReactNode;
  /** Fixed item height (px) for better performance */
  estimateSize: number;
  /** Gap (px) between items. Default 0 */
  gap?: number;
  /** Overscan count to render above/below the viewport */
  overscan?: number;
  /** Scroll container element ref (e.g., ScrollArea viewport) */
  scrollElementRef: React.RefObject<HTMLElement>;
  /** Disable virtual list and render all items directly */
  disabled?: boolean;
  /** Class name for wrapper */
  className?: string;
  /** Optional role for a11y (e.g. listbox) */
  role?: string;
  /** Optional aria-label for a11y */
  ariaLabel?: string;
  /** Placeholder when items are empty */
  emptyPlaceholder?: React.ReactNode;
  /** Fired when scrolled to the end (for infinite loading) */
  onScrollEnd?: () => void;
}

function isNearScrollEnd(el: HTMLElement): boolean {
  const threshold = 64; // px
  return el.scrollTop + el.clientHeight >= el.scrollHeight - threshold;
}

export default function VirtualList<T>({
  items,
  itemKey,
  renderItem,
  estimateSize,
  gap = 0,
  overscan = 8,
  scrollElementRef,
  disabled = false,
  className,
  role,
  ariaLabel,
  emptyPlaceholder,
  onScrollEnd,
}: VirtualListProps<T>) {
  // Always initialize the virtualizer to satisfy rules-of-hooks
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollElementRef.current,
    estimateSize: () => estimateSize + gap,
    overscan,
  });

  // Optional: trigger onScrollEnd when close to bottom
  useEffect(() => {
    const el = scrollElementRef.current;
    if (!el || !onScrollEnd) return;
    const handler = () => {
      if (isNearScrollEnd(el)) onScrollEnd();
    };
    el.addEventListener('scroll', handler);
    return () => el.removeEventListener('scroll', handler);
  }, [scrollElementRef, onScrollEnd]);

  // Non-virtualized rendering path (disabled or small lists)
  if (disabled) {
    if (!items || items.length === 0) {
      return (
        <div className={className} role={role} aria-label={ariaLabel}>
          {emptyPlaceholder ?? null}
        </div>
      );
    }
    return (
      <div className={className} role={role} aria-label={ariaLabel}>
        {items.map((item, index) => (
          <div key={itemKey(item, index)} style={{ marginBottom: gap }}>
            {renderItem(item, index)}
          </div>
        ))}
      </div>
    );
  }

  const virtualItems = virtualizer.getVirtualItems();
  const totalSize = virtualizer.getTotalSize();

  return (
    <div className={className} role={role} aria-label={ariaLabel}>
      {items.length === 0 ? (
        emptyPlaceholder ?? null
      ) : (
        <div style={{ position: 'relative', height: totalSize }}>
          {virtualItems.map(vi => {
            const item = items[vi.index];
            const key = itemKey(item, vi.index);
            return (
              <div
                key={key}
                data-index={vi.index}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  transform: `translateY(${vi.start}px)`,
                  height: vi.size,
                }}
              >
                <div style={{ height: estimateSize }}>
                  {renderItem(item, vi.index)}
                </div>
                {gap > 0 ? <div style={{ height: gap }} /> : null}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
