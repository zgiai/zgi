import React, { useMemo } from 'react';
import { Braces, Trash2, Type } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Tabs } from '@/components/ui/tabs';
import { ALLOWED_FILE_TYPES } from '@/components/workflow/types/input-var';
import type { UpstreamExportItem } from '@/components/workflow/store/helpers/graph';
import { useT } from '@/i18n';
import NodeValueSelector from '../node-value-selector';
import {
  WorkflowCompactFieldHint,
  WorkflowCompactFieldLabel,
  WorkflowCompactSelectTrigger,
  WorkflowCompactTabsList,
  WorkflowCompactTabsTrigger,
} from '../compact-form';
import { WORKFLOW_CONTROL_COMPACT_CLASS } from '../form-density';
import {
  clampWorkflowSafeNumber,
  WORKFLOW_SAFE_NUMBER_MAX,
  WORKFLOW_SAFE_NUMBER_MIN,
} from '../number-limits';
import type { ComparisonOperator, Condition, VarType } from './types';
import { getOperators, isTypeCompatible, operatorI18nKey, operatorNeedsValue } from './utils';

export interface ConditionRowProps {
  nodeId: string;
  cond: Condition;
  onUpdateCondition: (conditionId: string, patch: Partial<Condition>) => void;
  onRemoveCondition: (conditionId: string) => void;
  upstreamsOverride?: UpstreamExportItem[];
  readOnly?: boolean;
}

