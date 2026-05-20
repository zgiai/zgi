'use client';

import { memo } from 'react';
import { Skeleton } from '@/components/ui/skeleton';

// Loading skeleton component for initial load state
export const LoadingSkeleton = memo(function LoadingSkeleton() {
  return (
    <div className="p-2 space-y-2">
      {Array.from({ length: 8 }).map((_, i) => (
        <div key={i} className="flex items-center gap-2">
          <Skeleton className="h-4 w-28" />
          <Skeleton className="h-4 w-48" />
        </div>
      ))}
    </div>
  );
});
