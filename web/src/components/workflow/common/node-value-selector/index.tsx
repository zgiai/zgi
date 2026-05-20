'use client';

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useT } from '@/i18n';
import { Label } from '@/components/ui/label';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip';
import { Button } from '@/components/ui/button';
import type { UpstreamExportItem } from '../../store/store';
import type { WorkflowVariable } from '../../store/type';
import { cn } from '@/lib/utils';
import { ChevronDown } from 'lucide-react';
import {
  WORKFLOW_CONTROL_COMPACT_CLASS,
  WORKFLOW_FIELD_LABEL_COMPACT_CLASS,
} from '../form-density';
import { useWorkflowVariableCatalog } from '../../hooks';
import { buildVariableSelectionKey, normalizeVariableSelector } from '../variable-reference';

// Primitive type alias derived from workflow variable type
type PrimitiveType = WorkflowVariable['type'];

interface NodeValueSelectorProps {
  /** Current node id where we are selecting a value for */
  nodeId: string | null | undefined;
  /** Selected value as [sourceNodeId, variableKey] or [sourceNodeId, variableKey, subField, ...] */
  value?: string[];
  /**
   * Change handler returns selected values.
   * - key: top-level variable key (backward compatibility)
   * - path: full path [key, subField, ...] for multi-level selection
   * - valuePath: complete array [sourceId, key, subField, ...] ready to use as value
   */
  onChange?: (val: {
    sourceId: string;
    key: string;
    path?: string[];
    valuePath: string[];
    type: PrimitiveType;
  }) => void;
  className?: string;
  /** Optional field label */
  label?: string;
  /** When true, only allow Start node variables */
  startOnly?: boolean;
  /** When true, only show writable variables (e.g., iteration-start item/index) */
  writableOnly?: boolean;
  /** Placeholder for the select */
  placeholder?: string;
  /** Force disable control, e.g., in read-only mode */
  disabled?: boolean;
  /** Optional override for upstream groups (use when not selecting true upstreams) */
  upstreamsOverride?: UpstreamExportItem[];
  /** Optional filter to restrict variable types */
  typeFilter?: (type: PrimitiveType) => boolean;
  /** Pin certain groups to the top */
  pinGroupsFirst?: (group: UpstreamExportItem) => boolean;
  /** Open dropdown automatically on mount (once) */
  autoOpen?: boolean;
  /** Hide system variables (sys.*) entirely */
  hideSystem?: boolean;
  /** Shared density variant for workflow side panels */
  density?: 'default' | 'compact';
  /** Optional className for the trigger button */
  triggerClassName?: string;
  /** Optional className for the field label */
  labelClassName?: string;
}

import { ValueSelectorMenu, type ValueSelectorMenuProps } from './value-selector-menu';

export { ValueSelectorMenu };
export type { ValueSelectorMenuProps };

/**
 * NodeValueSelector
 * - Presents upstream variables for the given nodeId
 * - Grouped by upstream nodes
 * - Emits payload including selected variable's type
 * - Supports cascading sub-menus for structured types (file, aggregator groups)
 */
