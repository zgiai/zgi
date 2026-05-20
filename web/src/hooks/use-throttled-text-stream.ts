'use client';

import { useEffect, useRef } from 'react';
import {
  createTextStreamThrottler,
  type TextStreamThrottler,
} from '@/utils/throttle-text-stream';

/**
 * Hook that exposes a stable text-stream throttler for client rendering.
 */
export function useThrottledTextStream(
  interval: number,
  apply: (text: string) => void
): TextStreamThrottler {
  const applyRef = useRef(apply);

  useEffect(() => {
    applyRef.current = apply;
  }, [apply]);

  const throttlerRef = useRef<TextStreamThrottler>({
    append: () => {},
    flush: () => {},
    cancel: () => {},
  });

  useEffect(() => {
    const throttler = createTextStreamThrottler(interval, text => applyRef.current(text));
    throttlerRef.current = throttler;

    return () => {
      throttler.flush();
      throttler.cancel();
    };
  }, [interval]);

  return throttlerRef.current;
}
