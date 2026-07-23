import React from 'react';
import { cn } from '@/lib/utils';

interface GraphCanvasProps {
  containerRef: React.RefObject<HTMLDivElement>;
  className?: string;
  nodeCount?: number;
  edgeCount?: number;
}

export const GraphCanvas: React.FC<GraphCanvasProps> = ({
  containerRef,
  className,
  nodeCount = 0,
  edgeCount = 0,
}) => {
  return (
    <div className={cn('w-full h-full relative', className)}>
      <div ref={containerRef} className="w-full h-full" />
      <div className="pointer-events-none absolute bottom-3 left-3 rounded bg-background/80 px-2 py-1 text-xs text-muted-foreground shadow-sm">
        {nodeCount} nodes · {edgeCount} edges
      </div>
    </div>
  );
};
