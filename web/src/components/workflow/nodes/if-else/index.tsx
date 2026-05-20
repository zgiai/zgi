import React from 'react';
import { type IfElseNodeData, type CaseItem, type Condition, ComparisonOperator } from './types';
import { cn } from '@/lib/utils';
import CustomHandle from '../../ui/custom-handle';
import { Position } from '@xyflow/react';
import ValueBadge from '../../ui/value-badge';
import { operatorI18nKey } from './utils';
import { useWorkflowStore } from '../../store';
import { useT, type NodesSuffix } from '@/i18n';

export interface IfElseContentProps {
  nodeId: string;
  data: IfElseNodeData;
}

// Render a simple, compact summary of conditions per case
const ConditionSummary: React.FC<{ c: Condition; currentNodeId?: string }> = ({
  c,
  currentNodeId,
}) => {
  const t = useT();
  const op = c.comparison_operator;
  const isTemplate = (s: unknown): s is string =>
    typeof s === 'string' && /^\s*\{\{#.+\..+#\}\}\s*$/u.test(s.trim());
  const val = (() => {
    if (typeof c.value === 'boolean') return c.value ? 'True' : 'False';
    if (c.key === 'type' && (c.varType === 'file' || c.varType === 'string')) {
      if (Array.isArray(c.value))
        return c.value.map(v => t(`nodes.ifElse.fileTypes.${v}` as any)).join(', ');
      if (typeof c.value === 'string' && !isTemplate(c.value)) {
        return t(`nodes.ifElse.fileTypes.${c.value}` as any);
      }
    }
    if (Array.isArray(c.value)) return c.value.join(', ');
    if (typeof c.value === 'string' && !isTemplate(c.value)) return c.value;
    return '';
  })();

  return (
    <div className="text-xs text-secondary-foreground flex items-center flex-wrap">
      {Array.isArray(c.variable_selector) && c.variable_selector.length >= 2 ? (
        <ValueBadge
          className="max-w-[100px]"
          selector={c.variable_selector}
          currentNodeId={currentNodeId}
        />
      ) : (
        <span className="font-medium text-sm text-destructive">
          {t('nodes.ifElse.fields.unconfigured')}
        </span>
      )}
      <span className="mx-1">{t(`nodes.ifElse.operators.${operatorI18nKey(op)}` as any)}</span>
      {![
        ComparisonOperator.isNull,
        ComparisonOperator.isNotNull,
        ComparisonOperator.empty,
        ComparisonOperator.notEmpty,
        ComparisonOperator.exists,
        ComparisonOperator.notExists,
      ].includes(op) &&
        (isTemplate(c.value) ? (
          <ValueBadge
            className="max-w-[100px]"
            template={c.value as string}
            currentNodeId={currentNodeId}
          />
        ) : val !== '' ? (
          <span
            className="text-sm truncate inline-block w-0 grow align-bottom text-highlight"
            title={val}
          >
            {val}
          </span>
        ) : (
          <span className="text-sm truncate inline-block max-w-[100px] align-bottom text-destructive">
            {t('nodes.ifElse.fields.null')}
          </span>
        ))}
      {c.sub_variable_condition ? (
        <span className="ml-1 text-muted-foreground">
          (+ {t('nodes.ifElse.fields.subVariableCondition')})
        </span>
      ) : null}
    </div>
  );
};

const CaseBlock: React.FC<{
  caseItem: CaseItem;
  idx: number;
  total: number;
  currentNodeId?: string;
}> = ({ caseItem, idx, total, currentNodeId }) => {
  const headerRight =
    idx === 0 && total === 1 ? 'IF' : total > 1 && idx < total ? (idx === 0 ? 'IF' : 'ELIF') : '';
  const t = useT();

  return (
    <div className="border-t pt-2 mt-2">
      <div className="flex items-center justify-between text-xs text-primary font-medium relative">
        <div className="">{total > 1 ? `CASE ${idx + 1}` : ''}</div>
        <div className="text-[10px] text-slate-400 font-bold text-right tracking-wider">
          {headerRight}
        </div>
        <CustomHandle
          type="source"
          position={Position.Right}
          id={caseItem.case_id}
          style={{ top: '50%', right: -14 }}
        />
      </div>
      <div className="flex items-stretch mt-1">
        {caseItem.conditions.length > 1 && (
          <div className="relative flex-none pl-4 mr-1">
            <div className="absolute top-0 left-0 w-3 h-full">
              <div className="absolute top-0 left-full h-px w-1 bg-muted-foreground" />
              <div className="absolute top-0 left-full w-px h-full bg-muted-foreground" />
              <div className="absolute bottom-0 left-full h-px w-1 bg-muted-foreground" />
            </div>
            {/* Operator label centered */}
            <div className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 text-[9px] text-muted-foreground bg-card">
              {caseItem.logical_operator.toUpperCase()}
            </div>
          </div>
        )}
        <div className="space-y-1 grow">
          {caseItem.conditions.length === 0 ? (
            <div className="text-xs text-muted-foreground">
              {t('nodes.ifElse.fields.unconfigured')}
            </div>
          ) : (
            caseItem.conditions.map(cond => (
              <ConditionSummary key={cond.id} c={cond} currentNodeId={currentNodeId} />
            ))
          )}
        </div>
      </div>
    </div>
  );
};

const IfElseContent: React.FC<IfElseContentProps> = ({ nodeId, data }) => {
  const cases = Array.isArray(data.cases) ? data.cases : [];
  const total = cases.length;

  return (
    <div className={cn('mt-1')}>
      <CustomHandle type="target" position={Position.Left} id="target" style={{ top: -18 }} />
      {cases.map((ci, idx) => (
        <CaseBlock
          key={ci.case_id || idx}
          caseItem={ci}
          idx={idx}
          total={total}
          currentNodeId={nodeId}
        />
      ))}

      {/* ELSE label row */}
      <div className="mt-2 pt-2 border-t border-slate-200/50 dark:border-slate-800/50">
        <div className="relative">
          <div className="text-[10px] text-slate-400 font-bold text-right tracking-wider">ELSE</div>
          <CustomHandle
            type="source"
            position={Position.Right}
            id="false"
            style={{ top: '50%', right: -15 }}
          />
        </div>
      </div>
    </div>
  );
};

export default IfElseContent;
