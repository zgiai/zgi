'use client';

import React from 'react';
import { useT } from '@/i18n';
import { useContainerVariableSources, useNodeData, useNodeDataUpdate } from '../../../hooks';
import type { LoopNodeData } from '../config';
import { Label } from '@/components/ui/label';
import { Slider } from '@/components/ui/slider';
import ConditionList from '../../../common/condition-builder/condition-list';
import { newCondition } from '../../../common/condition-builder/utils';
import type { Condition, VarType } from '../../../common/condition-builder/types';
import LoopVariablesPanel from './loop-variables-panel';

interface LoopManagerProps {
  id: string;
  readOnly?: boolean;
}

const LoopManager: React.FC<LoopManagerProps> = ({ id, readOnly = false }) => {
  const t = useT();
  const nodeData = useNodeData<LoopNodeData>(id);
  const updateNodeData = useNodeDataUpdate<LoopNodeData>(id);
  const upstreamsOverride = useContainerVariableSources(id, {
    includeUpstream: true,
    includeScopedSelf: true,
    includeChildOutputs: true,
    excludeChildNodeTypes: ['loop-start'],
  });

  const handleLoopCountChange = (vals: number[]) => {
    if (readOnly) return;
    updateNodeData({ loop_count: vals[0] });
  };

  const addCondition = (varType: VarType = 'string') => {
    if (readOnly) return;
    const newCond = newCondition(varType);
    updateNodeData(prev => ({
      break_conditions: [...(prev.break_conditions || []), newCond],
    }));
  };

  const removeCondition = (conditionId: string) => {
    if (readOnly) return;
    updateNodeData(prev => ({
      break_conditions: (prev.break_conditions || []).filter(c => c.id !== conditionId),
    }));
  };

  const updateCondition = (conditionId: string, patch: Partial<Condition>) => {
    if (readOnly) return;
    updateNodeData(prev => ({
      break_conditions: (prev.break_conditions || []).map(c =>
        c.id === conditionId ? { ...c, ...patch } : c
      ),
    }));
  };

  return (
    <div className="space-y-6">
      <LoopVariablesPanel
        nodeId={id}
        variables={nodeData?.loop_variables || []}
        onChange={vars => updateNodeData({ loop_variables: vars })}
        readOnly={readOnly}
      />

      <div className="space-y-3">
        <div className="flex justify-between items-center">
          <Label>{t('nodes.loop.fields.loopCount')}</Label>
          <span className="text-sm font-medium">{nodeData?.loop_count ?? 10}</span>
        </div>
        <Slider
          value={[nodeData?.loop_count ?? 10]}
          min={0}
          max={100}
          step={1}
          onValueChange={handleLoopCountChange}
          disabled={readOnly}
        />
        <p className="text-xs text-muted-foreground">{t('nodes.loop.hints.loopCount')}</p>
      </div>

      <ConditionList
        nodeId={id}
        conditions={nodeData?.break_conditions || []}
        logicalOperator={nodeData?.logical_operator ?? 'and'}
        onUpdateLogic={val => updateNodeData({ logical_operator: val })}
        onAddCondition={addCondition}
        onUpdateCondition={updateCondition}
        onRemoveCondition={removeCondition}
        title={t('nodes.loop.fields.breakConditions')}
        description={t('nodes.loop.hints.breakConditions')}
        addLabel={t('nodes.loop.actions.addCondition')}
        upstreamsOverride={upstreamsOverride}
        readOnly={readOnly}
      />
    </div>
  );
};

export default LoopManager;
