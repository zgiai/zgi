'use client';

import React, { useCallback } from 'react';
import { Trash2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import NodeValueSelector from '@/components/workflow/common/node-value-selector';
import SortableListSection from '@/components/workflow/common/sortable-list/sortable-list-section';
import { useStableSortableList } from '@/components/workflow/common/sortable-list/use-stable-sortable-list';
import type { WorkflowVariable } from '@/components/workflow/store/type';
import { logWorkflowEditDebug } from '@/components/workflow/utils/edit-debug';
import { cn } from '@/lib/utils';
import { isValidIdentifier } from '@/utils/validation';

import { WorkflowIdentifierInput } from './identifier-input';

interface VariableSelectorPayload {
  sourceId: string;
  key: string;
  path?: string[];
  valuePath: string[];
  type: WorkflowVariable['type'];
}

export interface VariableBindingEditorLabels {
  title: string;
  addLabel: string;
  emptyText: string;
  namePlaceholder: string;
  selectorPlaceholder?: string;
  removeLabel: (index: number) => string;
}

export interface VariableBindingEditorAdapter<TRow> {
  createRow: (rows: TRow[]) => TRow;
  isRowEqual: (a: TRow, b: TRow) => boolean;
  getName: (row: TRow) => string;
  setName: (row: TRow, name: string) => TRow;
  getSelector: (row: TRow) => string[] | undefined;
  applySelectorChange: (args: {
    row: TRow;
    rows: TRow[];
    index: number;
    payload: VariableSelectorPayload;
  }) => TRow;
  normalizeRowOnBlur?: (args: { row: TRow; rows: TRow[]; index: number }) => TRow;
  isNameInvalid?: (row: TRow) => boolean;
}

export interface VariableBindingEditorProps<TRow> {
  rows: TRow[];
  onChange: (rows: TRow[]) => void;
  labels: VariableBindingEditorLabels;
  adapter: VariableBindingEditorAdapter<TRow>;
  nodeId: string | null | undefined;
  readOnly?: boolean;
  className?: string;
  debugLabel?: string;
}

function areStringArraysEqual(left: string[] | undefined, right: string[] | undefined): boolean {
  const lhs = Array.isArray(left) ? left : [];
  const rhs = Array.isArray(right) ? right : [];

  if (lhs.length !== rhs.length) return false;
  return lhs.every((item, index) => item === rhs[index]);
}

function isDeleteShortcut(event: React.KeyboardEvent<HTMLElement>): boolean {
  return event.key === 'Delete' || event.key === 'Backspace';
}

function isClipboardShortcut(event: React.KeyboardEvent<HTMLElement>): boolean {
  if (!event.ctrlKey && !event.metaKey) return false;
  const key = event.key.toLowerCase();
  return key === 'c' || key === 'v' || key === 'x';
}

/**
 * @component VariableBindingEditor
 * @category Common
 * @status Beta
 * @description Shared sortable editor for rows that bind a local variable name to an upstream workflow variable.
 * @usage Use in node side panels that support add, remove, reorder, identifier editing, and upstream-variable selection.
 * @example
 * <VariableBindingEditor rows={rows} onChange={setRows} labels={labels} adapter={adapter} nodeId={nodeId} />
 */
export function VariableBindingEditor<TRow>({
  rows,
  onChange,
  labels,
  adapter,
  nodeId,
  readOnly = false,
  className,
  debugLabel,
}: VariableBindingEditorProps<TRow>) {
  const debug = useCallback(
    (message: string, data?: Record<string, unknown>) => {
      logWorkflowEditDebug(debugLabel ? `${debugLabel}:binding` : undefined, message, data);
    },
    [debugLabel]
  );

  const {
    rows: draftRows,
    items,
    sensors,
    handleDragEnd,
    append,
    removeAt,
    updateAt,
  } = useStableSortableList<TRow>({
    derive: () => rows || [],
    deps: [rows],
    isRowEqual: adapter.isRowEqual,
    debugLabel,
    serialize: nextRows => {
      if (readOnly) return;
      debug('serialize rows to onChange', { nextRows });
      onChange(nextRows);
    },
  });

  const handleAdd = useCallback(() => {
    if (readOnly) return;
    const row = adapter.createRow(draftRows);
    debug('add row', { row, draftRows });
    append(row);
  }, [adapter, append, debug, draftRows, readOnly]);

  const handleRemove = useCallback(
    (index: number) => {
      if (readOnly) return;
      debug('remove row', { index, row: draftRows[index], draftRows });
      removeAt(index);
    },
    [debug, draftRows, readOnly, removeAt]
  );

  const handleNameCommit = useCallback(
    (index: number, value: string) => {
      if (readOnly) return;
      debug('commit row name', { index, value, row: draftRows[index] });
      updateAt(index, row => adapter.setName(row, value));
    },
    [adapter, debug, draftRows, readOnly, updateAt]
  );

  const handleNameBlur = useCallback(
    (index: number) => {
      if (readOnly || !adapter.normalizeRowOnBlur) return;
      debug('blur normalize row', { index, row: draftRows[index], draftRows });
      updateAt(index, row => adapter.normalizeRowOnBlur?.({ row, rows: draftRows, index }) ?? row);
    },
    [adapter, debug, draftRows, readOnly, updateAt]
  );

  const handleSelectorChange = useCallback(
    (index: number, payload: VariableSelectorPayload) => {
      if (readOnly) return;
      debug('selector change', { index, payload, row: draftRows[index], draftRows });
      updateAt(index, row =>
        adapter.applySelectorChange({
          row,
          rows: draftRows,
          index,
          payload,
        })
      );
    },
    [adapter, debug, draftRows, readOnly, updateAt]
  );

  return (
    <div className={cn('space-y-3', className)}>
      <SortableListSection
        title={labels.title}
        addLabel={labels.addLabel}
        emptyText={labels.emptyText}
        isReadOnly={readOnly}
        items={items}
        sensors={sensors}
        onDragEnd={handleDragEnd}
        onAdd={handleAdd}
        renderRow={index => {
          const row = draftRows[index];
          const name = adapter.getName(row);
          const isNameInvalid = adapter.isNameInvalid
            ? adapter.isNameInvalid(row)
            : name.trim().length > 0 && !isValidIdentifier(name);

          return (
            <div
              className="flex items-center gap-1 min-w-0"
              onKeyDownCapture={event => {
                if (isDeleteShortcut(event) || isClipboardShortcut(event)) {
                  event.stopPropagation();
                }
              }}
            >
              <div className="flex-1 min-w-0">
                <WorkflowIdentifierInput
                  initial={name}
                  onCommit={value => handleNameCommit(index, value)}
                  onBlurNormalize={() => handleNameBlur(index)}
                  placeholder={labels.namePlaceholder}
                  invalid={isNameInvalid}
                  disabled={readOnly}
                  debugLabel={debugLabel ? `${debugLabel}:row-${index}:name` : undefined}
                />
              </div>

              <div className="flex-1 min-w-0">
                <NodeValueSelector
                  nodeId={nodeId}
                  value={adapter.getSelector(row)}
                  onChange={payload => handleSelectorChange(index, payload)}
                  placeholder={labels.selectorPlaceholder}
                  disabled={readOnly}
                />
              </div>

              <Button
                variant="ghost"
                isIcon
                onClick={() => handleRemove(index)}
                disabled={readOnly}
                className="shrink-0"
                aria-label={labels.removeLabel(index)}
                title={labels.removeLabel(index)}
              >
                <Trash2 className="w-4 h-4" />
              </Button>
            </div>
          );
        }}
      />
    </div>
  );
}

export function selectorsEqual(left: string[] | undefined, right: string[] | undefined) {
  return areStringArraysEqual(left, right);
}
