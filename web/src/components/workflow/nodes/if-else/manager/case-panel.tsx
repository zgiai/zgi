import React from 'react';
import { Button } from '@/components/ui/button';
import ConditionList from '../../../common/condition-builder/condition-list';
import type { CaseItem, Condition, VarType } from '../types';
import { useT } from '@/i18n';
import { Trash2 } from 'lucide-react';

interface CasePanelProps {
  nodeId: string;
  item: CaseItem;
  idx: number;
  readOnly?: boolean;
  onToggleLogic: (caseId: string) => void;
  onAddCondition: (caseId: string, varType: VarType) => void;
  onRemoveCase: (caseId: string) => void;
  onUpdateCondition: (caseId: string, conditionId: string, patch: Partial<Condition>) => void;
  onRemoveCondition: (caseId: string, conditionId: string) => void;
}

const CasePanel: React.FC<CasePanelProps> = React.memo(
  ({
    nodeId,
    item,
    idx,
    readOnly = false,
    onToggleLogic,
    onAddCondition,
    onRemoveCase,
    onUpdateCondition,
    onRemoveCondition,
  }) => {
    const t = useT();
    const title = idx === 0 ? t('nodes.ifElse.labels.if') : t('nodes.ifElse.labels.caseN', { index: idx + 1 });
    const preview = item.conditions.length === 0
      ? t('nodes.ifElse.preview.empty')
      : item.conditions.length === 1
        ? t('nodes.ifElse.preview.single')
        : t(
            item.logical_operator === 'and'
              ? 'nodes.ifElse.preview.all'
              : 'nodes.ifElse.preview.any',
            { count: item.conditions.length }
          );

    return (
      <div className="relative rounded-lg border bg-card px-3 py-2.5">
        <div className="pr-8">
          <div className="text-[11px] font-medium leading-4 text-muted-foreground">{title}</div>
          <p className="mt-1 text-xs leading-5 text-foreground/80">{preview}</p>
        </div>
        <ConditionList
          nodeId={nodeId}
          conditions={item.conditions}
          logicalOperator={item.logical_operator}
          onToggleLogic={() => onToggleLogic(item.case_id)}
          onAddCondition={varType => onAddCondition(item.case_id, varType)}
          onUpdateCondition={(cid, patch) => onUpdateCondition(item.case_id, cid, patch)}
          onRemoveCondition={cid => onRemoveCondition(item.case_id, cid)}
          title={t('nodes.ifElse.fields.branchConditions')}
          description={t('nodes.ifElse.hints.conditionsHelp')}
          readOnly={readOnly}
        />
        {idx > 0 && (
          <Button
            variant="ghost"
            size="xs"
            isIcon
            className="absolute right-2 top-2 h-7 w-7 rounded-md hover:bg-destructive/5 hover:text-destructive"
            onClick={() => onRemoveCase(item.case_id)}
            aria-label={t('nodes.ifElse.fields.deleteBranch')}
            disabled={readOnly}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        )}
      </div>
    );
  },
  (prev, next) =>
    prev.nodeId === next.nodeId &&
    prev.idx === next.idx &&
    prev.item === next.item &&
    prev.readOnly === next.readOnly
);

CasePanel.displayName = 'CasePanel';

export default CasePanel;
