import { useCallback, useEffect, useRef, useMemo } from 'react';
import { useDebouncedCommit } from './use-debounced-commit';
import { useNodeData } from './use-node-data';
import { useNodeDataUpdate } from './use-node-data-update';
import { registerWorkflowPendingEditFlush } from './pending-edits';
import { logWorkflowEditDebug } from '../utils/edit-debug';
import type { WorkflowNodeData } from '../store/type';

const EMPTY_OBJ = {} as unknown;

function isPlainEqual<T>(a: T, b: T): boolean {
  try {
    return JSON.stringify(a) === JSON.stringify(b);
  } catch {
    return a === b;
  }
}

export interface UseLocalNodeDataOptions<T> {
  delay?: number;
  onCommit?: (data: Partial<T>) => void;
  debugLabel?: string;
  isEqual?: (a: T, b: T) => boolean;
  flushOnUnmount?: boolean;
}

/**
 * Store-aware options for useLocalNodeData when using nodeId.
 */
export interface UseLocalNodeDataStoreOptions<T> {
  delay?: number;
  debugLabel?: string;
  isEqual?: (a: T, b: T) => boolean;
  flushOnUnmount?: boolean;
  /**
   * Optional path selector to pick a subset of node data.
   * Example: 'model' will select only the `model` field.
   */
  path?: string;
  /**
   * Register this local draft with the workflow-level pending edit flusher.
   * Defaults to true so save/run can capture debounced edits before reading the graph.
   */
  registerPendingFlush?: boolean;
}

/**
 * Manage a local editable copy of nodeData and debounce commits to the store.
 * - Immediate local updates for snappy UI
 * - Debounced commit to reduce store writes/history churn
 * - Structural updates can call `flush()` to commit instantly
 *
 * @overload Store-aware mode: Pass nodeId to automatically read/write from store
 * @overload Legacy mode: Pass externalData + onCommit for manual control
 */
export function useLocalNodeData<T>(
  nodeId: string,
  options?: UseLocalNodeDataStoreOptions<T>
): {
  localData: T;
  setLocalData: (update: Partial<T> | ((prev: T) => T)) => void;
  flush: () => void;
  cancel: () => void;
};

// eslint-disable-next-line no-redeclare, @typescript-eslint/no-redeclare
export function useLocalNodeData<T>(
  externalData: T,
  options: UseLocalNodeDataOptions<T> & { onCommit: (data: Partial<T>) => void }
): {
  localData: T;
  setLocalData: (update: Partial<T> | ((prev: T) => T)) => void;
  flush: () => void;
  cancel: () => void;
};

// eslint-disable-next-line no-redeclare, @typescript-eslint/no-redeclare
export function useLocalNodeData<T>(
  nodeIdOrData: string | T,
  options?: UseLocalNodeDataOptions<T> | UseLocalNodeDataStoreOptions<T>
): {
  localData: T;
  setLocalData: (update: Partial<T> | ((prev: T) => T)) => void;
  flush: () => void;
  cancel: () => void;
} {
  // Determine if store-aware mode (nodeId is a string)
  const isStoreAware = typeof nodeIdOrData === 'string';
  const nodeId = isStoreAware ? (nodeIdOrData as string) : '';
  const opts = options ?? {};
  const path = (opts as UseLocalNodeDataStoreOptions<T>).path;
  const debugLabel = (opts as UseLocalNodeDataStoreOptions<T>).debugLabel;
  const registerPendingFlush =
    (opts as UseLocalNodeDataStoreOptions<T>).registerPendingFlush ?? true;

  // Store-aware hooks
  const storeData = useNodeData<WorkflowNodeData>(nodeId);
  const updateStoreData = useNodeDataUpdate<WorkflowNodeData>(nodeId);

  // Determine external data source
  const externalData = useMemo(() => {
    if (!isStoreAware) return nodeIdOrData as T;
    if (!storeData) return EMPTY_OBJ as T;
    if (path) {
      const val = storeData[path as keyof WorkflowNodeData];
      return (val as unknown as T) ?? (EMPTY_OBJ as T);
    }
    return storeData as unknown as T;
  }, [isStoreAware, nodeIdOrData, storeData, path]);

  // Handle options
  const { delay = 300, isEqual = isPlainEqual, flushOnUnmount = true } = opts;
  const legacyOnCommit = (opts as UseLocalNodeDataOptions<T>).onCommit;

  const debug = useCallback(
    (message: string, data?: Record<string, unknown>) => {
      logWorkflowEditDebug(debugLabel, message, data);
    },
    [debugLabel]
  );

  // Determine onCommit callback
  const onCommit = useMemo(() => {
    if (isStoreAware) {
      // Store-aware: commit to store via updateStoreData
      return (data: Partial<T>) => {
        debug('commit local data to workflow store', { path, data });
        if (path) {
          updateStoreData({ [path as string]: data } as Partial<WorkflowNodeData>);
        } else {
          updateStoreData(data as Partial<WorkflowNodeData>);
        }
      };
    }
    // Legacy: use provided onCommit
    return legacyOnCommit ?? (() => {});
  }, [isStoreAware, updateStoreData, legacyOnCommit, path, debug]);

  const commitLocalData = useCallback(
    (data: T) => {
      debug('commit local draft', { data });
      onCommit(data as Partial<T>);
    },
    [debug, onCommit]
  );

  const {
    value: localData,
    setValue: setLocalValue,
    flush,
    cancel,
    isDirty,
  } = useDebouncedCommit<T>(externalData, {
    delay,
    onCommit: commitLocalData,
    debugLabel,
    isEqual,
    flushOnUnmount,
  });

  useEffect(() => {
    if (!registerPendingFlush) return;
    debug('register workflow pending edit flush');
    return registerWorkflowPendingEditFlush(() => {
      if (!isDirty()) return;
      flush();
    });
  }, [debug, flush, isDirty, registerPendingFlush]);

  useEffect(() => {
    debug('external node data changed', { externalData, path, isStoreAware });
  }, [debug, externalData, isStoreAware, path]);

  const latestRef = useRef<T>(externalData);
  useEffect(() => {
    latestRef.current = localData;
  }, [localData]);

  const setLocalData = useCallback(
    (update: Partial<T> | ((prev: T) => T)) => {
      const prev = latestRef.current;
      let next: T;
      if (typeof update === 'function') {
        next = (update as (p: T) => T)(prev);
      } else if (Array.isArray(prev)) {
        // For arrays, we assume the update is the whole new array
        next = update as T;
      } else if (typeof prev === 'object' && prev !== null) {
        next = {
          ...(prev as object),
          ...(update as object),
        } as T;
      } else {
        // For primitives, the update is the new value
        next = update as T;
      }
      latestRef.current = next;
      debug('set local draft value', { prev, next });
      setLocalValue(next);
    },
    [debug, setLocalValue]
  );

  return { localData, setLocalData, flush, cancel } as const;
}

export default useLocalNodeData;
