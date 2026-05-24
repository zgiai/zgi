import { useCallback, useEffect, useRef, useState } from 'react';

import { logWorkflowEditDebug } from '../utils/edit-debug';

interface UseDebouncedCommitOptions<T> {
  delay?: number;
  onCommit: (value: T) => void;
  debugLabel?: string;
  /**
   * Compare function to determine equality between values to avoid redundant commits.
   */
  isEqual?: (a: T, b: T) => boolean;
  /**
   * If true, automatically flush pending value when the component unmounts.
   * This prevents data loss when the user makes changes and immediately closes the panel.
   */
  flushOnUnmount?: boolean;
}

/**
 * Keep a local value and commit changes after a debounce interval.
 * - Call `setValue` freely for smooth typing.
 * - Commit runs after `delay` ms of inactivity or when `flush` is called (e.g., onBlur).
 * - If `flushOnUnmount` is true, pending changes are committed when the component unmounts.
 */
export function useDebouncedCommit<T>(initialValue: T, options: UseDebouncedCommitOptions<T>) {
  const { delay = 300, onCommit, debugLabel, isEqual, flushOnUnmount = false } = options;
  const [value, setValue] = useState<T>(initialValue);
  const lastCommittedRef = useRef<T>(initialValue);
  const timerRef = useRef<number | null>(null);

  // Keep refs to latest values for unmount flush (avoids stale closure issues)
  const latestValueRef = useRef<T>(value);
  const latestOnCommitRef = useRef(onCommit);
  const latestIsEqualRef = useRef(isEqual);

  const debug = useCallback(
    (message: string, data?: Record<string, unknown>) => {
      logWorkflowEditDebug(debugLabel, message, data);
    },
    [debugLabel]
  );

  const isDirty = useCallback(() => {
    const current = latestValueRef.current;
    const lastCommitted = lastCommittedRef.current;
    const eq = latestIsEqualRef.current;
    return eq ? !eq(current, lastCommitted) : current !== lastCommitted;
  }, []);

  useEffect(() => {
    latestValueRef.current = value;
  }, [value]);

  useEffect(() => {
    latestOnCommitRef.current = onCommit;
  }, [onCommit]);

  useEffect(() => {
    latestIsEqualRef.current = isEqual;
  }, [isEqual]);

  const setNextValue = useCallback((nextValue: T | ((prev: T) => T)) => {
    const current = latestValueRef.current;
    const next =
      typeof nextValue === 'function' ? (nextValue as (value: T) => T)(current) : nextValue;
    latestValueRef.current = next;
    setValue(next);
  }, []);

  const valuesEqual = useCallback(
    (left: T, right: T) => (isEqual ? isEqual(left, right) : left === right),
    [isEqual]
  );

  // Sync local value when external initialValue changes
  useEffect(() => {
    const hasPendingLocalValue = !valuesEqual(value, lastCommittedRef.current);

    if (hasPendingLocalValue) {
      if (valuesEqual(value, initialValue)) {
        debug('external synced to pending local value; mark committed', {
          initialValue,
          value,
        });
        if (timerRef.current) {
          window.clearTimeout(timerRef.current);
          timerRef.current = null;
        }
        lastCommittedRef.current = initialValue;
      }
      debug('ignore external value because local edit is pending', {
        initialValue,
        value,
        lastCommitted: lastCommittedRef.current,
      });
      return;
    }

    if (valuesEqual(value, initialValue)) {
      lastCommittedRef.current = initialValue;
      return;
    }

    debug('sync local value from external initialValue', {
      initialValue,
      previousValue: value,
      lastCommitted: lastCommittedRef.current,
    });
    setValue(initialValue);
    lastCommittedRef.current = initialValue;
    // Do not trigger commit here; this is an upstream change
  }, [debug, initialValue, value, valuesEqual]);

  // Flush pending changes on unmount if enabled
  useEffect(() => {
    if (!flushOnUnmount) return;
    return () => {
      if (timerRef.current) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
      if (isDirty()) {
        const current = latestValueRef.current;
        const lastCommitted = lastCommittedRef.current;
        debug('flush on unmount commit', { current, lastCommitted });
        lastCommittedRef.current = current;
        latestOnCommitRef.current(current);
      }
    };
    // Only depend on flushOnUnmount to avoid frequent cleanup recreation
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [flushOnUnmount]);

  // Debounced effect
  useEffect(() => {
    if (valuesEqual(value, lastCommittedRef.current)) {
      return;
    }
    if (timerRef.current) window.clearTimeout(timerRef.current);
    debug('schedule debounced commit', {
      delay,
      value,
      lastCommitted: lastCommittedRef.current,
    });
    timerRef.current = window.setTimeout(() => {
      lastCommittedRef.current = value;
      debug('execute debounced commit', { value });
      onCommit(value);
      timerRef.current = null;
    }, delay);
    return () => {
      if (timerRef.current) {
        debug('clear scheduled commit on effect cleanup');
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [debug, value, delay, onCommit, valuesEqual]);

  const flush = useCallback(() => {
    if (timerRef.current) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    const current = latestValueRef.current;
    if (isDirty()) {
      debug('manual flush commit', { value: current, lastCommitted: lastCommittedRef.current });
      lastCommittedRef.current = current;
      latestOnCommitRef.current(current);
    } else {
      debug('manual flush skipped; no pending value', { value: current });
    }
  }, [debug, isDirty]);

  const cancel = useCallback(() => {
    if (timerRef.current) {
      debug('cancel scheduled commit');
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, [debug]);

  return { value, setValue: setNextValue, flush, cancel, isDirty } as const;
}

export default useDebouncedCommit;
