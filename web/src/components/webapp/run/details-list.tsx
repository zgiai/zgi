'use client';

import React from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { NodeInfo } from '@/components/chat/types';
import { CheckCircle2, XCircle, Loader2, PauseCircle } from 'lucide-react';
import { useT } from '@/i18n';

interface DetailsListProps {
  items: NodeInfo[];
}

/**
 * Virtualized list of node execution details
 * Displays status, title, elapsed time, and errors
 */
export const DetailsList: React.FC<DetailsListProps> = ({ items }) => {
  const t = useT();
  const parentRef = React.useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 60,
    overscan: 5,
  });

  const virtualItems = virtualizer.getVirtualItems();

  const renderStatusIcon = (status: NodeInfo['status']) => {
    switch (status) {
      case 'success':
        return <CheckCircle2 className="w-4 h-4 text-green-500" />;
      case 'failed':
        return <XCircle className="w-4 h-4 text-red-500" />;
      case 'running':
        return <Loader2 className="w-4 h-4 text-blue-500 animate-spin" />;
      case 'paused':
        return <PauseCircle className="w-4 h-4 text-warning" />;
      default:
        return null;
    }
  };

  const formatElapsed = (ms: number | undefined) => {
    if (!ms) return '';
    return ms < 1000 ? `${ms.toFixed(0)}ms` : `${(ms / 1000).toFixed(2)}s`;
  };

  if (items.length === 0) {
    return (
      <div className="h-full flex items-center justify-center text-sm text-muted-foreground">
        {t('webapp.run.empty')}
      </div>
    );
  }

  return (
    <div ref={parentRef} className="h-full overflow-auto">
      <div
        style={{
          height: `${virtualizer.getTotalSize()}px`,
          width: '100%',
          position: 'relative',
        }}
      >
        {virtualItems.map(virtualItem => {
          const item = items[virtualItem.index];
          return (
            <div
              key={virtualItem.key}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                height: `${virtualItem.size}px`,
                transform: `translateY(${virtualItem.start}px)`,
              }}
              className="px-3 py-2 border-b"
            >
              <div className="flex items-start gap-2">
                <div className="mt-0.5">{renderStatusIcon(item.status)}</div>
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-medium truncate">{item.title || item.nodeId}</div>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
                    {item.nodeType && <span className="truncate">{item.nodeType}</span>}
                    {item.elapsedTime !== undefined && (
                      <span className="shrink-0">
                        {t('webapp.run.elapsed')}: {formatElapsed(item.elapsedTime)}
                      </span>
                    )}
                  </div>
                  {item.error && (
                    <div className="text-xs text-red-500 mt-1 line-clamp-2">{item.error}</div>
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};