const NodeValueSelector: React.FC<NodeValueSelectorProps> = ({
  nodeId,
  value,
  onChange,
  className,
  label = '',
  startOnly = false,
  placeholder = '',
  disabled: disabledProp = false,
  upstreamsOverride,
  typeFilter,
  pinGroupsFirst,
  writableOnly = false,
  autoOpen = false,
  hideSystem = false,
  density = 'default',
  triggerClassName,
  labelClassName,
}) => {
  const t = useT();
  const { selectionIndex, totalSelectableCount } = useWorkflowVariableCatalog({
    nodeId,
    startOnly,
    writableOnly,
    upstreamsOverride,
    typeFilter,
    pinGroupsFirst: pinGroupsFirst
      ? group =>
          pinGroupsFirst({
            nodeId: group.sourceId,
            nodeType: group.sourceNodeType as UpstreamExportItem['nodeType'],
            nodeTitle: group.sourceNodeTitle,
            variables: [],
          })
      : undefined,
    hideSystem,
  });
  const normalizedValue = useMemo(() => normalizeVariableSelector(value), [value]);
  const selectedOption = useMemo(() => {
    if (!normalizedValue) return null;
    return selectionIndex.get(buildVariableSelectionKey(normalizedValue) || '');
  }, [normalizedValue, selectionIndex]);

  // Build selected label for trigger
  const { selectedLabel, selectedLabelText } = useMemo(() => {
    if (!selectedOption) return { selectedLabel: '', selectedLabelText: '' };
    const label = (
      <>
        <span className="mr-1">{selectedOption.sourceTitle}</span>
        <span className="text-highlight">
          ({selectedOption.displayPath})
        </span>
      </>
    );
    const text = selectedOption.displayText;
    return { selectedLabel: label, selectedLabelText: text };
  }, [selectedOption]);

  const invalidSelected = useMemo(() => {
    if (!normalizedValue) return false;
    return !selectedOption;
  }, [normalizedValue, selectedOption]);

  const [open, setOpen] = useState(false);
  const hasAutoOpenedRef = useRef(false);
  const initialAutoOpen = useRef<boolean>(autoOpen);

  const disabled = disabledProp || !nodeId || totalSelectableCount === 0;
  const disabledNoOptions = !disabledProp && !!nodeId && totalSelectableCount === 0;
  const isCompact = density === 'compact';

  useEffect(() => {
    if (!hasAutoOpenedRef.current && !disabled && initialAutoOpen.current) {
      hasAutoOpenedRef.current = true;
      setOpen(true);
    }
  }, [disabled]);

  return (
    <div className={cn('space-y-2', className)}>
      {label ? (
        <Label
          className={cn(isCompact && WORKFLOW_FIELD_LABEL_COMPACT_CLASS, labelClassName)}
        >
          {label}
        </Label>
      ) : null}
      <DropdownMenu open={open} onOpenChange={setOpen}>
        {disabledNoOptions ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <DropdownMenuTrigger asChild disabled={disabled}>
                <Button
                  variant="outline"
                  role="combobox"
                  aria-label={label}
                  disabled={disabled}
                  className={cn(
                    'min-w-0 w-full justify-between overflow-hidden font-normal',
                    !selectedLabel && 'text-muted-foreground',
                    isCompact && WORKFLOW_CONTROL_COMPACT_CLASS,
                    triggerClassName
                  )}
                >
                  {selectedLabel || <span>{placeholder}</span>}
                  <ChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                </Button>
              </DropdownMenuTrigger>
            </TooltipTrigger>
            <TooltipContent side="top">
              <p>{t('nodes.validation.noMatchingVariables')}</p>
            </TooltipContent>
          </Tooltip>
        ) : (
          <Tooltip>
            <TooltipTrigger asChild disabled={disabled}>
              <DropdownMenuTrigger asChild disabled={disabled}>
                <Button
                  variant="outline"
                  role="combobox"
                  aria-label={label}
                  disabled={disabled}
                  aria-invalid={invalidSelected || undefined}
                  className={cn(
                    'min-w-0 w-full justify-between overflow-hidden font-normal',
                    invalidSelected && 'border-destructive',
                    !selectedLabel && 'text-muted-foreground',
                    isCompact && WORKFLOW_CONTROL_COMPACT_CLASS,
                    triggerClassName
                  )}
                >
                  {selectedLabel ? (
                    <span className="truncate text-start flex-1">{selectedLabel}</span>
                  ) : invalidSelected ? (
                    <span className="text-destructive">
                      {t('nodes.validation.invalidVariable')}
                    </span>
                  ) : (
                    <span>{placeholder}</span>
                  )}
                  <ChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                </Button>
              </DropdownMenuTrigger>
            </TooltipTrigger>
            {selectedLabel && (
              <TooltipContent side="top" className="max-w-sm break-all">
                {selectedLabelText}
              </TooltipContent>
            )}
          </Tooltip>
        )}
        <DropdownMenuContent
          className="w-[var(--radix-dropdown-menu-trigger-width)] max-h-[300px] overflow-auto py-0 min-w-[260px]"
          align="start"
        >
          <ValueSelectorMenu
            nodeId={nodeId}
            value={value}
            onSelect={onChange}
            onClose={() => setOpen(false)}
            startOnly={startOnly}
            writableOnly={writableOnly}
            upstreamsOverride={upstreamsOverride}
            typeFilter={typeFilter}
            pinGroupsFirst={pinGroupsFirst}
            hideSystem={hideSystem}
          />
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
};

export default React.memo(NodeValueSelector);
