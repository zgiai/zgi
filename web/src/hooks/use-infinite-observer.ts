import { useEffect, useRef, type RefObject } from 'react';

interface UseInfiniteObserverParams {
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => Promise<unknown>;
  rootMargin?: string;
  // Optional root element for IntersectionObserver; defaults to viewport
  rootRef?: RefObject<HTMLElement | null>;
}

/**
 * IntersectionObserver hook for infinite scrolling.
 * Returns a ref to attach to a sentinel element.
 */
export function useInfiniteObserver({
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  rootMargin = '200px',
  rootRef,
}: UseInfiniteObserverParams) {
  const sentinelRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const el = sentinelRef.current;
    if (!el) return;

    const observer = new IntersectionObserver(
      entries => {
        if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
          void fetchNextPage();
        }
      },
      { root: rootRef?.current ?? null, rootMargin }
    );

    observer.observe(el);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage, rootMargin, rootRef]);

  return sentinelRef;
}