const ConditionRow: React.FC<ConditionRowProps> = React.memo(
  ({
    nodeId,
    cond,
    onUpdateCondition,
    onRemoveCondition,
    upstreamsOverride,
    readOnly = false,
  }) => {
    const t = useT();
    const tCommon = useT('common');

    const effectiveKey = useMemo(() => {
      if (cond.key) return cond.key;
      if (Array.isArray(cond.variable_selector) && cond.variable_selector.length > 2) {
        return cond.variable_selector[cond.variable_selector.length - 1];
      }
      return undefined;
    }, [cond.key, cond.variable_selector]);

    const ops = useMemo(
      () => getOperators(cond.varType, effectiveKey),
      [cond.varType, effectiveKey]
    );
    const needsValue = useMemo(
      () => operatorNeedsValue(cond.comparison_operator),
      [cond.comparison_operator]
    );
    const hasSelectedVariable = useMemo(
      () => Array.isArray(cond.variable_selector) && cond.variable_selector.length >= 2,
      [cond.variable_selector]
    );
    const compareMode = cond.numberVarType === 'variable' ? 'variable' : 'number';
    const compareVariableValue = useMemo(() => {
      if (typeof cond.value !== 'string') return undefined;
      const s = cond.value;
      if (!s.startsWith('{{#') || !s.endsWith('#}}')) return undefined;
      return s.slice(3, -3).split('.');
    }, [cond.value]);

    const valueHint = useMemo(() => {
      if (!hasSelectedVariable) return t('nodes.ifElse.hints.selectVariableFirst');
      if (!needsValue) return t('nodes.ifElse.hints.noValueRequired');
      if (cond.varType === 'number' && compareMode === 'variable') {
        return t('nodes.ifElse.hints.numericVariableOnly');
      }
      if (cond.varType === 'number') return t('nodes.ifElse.hints.inputNumber');
      if (typeof cond.value === 'boolean') return t('nodes.ifElse.hints.toggleBoolean');
      if ((cond.varType === 'file' || cond.varType === 'string') && effectiveKey === 'type') {
        return t('nodes.ifElse.hints.selectFileType');
      }
      return t('nodes.ifElse.hints.inputText');
    }, [compareMode, cond.value, cond.varType, effectiveKey, hasSelectedVariable, needsValue, t]);

    const valueLabel =
      cond.varType === 'number' && compareMode === 'variable'
        ? t('nodes.ifElse.fields.sourceVariable')
        : t('nodes.ifElse.fields.compareValue');
    const valuePlaceholderKey =
      cond.varType === 'number'
        ? ('nodes.ifElse.placeholders.inputNumber' as const)
        : ('nodes.ifElse.placeholders.inputText' as const);

    return (
      <div className="relative rounded-lg border bg-card px-3 py-2.5">
        <div className="space-y-3 pr-8">
          <div className="flex items-start gap-2.5">
            <div className="min-w-0 flex-[1.35] space-y-1">
              <NodeValueSelector
                nodeId={nodeId}
                upstreamsOverride={upstreamsOverride}
                label={t('nodes.ifElse.fields.targetVariable')}
                density="compact"
                className="space-y-1"
                placeholder={t('nodes.ifElse.placeholders.selectVariable')}
                disabled={readOnly}
                value={
                  Array.isArray(cond.variable_selector) && cond.variable_selector.length >= 2
                    ? cond.variable_selector
                    : undefined
                }
                onChange={({ key: topKey, path, valuePath, type }) => {
                  const leafKey = path && path.length > 0 ? path[path.length - 1] : topKey;
                  const nextVarType = (type as VarType) ?? cond.varType;
                  const allowed = getOperators(nextVarType, leafKey);
                  const keepOp = allowed.includes(cond.comparison_operator);
                  const nextOp: ComparisonOperator = keepOp
                    ? (cond.comparison_operator as ComparisonOperator)
                    : (allowed[0] as ComparisonOperator);
                  let nextValue = cond.value;
                  if (operatorNeedsValue(nextOp)) {
                    if (nextVarType === 'boolean') nextValue = Boolean(cond.value);
                    else if (nextVarType === 'number') {
                      nextValue = typeof nextValue === 'string' ? nextValue : '';
                    } else if (
                      (nextVarType === 'file' || nextVarType === 'string') &&
                      leafKey === 'type'
                    ) {
                      nextValue = Array.isArray(nextValue) ? nextValue : [];
                    } else if (typeof nextValue !== 'string') {
                      nextValue = '';
                    }
                  } else {
                    nextValue = undefined;
                  }
                  onUpdateCondition(cond.id, {
                    variable_selector: valuePath,
                    varType: nextVarType,
                    key: leafKey,
                    comparison_operator: nextOp,
                    numberVarType:
                      nextVarType === 'number' ? (cond.numberVarType ?? 'number') : undefined,
                    value: nextValue,
                  });
                }}
              />
            </div>
            <div className="w-32 shrink-0 space-y-1">
              <WorkflowCompactFieldLabel>
                {t('nodes.ifElse.fields.operator')}
              </WorkflowCompactFieldLabel>
              <Select
                value={String(cond.comparison_operator)}
                disabled={readOnly || !hasSelectedVariable}
                onValueChange={(val: string) => {
                  const nextOperator = val as ComparisonOperator;
                  const nextPatch: Partial<Condition> = {
                    comparison_operator: nextOperator,
                  };
                  if (!operatorNeedsValue(nextOperator)) {
                    nextPatch.value = undefined;
                  } else if (cond.varType === 'boolean') {
                    nextPatch.value = typeof cond.value === 'boolean' ? cond.value : false;
                  } else if (Array.isArray(cond.value)) {
                    nextPatch.value = cond.value;
                  } else {
                    nextPatch.value = typeof cond.value === 'string' ? cond.value : '';
                  }
                  onUpdateCondition(cond.id, nextPatch);
                }}
              >
                <WorkflowCompactSelectTrigger>
                  <SelectValue placeholder={t('nodes.ifElse.fields.selectOperator')} />
                </WorkflowCompactSelectTrigger>
                <SelectContent>
                  {ops.map(op => (
                    <SelectItem key={op} value={op}>
                      {t(`nodes.ifElse.operators.${operatorI18nKey(op)}` as any)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          {needsValue ? (
            <div className="space-y-2">
              {cond.varType === 'number' ? (
                <div className="space-y-1">
                  <WorkflowCompactFieldLabel className="text-muted-foreground">
                    {t('nodes.ifElse.fields.compareMode')}
                  </WorkflowCompactFieldLabel>
                  <Tabs
                    value={compareMode}
                    onValueChange={val =>
                      onUpdateCondition(cond.id, {
                        numberVarType: val as 'variable' | 'number',
                        value: '',
                      })
                    }
                  >
                    <WorkflowCompactTabsList className="w-fit">
                      <WorkflowCompactTabsTrigger value="number" disabled={readOnly}>
                        <Type className="h-3.5 w-3.5 shrink-0" />
                        <span>{t('nodes.ifElse.fields.compareWithNumber')}</span>
                      </WorkflowCompactTabsTrigger>
                      <WorkflowCompactTabsTrigger value="variable" disabled={readOnly}>
                        <Braces className="h-3.5 w-3.5 shrink-0" />
                        <span>{t('nodes.ifElse.fields.compareWithVariable')}</span>
                      </WorkflowCompactTabsTrigger>
                    </WorkflowCompactTabsList>
                  </Tabs>
                </div>
              ) : null}

              <div className="space-y-1">
                <WorkflowCompactFieldLabel>{valueLabel}</WorkflowCompactFieldLabel>
                {typeof cond.value === 'boolean' ? (
                  <div className="flex h-8 items-center justify-between rounded-md border bg-background px-2.5">
                    <span className="text-xs text-muted-foreground">
                      {Boolean(cond.value) ? tCommon('yes') : tCommon('no')}
                    </span>
                    <Switch
                      checked={Boolean(cond.value)}
                      onCheckedChange={val => onUpdateCondition(cond.id, { value: val })}
                      disabled={readOnly}
                    />
                  </div>
                ) : cond.varType === 'number' && compareMode === 'variable' ? (
                  <NodeValueSelector
                    nodeId={nodeId}
                    upstreamsOverride={upstreamsOverride}
                    density="compact"
                    className="space-y-1"
                    typeFilter={type => isTypeCompatible(cond.varType, type)}
                    placeholder={t('nodes.ifElse.placeholders.selectNumericVariable')}
                    value={compareVariableValue}
                    disabled={readOnly}
                    onChange={({ valuePath }) =>
                      onUpdateCondition(cond.id, {
                        value: `{{#${valuePath.join('.')}#}}`,
                      })
                    }
                  />
                ) : (cond.varType === 'file' || cond.varType === 'string') &&
                  effectiveKey === 'type' ? (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <button
                        type="button"
                        className="flex min-h-8 w-full flex-wrap items-center gap-1 rounded-md border border-input bg-background px-2.5 py-1 text-left text-xs shadow-xs"
                        disabled={readOnly}
                      >
                        {Array.isArray(cond.value) && cond.value.length > 0 ? (
                          cond.value.map(v => (
                            <Badge key={v} variant="secondary" className="px-1 py-0 text-[10px]">
                              {t(`nodes.ifElse.fileTypes.${v}` as any)}
                            </Badge>
                          ))
                        ) : (
                          <span className="text-muted-foreground">
                            {t('nodes.ifElse.fields.selectType')}
                          </span>
                        )}
                      </button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent className="w-48" align="start">
                      {ALLOWED_FILE_TYPES.map(type => (
                        <DropdownMenuCheckboxItem
                          key={type}
                          checked={Array.isArray(cond.value) && cond.value.includes(type)}
                          disabled={readOnly}
                          onCheckedChange={checked => {
                            const currentValues = Array.isArray(cond.value) ? cond.value : [];
                            const nextValues = checked
                              ? [...currentValues, type]
                              : currentValues.filter(v => v !== type);
                            onUpdateCondition(cond.id, { value: nextValues });
                          }}
                          onSelect={e => e.preventDefault()}
                        >
                          {t(`nodes.ifElse.fileTypes.${type}` as any)}
                        </DropdownMenuCheckboxItem>
                      ))}
                    </DropdownMenuContent>
                  </DropdownMenu>
                ) : (
                  <Input
                    type={cond.varType === 'number' ? 'number' : 'text'}
                    min={cond.varType === 'number' ? WORKFLOW_SAFE_NUMBER_MIN : undefined}
                    max={cond.varType === 'number' ? WORKFLOW_SAFE_NUMBER_MAX : undefined}
                    className={WORKFLOW_CONTROL_COMPACT_CLASS}
                    placeholder={t(valuePlaceholderKey)}
                    value={typeof cond.value === 'string' ? cond.value : ''}
                    onChange={e => {
                      const rawValue = e.target.value;
                      if (cond.varType !== 'number' || rawValue === '') {
                        onUpdateCondition(cond.id, { value: rawValue });
                        return;
                      }

                      const parsed = Number(rawValue);
                      onUpdateCondition(cond.id, {
                        value: Number.isNaN(parsed)
                          ? ''
                          : String(clampWorkflowSafeNumber(parsed)),
                      });
                    }}
                    disabled={readOnly}
                  />
                )}
                <WorkflowCompactFieldHint>{valueHint}</WorkflowCompactFieldHint>
              </div>
            </div>
          ) : null}
        </div>

        <Button
          variant="ghost"
          size="xs"
          isIcon
          className="absolute right-2 top-2 h-7 w-7 rounded-md hover:bg-destructive/5 hover:text-destructive"
          onClick={() => onRemoveCondition(cond.id)}
          aria-label={t('nodes.ifElse.actions.deleteCondition')}
          disabled={readOnly}
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    );
  },
  (prev, next) => {
    if (prev.nodeId !== next.nodeId) return false;
    if (prev.readOnly !== next.readOnly) return false;
    const a = prev.cond;
    const b = next.cond;
    if (a.id !== b.id) return false;
    if (a.varType !== b.varType) return false;
    if (a.key !== b.key) return false;
    if (a.numberVarType !== b.numberVarType) return false;
    if (a.comparison_operator !== b.comparison_operator) return false;
    const aSel = Array.isArray(a.variable_selector) ? a.variable_selector.join('.') : '';
    const bSel = Array.isArray(b.variable_selector) ? b.variable_selector.join('.') : '';
    if (aSel !== bSel) return false;
    const aVal = Array.isArray(a.value)
      ? a.value.join(',')
      : typeof a.value === 'string' || typeof a.value === 'boolean'
        ? String(a.value)
        : '';
    const bVal = Array.isArray(b.value)
      ? b.value.join(',')
      : typeof b.value === 'string' || typeof b.value === 'boolean'
        ? String(b.value)
        : '';
    if (aVal !== bVal) return false;
    return true;
  }
);

ConditionRow.displayName = 'ConditionRow';

export default ConditionRow;
