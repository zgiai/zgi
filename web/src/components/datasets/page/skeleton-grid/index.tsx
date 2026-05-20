import { Skeleton } from '@/components/ui/skeleton';

export interface SkeletonGridProps {
  showFolderSkeletons: boolean;
  showDatasetSkeletons: boolean;
  isRootView: boolean;
  folderSkeletonCount?: number;
  datasetSkeletonCount?: number;
  className?: string;
}

/**
 * Unified skeleton grid for folders and datasets.
 */
export function SkeletonGrid({
  showFolderSkeletons,
  showDatasetSkeletons,
  isRootView,
  folderSkeletonCount = 20,
  datasetSkeletonCount = 20,
  className = 'grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-5 gap-3 sm:gap-4 md:gap-6 lg:gap-8 xl:gap-10 2xl:gap-12',
}: SkeletonGridProps) {
  if (!showFolderSkeletons && !showDatasetSkeletons) return null;

  return (
    <div className={className}>
      {showFolderSkeletons && (
        <>
          {Array.from({ length: folderSkeletonCount }).map((_, idx) => (
            <Skeleton key={`folder-skel-${idx}`} className="h-40 w-full" />
          ))}
        </>
      )}

      {isRootView && showDatasetSkeletons && (
        <>
          {Array.from({ length: datasetSkeletonCount }).map((_, idx) => (
            <Skeleton key={`ds-skel-${idx}`} className="h-40 w-full" />
          ))}
        </>
      )}
    </div>
  );
}
