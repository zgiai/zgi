import React, { useCallback, useEffect, useMemo, useRef } from 'react';
import type { CodeNodeData, OutputVariable } from '../../../../store/type';
import {
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import { arrayMove, sortableKeyboardCoordinates } from '@dnd-kit/sortable';
import { sanitizeIdentifier, ensureUniqueIdentifier } from '@/utils/validation';
import { useNodeData } from '../../../../hooks/use-node-data';
import { useNodeDataUpdate } from '../../../../hooks/use-node-data-update';

export interface OutputRowModel {
  key: string;
  type: OutputVariable['type'];
  children: unknown | null;
}

export interface DndItem {
  id: string;
  index: number;
}

export interface UseCodeOutputsResult {
  rows: OutputRowModel[];
  items: DndItem[];
  sensors: ReturnType<typeof useSensors>;
  handleAddOutput: () => void;
  handleRemoveOutput: (key: string) => void;
  handleOutputKeyChangeAtIndex: (index: number, newKey: string) => void;
  handleOutputTypeChange: (key: string, type: OutputVariable['type']) => void;
  handleDragEnd: (event: DragEndEvent) => void;
}

/**
 * Store-aware hook for managing Code node outputs.
 * Automatically reads from and writes to the workflow store.
 */
export function useCodeOutputs(nodeId: string): UseCodeOutputsResult {
  const nodeData = useNodeData<CodeNodeData>(nodeId);
  const updateNodeData = useNodeDataUpdate<CodeNodeData>(nodeId);

  const idsRef = useRef<string[]>([]);
  const internalStructureUpdateRef = useRef(false);

  const deriveRowsFromNode = useCallback((data: CodeNodeData | undefined) => {
    if (!data) return [];
    const outputs = (data.outputs || {}) as CodeNodeData['outputs'];
    const orders =
      data.outputKeyOrders && data.outputKeyOrders.length > 0
        ? data.outputKeyOrders
        : Object.keys(outputs || {});
    return orders.map((key: string) => ({
      key,
      type: (outputs?.[key]?.type as OutputVariable['type']) || 'string',
      children: outputs?.[key]?.children ?? null,
    }));
  }, []);

  const [rows, setRows] = React.useState<OutputRowModel[]>(() => deriveRowsFromNode(nodeData));

  useEffect(() => {
    if (idsRef.current.length < rows.length) {
      const missing = rows.length - idsRef.current.length;
      for (let index = 0; index < missing; index += 1) {
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
  }, [rows.length]);

  useEffect(() => {
    if (internalStructureUpdateRef.current) {
      internalStructureUpdateRef.current = false;
      return;
    }
    const next = deriveRowsFromNode(nodeData);
    const sameLength = next.length === rows.length;
    let differs = !sameLength;
    if (!differs) {
      for (let index = 0; index < next.length; index += 1) {
        const left = next[index];
        const right = rows[index];
        if (!right || left.key !== right.key || left.type !== right.type) {
          differs = true;
          break;
        }
      }
    }
    if (differs) setRows(next);
  }, [deriveRowsFromNode, nodeData, rows]);

  const serializeAndCommit = useCallback(
    (nextRows: OutputRowModel[]) => {
      const outputs: Record<string, { type: OutputVariable['type']; children: unknown | null }> =
        {};
      const outputKeyOrders: string[] = [];
      nextRows.forEach(row => {
        outputKeyOrders.push(row.key);
        outputs[row.key] = { type: row.type, children: row.children ?? null };
      });
      internalStructureUpdateRef.current = true;
      updateNodeData({
        outputs: outputs as unknown as CodeNodeData['outputs'],
        outputKeyOrders,
      });
    },
    [updateNodeData]
  );

  const items = useMemo(
    () => rows.map((row, index) => ({ id: idsRef.current[index] || `${row.key}-${index}`, index })),
    [rows]
  );

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  );

  const handleAddOutput = useCallback(() => {
    const existing = new Set(rows.map(row => row.key));
    let suffix = rows.length + 1;
    let candidate = `result${suffix}`;
    while (existing.has(candidate)) {
      suffix += 1;
      candidate = `result${suffix}`;
    }
    const nextRows: OutputRowModel[] = [
      ...rows,
      { key: candidate, type: 'string' as OutputVariable['type'], children: null },
    ];
    idsRef.current.push(`${Date.now()}-${Math.random().toString(36).slice(2, 10)}`);
    setRows(nextRows);
    serializeAndCommit(nextRows);
  }, [rows, serializeAndCommit]);

  const handleRemoveOutput = useCallback(
    (key: string) => {
      const index = rows.findIndex(row => row.key === key);
      if (index === -1) return;
      const nextRows = rows.filter((_, rowIndex) => rowIndex !== index);
      idsRef.current.splice(index, 1);
      setRows(nextRows);
      serializeAndCommit(nextRows);
    },
    [rows, serializeAndCommit]
  );

  const handleOutputKeyChangeAtIndex = useCallback(
    (index: number, newKey: string) => {
      const trimmed = sanitizeIdentifier((newKey || '').trim());
      const current = rows[index];
      if (!current) return;
      if (!trimmed || trimmed === current.key) return;
      const names = rows.filter((_, rowIndex) => rowIndex !== index).map(row => row.key);
      const unique = ensureUniqueIdentifier(trimmed, names);
      const nextRows = rows.map((row, rowIndex) =>
        rowIndex === index ? { ...row, key: unique } : row
      );
      setRows(nextRows);
      serializeAndCommit(nextRows);
    },
    [rows, serializeAndCommit]
  );

  const handleOutputTypeChange = useCallback(
    (key: string, type: OutputVariable['type']) => {
      const nextRows = rows.map(row => (row.key === key ? { ...row, type } : row));
      setRows(nextRows);
      serializeAndCommit(nextRows);
    },
    [rows, serializeAndCommit]
  );

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      if (!over || active.id === over.id) return;
      const oldIndex = idsRef.current.indexOf(String(active.id));
      const newIndex = idsRef.current.indexOf(String(over.id));
      if (oldIndex === -1 || newIndex === -1) return;
      const nextRows = arrayMove(rows, oldIndex, newIndex);
      idsRef.current = arrayMove(idsRef.current, oldIndex, newIndex);
      setRows(nextRows);
      serializeAndCommit(nextRows);
    },
    [rows, serializeAndCommit]
  );

  return {
    rows,
    items,
    sensors,
    handleAddOutput,
    handleRemoveOutput,
    handleOutputKeyChangeAtIndex,
    handleOutputTypeChange,
    handleDragEnd,
  };
}
