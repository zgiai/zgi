import { Skeleton } from '@/components/ui/skeleton';

export function SysChatSkeleton() {
  return (
    <div className="flex-1 max-w-4xl mx-auto space-y-8 w-full p-4 pt-16 pb-64">
      {[1, 2, 3].map(i => (
        <div key={i} className="w-full space-y-4">
          {/* User Message Skeleton */}
          <div className="flex justify-end">
            <Skeleton className="h-10 w-2/3 rounded-2xl" />
          </div>

          {/* AI Message Skeleton */}
          <div className="flex justify-start w-full">
            <div className="w-full space-y-3">
              <div className="flex items-center gap-2">
                <Skeleton className="h-7 w-7 rounded-full" />
                <Skeleton className="h-4 w-24" />
              </div>
              <div className="pl-9 space-y-2 w-full">
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-4 w-[90%]" />
                <Skeleton className="h-4 w-[95%]" />
              </div>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}
