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
  className = 'grid grid-cols-[repeat(auto-fill,minmax(15rem,1fr))] gap-4',
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
