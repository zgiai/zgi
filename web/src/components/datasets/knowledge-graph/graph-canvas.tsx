import React from 'react';
import { cn } from '@/lib/utils';

interface GraphCanvasProps {
  containerRef: React.RefObject<HTMLDivElement>;
  className?: string;
}

export const GraphCanvas: React.FC<GraphCanvasProps> = ({ containerRef, className }) => {
  return (
    <div className={cn('w-full h-full relative', className)}>
      <div ref={containerRef} className="w-full h-full" />
    </div>
  );
};
