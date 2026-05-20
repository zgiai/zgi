'use client';

import React from 'react';
import { Skeleton } from '@/components/ui/skeleton';

export function LoadingSidebar() {
  return (
    <div className="space-y-3">
      {[1, 2, 3, 4, 5].map(i => (
        <div key={i} className="flex items-center justify-between p-3 border rounded-lg">
          <div className="flex items-center space-x-3 flex-1 min-w-0">
            <Skeleton className="h-5 w-5 rounded" />
            <div className="space-y-1 flex-1 min-w-0">
              <Skeleton className="h-4 w-20" />
              <Skeleton className="h-3 w-16" />
            </div>
          </div>
          <div className="flex items-center space-x-2 flex-shrink-0">
            <Skeleton className="h-5 w-10" />
            <Skeleton className="h-7 w-7" />
          </div>
        </div>
      ))}
    </div>
  );
}
