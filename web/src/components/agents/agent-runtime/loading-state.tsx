'use client';

import { Skeleton } from '@/components/ui/skeleton';

export function AgentRuntimeLoadingState() {
  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-background">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b px-4">
        <Skeleton className="size-8 rounded-md" />
        <Skeleton className="h-5 w-40" />
        <Skeleton className="ml-auto h-9 w-24" />
      </div>
      <div className="grid min-h-0 flex-1 grid-cols-3 divide-x">
        <div className="p-5">
          <Skeleton className="h-full w-full" />
        </div>
        <div className="p-5">
          <Skeleton className="h-full w-full" />
        </div>
        <div className="p-5">
          <Skeleton className="h-full w-full" />
        </div>
      </div>
    </div>
  );
}
