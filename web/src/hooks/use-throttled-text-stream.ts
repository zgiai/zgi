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

  const throttlerRef = useRef<TextStreamThrottler | null>(null);
  if (throttlerRef.current === null) {
    // SSE callbacks may run before an effect-triggered rerender. Creating the
    // first implementation synchronously prevents the initial chunks from
    // being routed to a temporary no-op throttler.
    throttlerRef.current = createTextStreamThrottler(interval, text => applyRef.current(text));
  }

  const publicThrottlerRef = useRef<TextStreamThrottler | null>(null);
  if (publicThrottlerRef.current === null) {
    // Keep a stable facade so callbacks captured by useMemo always delegate to
    // the current implementation when the interval changes.
    publicThrottlerRef.current = {
      append: text => throttlerRef.current?.append(text),
      flush: () => throttlerRef.current?.flush(),
      cancel: () => throttlerRef.current?.cancel(),
    };
  }

  useEffect(() => {
    throttlerRef.current?.flush();
    throttlerRef.current?.cancel();
    const throttler = createTextStreamThrottler(interval, text => applyRef.current(text));
    throttlerRef.current = throttler;

    return () => {
      throttler.flush();
      throttler.cancel();
    };
  }, [interval]);

  return publicThrottlerRef.current;
}
