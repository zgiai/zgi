import React from 'react';
import { Button } from '@/components/ui/button';
import { Tabs } from '@/components/ui/tabs';
import { Plus } from 'lucide-react';
import ConditionRow from './condition-row';
import type { Condition, VarType } from './types';
import { useT } from '@/i18n';
import type { UpstreamExportItem } from '@/components/workflow/store/helpers/graph';
import {
  WorkflowCompactFieldLabel,
  WorkflowCompactTabsList,
  WorkflowCompactTabsTrigger,
} from '../compact-form';

export interface ConditionListProps {
  nodeId: string;
  conditions: Condition[];
  logicalOperator?: 'and' | 'or';
  onToggleLogic?: () => void;
  onUpdateLogic?: (val: 'and' | 'or') => void;
  onAddCondition: (varType: VarType) => void;
  onUpdateCondition: (conditionId: string, patch: Partial<Condition>) => void;
  onRemoveCondition: (conditionId: string) => void;
  readOnly?: boolean;
  addLabel?: string;
  title?: React.ReactNode;
  description?: React.ReactNode;
  upstreamsOverride?: UpstreamExportItem[];
}

const ConditionList: React.FC<ConditionListProps> = React.memo(
  ({
    nodeId,
    conditions,
    logicalOperator = 'and',
    onToggleLogic,
    onUpdateLogic,
    onAddCondition,
    onUpdateCondition,
    onRemoveCondition,
    readOnly,
    addLabel,
    title,
    description,
    upstreamsOverride,
  }) => {
    const t = useT();
    const handleLogicChange = (val: 'and' | 'or') => {
      if (onUpdateLogic) {
        onUpdateLogic(val);
        return;
      }
      if (onToggleLogic && val !== logicalOperator) {
        onToggleLogic();
      }
    };

    return (
      <div className="space-y-3">
        {(title || conditions.length > 1) && (
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0 space-y-1">
              {title ? (
                typeof title === 'string' ? (
                  <WorkflowCompactFieldLabel className="text-sm leading-5">
                    {title}
                  </WorkflowCompactFieldLabel>
                ) : (
                  title
                )
              ) : (
                <div />
              )}
              {description ? (
                <p className="text-[11px] leading-4 text-muted-foreground">{description}</p>
              ) : null}
            </div>
            {conditions.length > 1 ? (
              <div className="flex shrink-0 items-center gap-2 pt-0.5">
                <Tabs
                  value={logicalOperator}
                  onValueChange={val => {
                    if (readOnly) return;
                    handleLogicChange(val as 'and' | 'or');
                  }}
                >
                  <WorkflowCompactTabsList className="w-fit">
                    <WorkflowCompactTabsTrigger value="and" disabled={readOnly}>
                      {t('nodes.ifElse.fields.and')}
                    </WorkflowCompactTabsTrigger>
                    <WorkflowCompactTabsTrigger value="or" disabled={readOnly}>
                      {t('nodes.ifElse.fields.or')}
                    </WorkflowCompactTabsTrigger>
                  </WorkflowCompactTabsList>
                </Tabs>
              </div>
            ) : null}
          </div>
        )}

        <div className="space-y-2">
          {conditions.map(cond => (
            <ConditionRow
              key={cond.id}
              nodeId={nodeId}
              cond={cond}
              onUpdateCondition={onUpdateCondition}
              onRemoveCondition={onRemoveCondition}
              upstreamsOverride={upstreamsOverride}
              readOnly={readOnly}
            />
          ))}
          {!readOnly && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => onAddCondition('string')}
              className="h-8 w-full text-xs"
            >
              <Plus className="mr-1.5 h-3.5 w-3.5" />
              {addLabel || t('nodes.ifElse.fields.addCondition')}
            </Button>
          )}
        </div>
      </div>
    );
  }
);

ConditionList.displayName = 'ConditionList';

export default ConditionList;
