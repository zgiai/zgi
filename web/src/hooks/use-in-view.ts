'use client';

import { useEffect, useRef, useState } from 'react';

interface UseInViewOptions {
  threshold?: number;
  rootMargin?: string;
  triggerOnce?: boolean;
}

interface UseInViewReturn {
  ref: (node?: Element | null) => void;
  inView: boolean;
  entry?: IntersectionObserverEntry;
}

/**
 * Hook that tracks whether an element is in view using Intersection Observer
 * @param options - Configuration options for the intersection observer
 * @returns Object containing ref callback, inView boolean, and entry
 */
export function useInView({
  threshold = 0,
  rootMargin = '0px',
  triggerOnce = false,
}: UseInViewOptions = {}): UseInViewReturn {
  const [inView, setInView] = useState(false);
  const [entry, setEntry] = useState<IntersectionObserverEntry>();
  const elementRef = useRef<Element | null>(null);
  const observerRef = useRef<IntersectionObserver | null>(null);

  const setRef = (node?: Element | null) => {
    if (elementRef.current && observerRef.current) {
      observerRef.current.unobserve(elementRef.current);
    }

    elementRef.current = node || null;

    if (node && !observerRef.current) {
      observerRef.current = new IntersectionObserver(
        ([entry]) => {
          const isIntersecting = entry.isIntersecting;
          setInView(isIntersecting);
          setEntry(entry);

          if (isIntersecting && triggerOnce && observerRef.current && elementRef.current) {
            observerRef.current.unobserve(elementRef.current);
            observerRef.current = null;
          }
        },
        {
          threshold,
          rootMargin,
        }
      );
    }

    if (node && observerRef.current) {
      observerRef.current.observe(node);
    }
  };

  useEffect(() => {
    return () => {
      if (observerRef.current) {
        observerRef.current.disconnect();
        observerRef.current = null;
      }
    };
  }, []);

  return {
    ref: setRef,
    inView,
    entry,
  };
}
