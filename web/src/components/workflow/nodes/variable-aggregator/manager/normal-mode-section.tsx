'use client';

import React from 'react';
import type { PrimitiveType, VarSelector } from '../config';
import { Button } from '@/components/ui/button';
import NodeValueSelector from '../../../common/node-value-selector';
import VariableSelector from '../../../common/variable-selector';
import { useT } from '@/i18n';
import { Trash2, Plus } from 'lucide-react';

interface NormalModeSectionProps {
  nodeId: string;
  variables: VarSelector[];
  outputType?: PrimitiveType;
  isReadOnly: boolean;
  // Handlers
  onRemoveVariable: (index: number) => void;
  onClearVariables: () => void;
  onChangeSelector: (
    idx: number,
    payload: { sourceId: string; key: string; valuePath: string[]; type: PrimitiveType }
  ) => void;
  onSelectVariable: (payload: {
    sourceId: string;
    key: string;
    valuePath: string[];
    type: PrimitiveType;
  }) => void;
}

/**
 * Normal mode section for variable aggregator
 * Displays a flat list of variables without grouping
 */
const NormalModeSection: React.FC<NormalModeSectionProps> = ({
  nodeId,
  variables,
  outputType,
  isReadOnly,
  onRemoveVariable,
  onClearVariables,
  onChangeSelector,
  onSelectVariable,
}) => {
  const t = useT('nodes');

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <div className="text-sm font-medium">{t('variableAggregator.manager.normalSection')}</div>
        <VariableSelector
          nodeId={nodeId}
          onSelect={onSelectVariable}
          hideSystem
          typeFilter={
            variables.length === 0 ? undefined : tpe => tpe === (outputType as PrimitiveType)
          }
        >
          <Button variant="ghost" isIcon disabled={isReadOnly}>
            <Plus className="h-4 w-4" />
          </Button>
        </VariableSelector>
      </div>
      <div className="space-y-2">
        {variables.length === 0 ? (
          <div className="text-xs text-muted-foreground">
            {t('variableAggregator.manager.noVariables')}
          </div>
        ) : null}
        {variables.map((v, idx) => (
          <div key={`${idx}`} className="flex items-center gap-2">
            <NodeValueSelector
              nodeId={nodeId}
              value={Array.isArray(v) && v.length >= 2 ? v : undefined}
              onChange={p => onChangeSelector(idx, p)}
              typeFilter={
                idx === 0
                  ? outputType
                    ? tpe => tpe === (outputType as PrimitiveType)
                    : undefined
                  : tpe => tpe === (outputType as PrimitiveType)
              }
              className="grow"
              hideSystem
              disabled={isReadOnly}
            />
            <Button
              variant="ghost"
              isIcon
              onClick={() => onRemoveVariable(idx)}
              disabled={isReadOnly}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ))}
        <div className="flex items-center gap-2">
          <Button size="sm" variant="secondary" onClick={onClearVariables} disabled={isReadOnly}>
            {t('variableAggregator.manager.clear')}
          </Button>
        </div>
        <div className="text-xs text-muted-foreground">
          {t('variableAggregator.manager.outputType')}:{' '}
          {(outputType as string) || t('variableAggregator.manager.undetermined')}
        </div>
      </div>
    </div>
  );
};

export default NormalModeSection;
