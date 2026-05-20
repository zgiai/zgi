import { useCallback, useEffect, useRef } from 'react';
import type { CodeNodeData } from '../../../../store/type';
import type { OutputVariable } from '../../../end/config';
import type { DragEndEvent, SensorDescriptor } from '@dnd-kit/core';
import { useStableSortableList } from '../../../../common/sortable-list/use-stable-sortable-list';
import { sanitizeIdentifier, ensureUniqueIdentifier } from '@/utils/validation';
import { useNodeData } from '../../../../hooks/use-node-data';
import { useNodeDataUpdate } from '../../../../hooks/use-node-data-update';

export interface UseCodeInputsResult {
  varRows: Required<CodeNodeData>['variables'];
  items: Array<{ id: string; index: number }>;
  sensors: Array<SensorDescriptor<object>>;
  handleDragEnd: (event: DragEndEvent) => void;
  handleAddVar: () => void;
  handleRemoveVar: (index: number) => void;
  handleVarNameChange: (index: number, name: string) => void;
  handleVarSelectorChange: (
    index: number,
    payload: { sourceId: string; key: string; valuePath: string[]; type: string }
  ) => void;
  varKeyAt: (index: number) => string;
}

/**
 * Store-aware hook for managing Code node input variables.
 * Automatically reads from and writes to the workflow store.
 */
export function useCodeInputs(nodeId: string): UseCodeInputsResult {
  type VarRowModel = Required<CodeNodeData>['variables'][number];

  // Store-aware: read data directly from store
  const nodeData = useNodeData<CodeNodeData>(nodeId);
  const updateNodeData = useNodeDataUpdate<CodeNodeData>(nodeId);

  // Guard flag to distinguish intentional deletion vs accidental shrink
  const isRemovingRef = useRef(false);
  const lastVarsRef = useRef<VarRowModel[]>(
    Array.isArray(nodeData?.variables) ? (nodeData?.variables ?? []) : []
  );

  const {
    rows: varRows,
    items,
    sensors,
    handleDragEnd,
    append,
    removeAt,
    updateAt,
  } = useStableSortableList<VarRowModel>({
    // Always derive a normalized deep copy to avoid shared references
    derive: () =>
      (Array.isArray(nodeData?.variables) ? (nodeData?.variables ?? []) : []).map(v => ({
        variable: typeof v.variable === 'string' ? v.variable : '',
        value_selector: Array.isArray(v.value_selector) ? [...v.value_selector] : [],
        value_type:
          (v.value_type as OutputVariable['type']) || ('string' as OutputVariable['type']),
      })),
    deps: [nodeData?.variables],
    isRowEqual: (a: VarRowModel, b: VarRowModel) => {
      if (a.variable !== b.variable || a.value_type !== b.value_type) return false;
      const aSel = Array.isArray(a.value_selector) ? a.value_selector : [];
      const bSel = Array.isArray(b.value_selector) ? b.value_selector : [];
      if (aSel.length !== bSel.length) return false;
      return aSel.every((v, i) => v === bSel[i]);
    },
    // Commit a deep copy with normalized shapes; prevent accidental shrink unless removing
    serialize: (next: VarRowModel[]) => {
      const normalizedNext = next.map(v => ({
        variable: typeof v.variable === 'string' ? v.variable : '',
        value_selector: Array.isArray(v.value_selector) ? [...v.value_selector] : [],
        value_type:
          (v.value_type as OutputVariable['type']) || ('string' as OutputVariable['type']),
      }));
      const prev = Array.isArray(lastVarsRef.current) ? lastVarsRef.current : [];
      const shouldPreventShrink = !isRemovingRef.current && normalizedNext.length < prev.length;
      const committed = shouldPreventShrink
        ? prev.map(
            (prevItem, i) =>
              normalizedNext[i] ?? {
                variable: typeof prevItem.variable === 'string' ? prevItem.variable : '',
                value_selector: Array.isArray(prevItem.value_selector)
                  ? [...prevItem.value_selector]
                  : [],
                value_type:
                  (prevItem.value_type as OutputVariable['type']) ||
                  ('string' as OutputVariable['type']),
              }
          )
        : normalizedNext;
      updateNodeData({ variables: committed });
      // keep last snapshot in sync
      lastVarsRef.current = committed as VarRowModel[];
      if (isRemovingRef.current) isRemovingRef.current = false;
    },
  });

  // Track last updated index for targeted commits (avoid unintended array rewrites)
  const lastUpdateIndexRef = useRef<number | null>(null);
  const updateAtIndex = useCallback(
    (index: number, updater: (cur: VarRowModel) => VarRowModel) => {
      lastUpdateIndexRef.current = index;
      updateAt(index, updater);
    },
    [updateAt]
  );

  // Sync ref whenever rows (source of truth for UI) change
  useEffect(() => {
    lastVarsRef.current = varRows as VarRowModel[];
  }, [varRows]);

  const handleAddVar = useCallback(() => {
    append({
      variable: `var${varRows.length + 1}`,
      value_selector: [],
      value_type: 'string' as OutputVariable['type'],
    });
  }, [append, varRows]);

  const handleRemoveVar = useCallback(
    (index: number) => {
      isRemovingRef.current = true;
      removeAt(index);
    },
    [removeAt]
  );

  const handleVarNameChange = useCallback(
    (index: number, name: string) => {
      const trimmed = sanitizeIdentifier((name || '').trim());
      const names = (varRows || []).map(v => v.variable || '');
      const exclude = varRows[index]?.variable || undefined;
      const unique = ensureUniqueIdentifier(trimmed, names, exclude);
      updateAtIndex(index, (cur: VarRowModel) =>
        cur.variable === unique ? cur : { ...cur, variable: unique }
      );
    },
    [updateAtIndex, varRows]
  );

  const handleVarSelectorChange = useCallback(
    (
      index: number,
      payload: { sourceId: string; key: string; valuePath: string[]; type: string }
    ) => {
      // Use functional update and preserve other rows; explicitly normalize selector & type
      updateAtIndex(index, (cur: VarRowModel) => {
        const nextSelector = payload.valuePath;
        const nextType = (payload.type ?? cur.value_type) as OutputVariable['type'];
        if (
          Array.isArray(cur.value_selector) &&
          cur.value_selector.length === nextSelector.length &&
          cur.value_selector.every((v, i) => v === nextSelector[i]) &&
          cur.value_type === nextType
        ) {
          return cur;
        }
        return {
          ...cur,
          value_selector: nextSelector,
          value_type: nextType,
        };
      });
    },
    [updateAtIndex]
  );

  const varKeyAt = useCallback((index: number) => items[index]?.id || String(index), [items]);

  return {
    varRows,
    items,
    sensors,
    handleDragEnd,
    handleAddVar,
    handleRemoveVar,
    handleVarNameChange,
    handleVarSelectorChange,
    varKeyAt,
  };
}
