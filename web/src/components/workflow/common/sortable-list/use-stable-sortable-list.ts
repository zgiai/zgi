import React, { useCallback, useEffect, useMemo, useRef } from 'react';
import {
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import { arrayMove, sortableKeyboardCoordinates } from '@dnd-kit/sortable';

import { logWorkflowEditDebug } from '@/components/workflow/utils/edit-debug';

export interface StableSortableOptions<T> {
  derive: () => T[];
  deps: React.DependencyList;
  isRowEqual: (a: T, b: T) => boolean;
  serialize: (next: T[]) => void;
  debugLabel?: string;
}

export interface StableSortableResult<T> {
  rows: T[];
  items: Array<{ id: string; index: number }>;
  sensors: ReturnType<typeof useSensors>;
  handleDragEnd: (event: DragEndEvent) => void;
  append: (item: T) => void;
  removeAt: (index: number) => void;
  updateAt: (index: number, updater: (cur: T) => T) => void;
  setRows: React.Dispatch<React.SetStateAction<T[]>>;
}

export function useStableSortableList<T>(
  options: StableSortableOptions<T>
): StableSortableResult<T> {
  const { derive, deps, isRowEqual, serialize, debugLabel } = options;
  const idsRef = useRef<string[]>([]);
  const internalUpdateRef = useRef(false);
  // Track commit version to guard multiple rapid commits from being overwritten by async derive
  const commitVersionRef = useRef(0);
  const lastAppliedVersionRef = useRef(0);
  // Temporal lock to ignore rapid derive updates right after commit
  const lockUntilTsRef = useRef(0);

  const debug = useCallback(
    (message: string, data?: Record<string, unknown>) => {
      logWorkflowEditDebug(debugLabel ? `${debugLabel}:sortable` : undefined, message, data);
    },
    [debugLabel]
  );

  const [rows, setRows] = React.useState<T[]>(() => {
    const initial = derive();
    if (idsRef.current.length === 0 && initial.length > 0) {
      idsRef.current = Array.from(
        { length: initial.length },
        () => `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
      );
    }
    return initial;
  });
  const rowsRef = useRef(rows);

  useEffect(() => {
    rowsRef.current = rows;
  }, [rows]);

  useEffect(() => {
    if (idsRef.current.length < rows.length) {
      const missing = rows.length - idsRef.current.length;
      for (let i = 0; i < missing; i += 1) {
        idsRef.current.push(`${Date.now()}-${Math.random().toString(36).slice(2, 10)}`);
      }
    } else if (idsRef.current.length > rows.length) {
      idsRef.current.splice(rows.length);
    }
  }, [rows.length]);

  useEffect(() => {
    if (idsRef.current.length === 0 && rows.length > 0) {
      idsRef.current = Array.from(
        { length: rows.length },
        () => `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
      );
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const now = Date.now();
    if (
      internalUpdateRef.current ||
      lastAppliedVersionRef.current < commitVersionRef.current ||
      now < lockUntilTsRef.current
    ) {
      // Skip this derive once to allow store to catch up with our recent commits
      debug('skip derive update after internal commit', {
        internalUpdate: internalUpdateRef.current,
        commitVersion: commitVersionRef.current,
        lastAppliedVersion: lastAppliedVersionRef.current,
        lockUntil: lockUntilTsRef.current,
        now,
      });
      internalUpdateRef.current = false;
      lastAppliedVersionRef.current = commitVersionRef.current;
      return;
    }
    const incoming = derive();
    let differs = incoming.length !== rows.length;
    if (!differs) {
      for (let i = 0; i < incoming.length; i += 1) {
        const a = incoming[i];
        const b = rows[i];
        if (!b || !isRowEqual(a, b)) {
          differs = true;
          break;
        }
      }
    }
    if (differs) {
      debug('derive changed rows; replace local rows', {
        incoming,
        rows,
      });
      rowsRef.current = incoming;
      setRows(incoming);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  );

  const items = useMemo(
    () => rows.map((_, index) => ({ id: idsRef.current[index] || `${index}`, index })),
    [rows]
  );

  const flushCommit = useCallback(
    (next: T[]) => {
      internalUpdateRef.current = true;
      commitVersionRef.current += 1;
      // short cooldown to absorb multi-phase external updates caused by a single commit
      lockUntilTsRef.current = Date.now() + 250;
      debug('serialize rows to parent', {
        next,
        commitVersion: commitVersionRef.current,
      });
      serialize(next);
    },
    [debug, serialize]
  );

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      if (!over || active.id === over.id) return;
      const oldIndex = idsRef.current.indexOf(String(active.id));
      const newIndex = idsRef.current.indexOf(String(over.id));
      if (oldIndex === -1 || newIndex === -1) return;
      const nextRows = arrayMove(rowsRef.current, oldIndex, newIndex);
      debug('drag rows', { oldIndex, newIndex, nextRows });
      idsRef.current = arrayMove(idsRef.current, oldIndex, newIndex);
      rowsRef.current = nextRows;
      setRows(nextRows);
      flushCommit(nextRows);
    },
    [debug, flushCommit]
  );

  const append = useCallback(
    (item: T) => {
      const newId = `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
      const next = [...rowsRef.current, item];
      debug('append row', { item, next });
      idsRef.current.push(newId);
      rowsRef.current = next;
      setRows(next);
      flushCommit(next);
    },
    [debug, flushCommit]
  );

  const removeAt = useCallback(
    (index: number) => {
      const current = rowsRef.current;
      if (index < 0 || index >= current.length) return;
      const next = current.filter((_, i) => i !== index);
      debug('remove row', { index, removed: current[index], next });
      idsRef.current.splice(index, 1);
      rowsRef.current = next;
      setRows(next);
      flushCommit(next);
    },
    [debug, flushCommit]
  );

  const updateAt = useCallback(
    (index: number, updater: (cur: T) => T) => {
      const current = rowsRef.current;
      if (index < 0 || index >= current.length) return;

      let changed = false;
      const next = current.map((item, itemIndex) => {
        if (itemIndex !== index) return item;
        const updated = updater(item);
        if (!Object.is(updated, item)) {
          changed = true;
        }
        return updated;
      });

      if (!changed) {
        debug('update row skipped; updater returned same item', { index, item: current[index] });
        return;
      }
      debug('update row', { index, previous: current[index], nextRow: next[index], next });
      rowsRef.current = next;
      setRows(next);
      flushCommit(next);
    },
    [debug, flushCommit]
  );

  return { rows, items, sensors, handleDragEnd, append, removeAt, updateAt, setRows };
}
