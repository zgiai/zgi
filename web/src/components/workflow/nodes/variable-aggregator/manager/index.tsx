'use client';

import React, { useMemo, useCallback } from 'react';
import type {
  VariableAggregatorNodeData,
  VariableAggregatorGroup,
  PrimitiveType,
  VarSelector,
} from '../config';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { useT } from '@/i18n';
import { sanitizeIdentifier, ensureUniqueIdentifier } from '@/utils/validation';
import { useLocalNodeData } from '../../../hooks/use-local-node-data';
import { useNodeOutputVariables } from '../../../hooks';
import OutputVariablesView from '../../../common/output-variables-view';
import NormalModeSection from './normal-mode-section';
import GroupItem from './group-item';

interface AggregatorManagerProps {
  id: string;
  readOnly?: boolean;
}

const VariableAggregatorManager: React.FC<AggregatorManagerProps> = ({ id, readOnly = false }) => {
  const t = useT('nodes');
  const isReadOnly = readOnly;

  const { localData, setLocalData, flush } = useLocalNodeData<{
    variables: VarSelector[];
    output_type?: PrimitiveType;
    advanced_settings: { group_enabled: boolean; groups: VariableAggregatorGroup[] };
  }>(id, {
    delay: 400,
    isEqual: (a, b) => {
      try {
        return JSON.stringify(a) === JSON.stringify(b);
      } catch {
        return a === (b as unknown);
      }
    },
  });

  const groupEnabled = Boolean(localData.advanced_settings?.group_enabled);
  const groups: VariableAggregatorGroup[] = React.useMemo(() => {
    const src = localData.advanced_settings?.groups;
    return Array.isArray(src) ? (src as VariableAggregatorGroup[]) : [];
  }, [localData.advanced_settings]);
  const variables: VarSelector[] = React.useMemo(() => {
    const src = localData.variables;
    return Array.isArray(src) ? (src as VarSelector[]) : [];
  }, [localData.variables]);


  // Helpers
  const newGroup = React.useCallback(
    (index: number): VariableAggregatorGroup => {
      const base = `group_${index + 1}`;
      const existing = groups.map(g => g.group_name).filter(Boolean) as string[];
      const name = ensureUniqueIdentifier(sanitizeIdentifier(base), existing);
      return {
        groupId: `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`,
        group_name: name,
        output_type: undefined,
        variables: [],
      };
    },
    [groups]
  );

  const setGroupEnabled = useCallback(
    (enabled: boolean) => {
      const nextGroups = groups.length > 0 ? groups : [newGroup(groups.length)];
      setLocalData({
        advanced_settings: { group_enabled: enabled, groups: nextGroups },
      });
      flush();
    },
    [groups, newGroup, setLocalData, flush]
  );

  // (functions moved above)

  const commitGroups = (next: VariableAggregatorGroup[]) => {
    setLocalData({ advanced_settings: { group_enabled: true, groups: next } });
    flush();
  };

  // Normal mode handlers
  const removeVariableAt = (index: number) => {
    const next = variables.filter((_, i) => i !== index);
    // When the first item removed, re-evaluate output_type
    let nextOutputType = localData.output_type;
    if (index === 0) {
      nextOutputType = next.length > 0 ? localData.output_type : undefined;
    }
    setLocalData({ variables: next, output_type: nextOutputType });
    flush();
  };
  const clearVariables = () => {
    setLocalData({ variables: [], output_type: undefined });
    flush();
  };

  // Commit for pending selection in normal mode
  const handlePendingAddSelect = (payload: {
    sourceId: string;
    key: string;
    valuePath: string[];
    type: PrimitiveType;
  }) => {
    // Disallow system variable anywhere
    if (payload.sourceId === 'sys') return;
    const targetType = (localData.output_type ||
      (variables.length === 0 ? payload.type : undefined)) as PrimitiveType | undefined;
    // Enforce type lock for subsequent adds
    if (variables.length > 0 && targetType && payload.type !== targetType) return;
    const pair: VarSelector = payload.valuePath;
    const exists = variables.some(v => v.join('.') === pair.join('.'));
    if (exists) return;
    const next = [...variables, pair];
    setLocalData({ variables: next, output_type: targetType });
    flush();
  };

  const handleChangeSelector = (
    idx: number,
    payload: { sourceId: string; key: string; valuePath: string[]; type: PrimitiveType }
  ) => {
    const pair: VarSelector = payload.valuePath;
    // Disallow system variable anywhere
    if (payload.sourceId === 'sys') return;
    // Type lock: establish output_type from first variable
    const targetType = (localData.output_type || (idx === 0 ? payload.type : undefined)) as
      | PrimitiveType
      | undefined;
    // When output_type already set, even first variable cannot change to other type
    if (idx === 0 && localData.output_type && payload.type !== localData.output_type) return;
    // Subsequent must match
    if (idx > 0 && targetType && payload.type !== targetType) return;
    // Duplicate guard
    const exists = variables.some(v => v.join('.') === pair.join('.'));
    if (exists) return;
    const next = [...variables];
    next[idx] = pair;
    setLocalData({ variables: next, output_type: targetType });
    flush();
  };

  // Group mode handlers
  const addGroup = () => commitGroups([...(groups || []), newGroup(groups.length)]);
  const removeGroupAt = (index: number) => {
    const gid = groups[index]?.groupId;
    const next = groups.filter((_, i) => i !== index);
    if (next.length === 0) next.push(newGroup(0));
    commitGroups(next);
  };
  const normalizeGroupNameOnBlur = (index: number, inputValue: string) => {
    const cur = groups[index];
    if (!cur) return;
    const raw = inputValue || '';
    const existing = groups
      .map((g, i) => (i === index ? undefined : g.group_name))
      .filter(Boolean) as string[];
    const finalName = ensureUniqueIdentifier(sanitizeIdentifier(raw), existing, cur.group_name);
    // Always update if the input value differs from stored value
    if (finalName !== cur.group_name) {
      const next = groups.map((g, i) => (i === index ? { ...g, group_name: finalName } : g));
      setLocalData({ advanced_settings: { group_enabled: true, groups: next } });
      flush();
    }
  };
  const groupRemoveVar = (gi: number, vi: number) => {
    const next = groups.map((g, i) => {
      if (i !== gi) return g;
      const vars = g.variables.filter((_, j) => j !== vi);
      return {
        ...g,
        variables: vars,
        output_type: vars.length === 0 ? undefined : g.output_type,
      } as VariableAggregatorGroup;
    });
    commitGroups(next);
  };
  const handlePendingGroupSelect = (
    gi: number,
    payload: { sourceId: string; key: string; valuePath: string[]; type: PrimitiveType }
  ) => {
    const g = groups[gi];
    if (!g) return;
    // Disallow system variable anywhere
    if (payload.sourceId === 'sys') return;
    const firstType: PrimitiveType | undefined =
      g.output_type || (g.variables.length === 0 ? payload.type : undefined);
    if (g.variables.length > 0 && firstType && payload.type !== firstType) return;
    const pair: VarSelector = payload.valuePath;
    const dup = g.variables.some(v => v.join('.') === pair.join('.'));
    if (dup) return;
    const nextGroups = groups.map((it, idx) => {
      if (idx !== gi) return it;
      return {
        ...it,
        variables: [...it.variables, pair],
        output_type: firstType,
      } as VariableAggregatorGroup;
    });
    commitGroups(nextGroups);
  };
  const groupChangeSelector = (
    gi: number,
    vi: number,
    payload: { sourceId: string; key: string; valuePath: string[]; type: PrimitiveType }
  ) => {
    if (payload.sourceId === 'sys') return; // disallow sys anywhere
    const group = groups[gi];
    if (vi === 0 && group?.output_type && payload.type !== group.output_type) return; // first var type cannot change once decided
    const firstType: PrimitiveType | undefined =
      group.output_type || (vi === 0 ? payload.type : undefined);
    if (vi > 0 && firstType && payload.type !== firstType) return; // type lock
    const pair: VarSelector = payload.valuePath;
    const dup = group.variables.some(v => v.join('.') === pair.join('.'));
    if (dup) return;
    const next = groups.map((g, i) => {
      if (i !== gi) return g;
      const vars = [...g.variables];
      vars[vi] = pair;
      return { ...g, variables: vars, output_type: firstType } as VariableAggregatorGroup;
    });
    commitGroups(next);
  };

  const outputVariables = useNodeOutputVariables(id);

  return (
    <div className="space-y-4">
      {/* Mode toggle */}
      <div className="flex items-center justify-between">
        <div className="text-sm font-medium">{t('variableAggregator.manager.mode')}</div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">
            {t('variableAggregator.manager.normal')}
          </span>
          <Switch
            checked={groupEnabled}
            onCheckedChange={v => setGroupEnabled(v)}
            disabled={isReadOnly}
          />
          <span className="text-xs text-muted-foreground">
            {t('variableAggregator.manager.group')}
          </span>
        </div>
      </div>

      {/* Normal mode or Group mode */}
      {!groupEnabled ? (
        <NormalModeSection
          nodeId={id}
          variables={variables}
          outputType={localData.output_type}
          isReadOnly={isReadOnly}
          onRemoveVariable={removeVariableAt}
          onClearVariables={clearVariables}
          onChangeSelector={handleChangeSelector}
          onSelectVariable={handlePendingAddSelect}
        />
      ) : (
        <div className="space-y-3">
          {groups.map((g, gi) => (
            <GroupItem
              key={g.groupId}
              nodeId={id}
              group={g}
              groupIndex={gi}
              isReadOnly={isReadOnly}
              canDelete={groups.length > 1}
              onNameBlur={normalizeGroupNameOnBlur}
              onRemoveGroup={removeGroupAt}
              onRemoveVariable={groupRemoveVar}
              onChangeSelector={groupChangeSelector}
              onSelectVariable={handlePendingGroupSelect}
            />
          ))}
          <Button className="w-full" variant="secondary" onClick={addGroup} disabled={isReadOnly}>
            {t('variableAggregator.manager.addGroup')}
          </Button>
        </div>
      )}

      {/* Output variables display */}
      <OutputVariablesView variables={outputVariables} />
    </div>
  );
};

export default VariableAggregatorManager;
