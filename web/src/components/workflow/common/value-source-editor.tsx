'use client';

import React from 'react';
import { Tabs } from '@/components/ui/tabs';
import NodeValueSelector from './node-value-selector';
import { cn } from '@/lib/utils';
import type { WorkflowVariable } from '../store/type';
import {
  WorkflowCompactFieldHint,
  WorkflowCompactFieldLabel,
  WorkflowCompactTabsList,
  WorkflowCompactTabsTrigger,
} from './compact-form';

interface ValueSourceEditorProps {
  nodeId: string;
  mode: 'constant' | 'variable';
  onModeChange?: (mode: 'constant' | 'variable') => void;
  constantEditor: React.ReactNode;
  variableValue?: string[];
  onVariableChange: (payload: {
    sourceId: string;
    key: string;
    valuePath: string[];
    type: WorkflowVariable['type'];
  }) => void;
  variableTypeFilter?: (type: WorkflowVariable['type']) => boolean;
  variablePlaceholder?: string;
  disabled?: boolean;
  density?: 'default' | 'compact';
  layout?: 'stacked' | 'inline';
  showModeSwitcher?: boolean;
  label?: string;
  hint?: string;
  className?: string;
  writableOnly?: boolean;
  constantLabel?: string;
  variableLabel?: string;
}

/**
 * @component ValueSourceEditor
 * @category Common
 * @status Beta
 * @description Shared editor shell for fields that can switch between constant values and workflow variables.
 * @usage Use in managers like assigner/loop/tool to standardize mode switch, selector filtering, and hint copy.
 */
export function ValueSourceEditor({
  nodeId,
  mode,
  onModeChange,
  constantEditor,
  variableValue,
  onVariableChange,
  variableTypeFilter,
  variablePlaceholder,
  disabled = false,
  density = 'default',
  layout = 'stacked',
  showModeSwitcher = true,
  label,
  hint,
  className,
  writableOnly = false,
  constantLabel = 'Constant',
  variableLabel = 'Variable',
}: ValueSourceEditorProps) {
  const isCompact = density === 'compact';
  const isInline = layout === 'inline';

  const modeSwitcher =
    showModeSwitcher && onModeChange ? (
      <Tabs value={mode} onValueChange={value => onModeChange(value as 'constant' | 'variable')}>
        <WorkflowCompactTabsList className="w-fit">
          <WorkflowCompactTabsTrigger value="constant" disabled={disabled}>
            <span>{constantLabel}</span>
          </WorkflowCompactTabsTrigger>
          <WorkflowCompactTabsTrigger value="variable" disabled={disabled}>
            <span>{variableLabel}</span>
          </WorkflowCompactTabsTrigger>
        </WorkflowCompactTabsList>
      </Tabs>
    ) : null;

  const valueEditor =
    mode === 'variable' ? (
      <NodeValueSelector
        nodeId={nodeId}
        value={variableValue}
        onChange={onVariableChange}
        typeFilter={variableTypeFilter}
        disabled={disabled}
        writableOnly={writableOnly}
        density={isCompact ? 'compact' : 'default'}
        className={isCompact ? 'space-y-1' : undefined}
        placeholder={variablePlaceholder}
      />
    ) : (
      constantEditor
    );

  return (
    <div className={cn('space-y-2', className)}>
      {label ? <WorkflowCompactFieldLabel>{label}</WorkflowCompactFieldLabel> : null}

      {isInline ? (
        <div className="flex flex-wrap items-start gap-2">
          <div className="shrink-0">{modeSwitcher}</div>
          <div className="min-w-0 basis-[180px] flex-1">{valueEditor}</div>
        </div>
      ) : (
        <div className="space-y-2">
          {modeSwitcher}
          {valueEditor}
        </div>
      )}

      {hint ? <WorkflowCompactFieldHint>{hint}</WorkflowCompactFieldHint> : null}
    </div>
  );
}

export default ValueSourceEditor;
