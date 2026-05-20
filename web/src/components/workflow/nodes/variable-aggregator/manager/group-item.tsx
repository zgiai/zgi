'use client';

import React from 'react';
import type { VariableAggregatorGroup, PrimitiveType, VarSelector } from '../config';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import NodeValueSelector from '../../../common/node-value-selector';
import VariableSelector from '../../../common/variable-selector';
import { useT } from '@/i18n';
import { sanitizeIdentifier, isValidIdentifier } from '@/utils/validation';
import { Trash2, Plus } from 'lucide-react';

/**
 * GroupNameField: pure local input, commit only on blur to avoid nested updates
 */
const GroupNameField: React.FC<{
  initial: string;
  onBlurNormalize: (value: string) => void;
  placeholder: string;
  disabled?: boolean;
  className?: string;
}> = React.memo(({ initial, onBlurNormalize, placeholder, disabled, className }) => {
  const [value, setValue] = React.useState<string>(initial || '');
  React.useEffect(() => {
    setValue(initial || '');
  }, [initial]);
  return (
    <Input
      value={value}
      onChange={e => setValue(sanitizeIdentifier(e.target.value).slice(0, 20))}
      onBlur={() => onBlurNormalize(value)}
      placeholder={placeholder}
      disabled={disabled}
      className={className}
    />
  );
});
GroupNameField.displayName = 'GroupNameField';

interface GroupItemProps {
  nodeId: string;
  group: VariableAggregatorGroup;
  groupIndex: number;
  isReadOnly: boolean;
  canDelete: boolean;
  // Handlers
  onNameBlur: (index: number, value: string) => void;
  onRemoveGroup: (index: number) => void;
  onRemoveVariable: (groupIndex: number, varIndex: number) => void;
  onChangeSelector: (
    groupIndex: number,
    varIndex: number,
    payload: { sourceId: string; key: string; valuePath: string[]; type: PrimitiveType }
  ) => void;
  onSelectVariable: (
    groupIndex: number,
    payload: { sourceId: string; key: string; valuePath: string[]; type: PrimitiveType }
  ) => void;
}

/**
 * Single group item in group mode
 * Contains group name input, variable list, and variable picker
 */
const GroupItem: React.FC<GroupItemProps> = ({
  nodeId,
  group,
  groupIndex,
  isReadOnly,
  canDelete,
  onNameBlur,
  onRemoveGroup,
  onRemoveVariable,
  onChangeSelector,
  onSelectVariable,
}) => {
  const t = useT('nodes');

  return (
    <div className="border rounded p-2 space-y-2">
      <div className="flex items-center gap-2">
        <GroupNameField
          initial={group.group_name}
          onBlurNormalize={val => onNameBlur(groupIndex, val)}
          placeholder={t('variableAggregator.manager.groupNamePlaceholder')}
          disabled={isReadOnly}
          className="flex-1"
        />
        <div className="ml-auto flex items-center gap-1">
          <VariableSelector
            nodeId={nodeId}
            onSelect={p => onSelectVariable(groupIndex, p)}
            hideSystem
            typeFilter={
              group.output_type ? tpe => tpe === (group.output_type as PrimitiveType) : undefined
            }
          >
            <Button variant="ghost" isIcon disabled={isReadOnly}>
              <Plus className="h-4 w-4" />
            </Button>
          </VariableSelector>
          <Button
            variant="ghost"
            isIcon
            onClick={() => onRemoveGroup(groupIndex)}
            disabled={!canDelete || isReadOnly}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>
      {!isValidIdentifier(group.group_name || '') ? (
        <div className="text-[11px] text-destructive">
          {t('variableAggregator.manager.groupNameInvalid')}
        </div>
      ) : null}
      <div className="space-y-2">
        {group.variables.length === 0 ? (
          <div className="text-xs text-muted-foreground">
            {t('variableAggregator.manager.noVariables')}
          </div>
        ) : null}
        {group.variables.map((sel, vi) => (
          <div key={`${group.groupId}-${vi}`} className="flex items-center gap-2">
            <NodeValueSelector
              nodeId={nodeId}
              value={Array.isArray(sel) && sel.length >= 2 ? sel : undefined}
              onChange={p => onChangeSelector(groupIndex, vi, p)}
              typeFilter={
                vi === 0
                  ? group.output_type
                    ? tpe => tpe === (group.output_type as PrimitiveType)
                    : undefined
                  : tpe => tpe === (group.output_type as PrimitiveType)
              }
              className="grow"
              hideSystem
              disabled={isReadOnly}
            />
            <Button
              variant="ghost"
              isIcon
              onClick={() => onRemoveVariable(groupIndex, vi)}
              disabled={isReadOnly}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ))}
        <div className="text-xs text-muted-foreground">
          {t('variableAggregator.manager.outputType')}:{' '}
          {(group.output_type as string) || t('variableAggregator.manager.undetermined')}
        </div>
      </div>
    </div>
  );
};

export default GroupItem;
