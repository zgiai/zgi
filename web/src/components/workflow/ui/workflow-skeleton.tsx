import { Skeleton } from '@/components/ui/skeleton';

export const WorkflowSkeleton = () => {
  return (
    <div className="h-screen w-full flex flex-col bg-gray-50">
      {/* Header skeleton */}
      {/* <div className="flex-shrink-0 border-b border-gray-200 bg-white h-20 px-4 py-2">
        <div className="flex items-center justify-between h-full">
          <Skeleton className="h-5 w-[400px]" />
          <div className="flex items-center gap-2">
            <Skeleton className="h-8 w-28" />
            <Skeleton className="h-8 w-24" />
            <Skeleton className="h-8 w-24" />
          </div>
        </div>
      </div> */}
      {/* Canvas skeleton */}
      <div className="flex-1 p-4">
        <Skeleton className="h-full w-full rounded-lg" />
      </div>
    </div>
  );
};
