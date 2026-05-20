import type { RefObject, ReactNode } from 'react';
import VirtualGrid from '@/components/common/virtual-grid';

export interface VirtualContentGridProps<T> {
  items: T[];
  itemKey: (item: T) => string;
  renderItem: (item: T) => ReactNode;
  rowHeight?: number;
  scrollElementRef?: RefObject<HTMLDivElement | HTMLElement>;
  columnGap?: number;
  rowGap?: number;
  overscan?: number;
  onScrollEnd?: () => void;
  className?: string;
}

/**
 * Thin wrapper over VirtualGrid with sensible defaults.
 */
export function VirtualContentGrid<T>({
  items,
  itemKey,
  renderItem,
  rowHeight = 160,
  scrollElementRef,
  columnGap = 16,
  rowGap = 16,
  overscan = 6,
  onScrollEnd,
  className = 'lg:px-20',
}: VirtualContentGridProps<T>) {
  return (
    <div className={className}>
      <VirtualGrid
        items={items}
        itemKey={itemKey}
        renderItem={renderItem}
        rowHeight={rowHeight}
        scrollElementRef={scrollElementRef}
        columnGap={columnGap}
        rowGap={rowGap}
        overscan={overscan}
        onScrollEnd={onScrollEnd}
      />
    </div>
  );
}
