'use client';

import React, { useCallback, useMemo, useRef } from 'react';
import type { IfElseNodeData, CaseItem, Condition, BranchRef, VarType } from '../types';
import { DEFAULT_BRANCHES } from '../types';
import { branchNameCorrect, newCase, newCondition } from '../utils';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { useWorkflowStore } from '../../../store';
import { useLocalNodeData } from '../../../hooks/use-local-node-data';
import { useUpdateNodeInternals } from '@xyflow/react';
import CasePanel from './case-panel';

interface IfElseManagerProps {
  id: string;
  readOnly?: boolean;
}

// Lightweight deep-equality tailored for local data to avoid heavy JSON.stringify
function eqLocalData(a: Partial<IfElseNodeData>, b: Partial<IfElseNodeData>): boolean {
  const aBranches = Array.isArray(a.targetBranches) ? a.targetBranches : [];
  const bBranches = Array.isArray(b.targetBranches) ? b.targetBranches : [];
  if (aBranches.length !== bBranches.length) return false;
  for (let i = 0; i < aBranches.length; i += 1) {
    const ab = aBranches[i];
    const bb = bBranches[i];
    if (!ab || !bb) return false;
    if ((ab.id ?? '') !== (bb.id ?? '')) return false;
    if ((ab.name ?? '') !== (bb.name ?? '')) return false;
  }
  const aCases = Array.isArray(a.cases) ? a.cases : [];
  const bCases = Array.isArray(b.cases) ? b.cases : [];
  if (aCases.length !== bCases.length) return false;
  for (let i = 0; i < aCases.length; i += 1) {
    const ac = aCases[i];
    const bc = bCases[i];
    if (!ac || !bc) return false;
    if ((ac.case_id ?? '') !== (bc.case_id ?? '')) return false;
    if ((ac.logical_operator ?? '') !== (bc.logical_operator ?? '')) return false;
    const aConds = Array.isArray(ac.conditions) ? ac.conditions : [];
    const bConds = Array.isArray(bc.conditions) ? bc.conditions : [];
    if (aConds.length !== bConds.length) return false;
    for (let j = 0; j < aConds.length; j += 1) {
      const av = aConds[j];
      const bv = bConds[j];
      if (!av || !bv) return false;
      if ((av.id ?? '') !== (bv.id ?? '')) return false;
      if ((av.varType ?? '') !== (bv.varType ?? '')) return false;
      if ((av.comparison_operator ?? '') !== (bv.comparison_operator ?? '')) return false;
      if ((av.numberVarType ?? '') !== (bv.numberVarType ?? '')) return false;
      const aSel = Array.isArray(av.variable_selector) ? av.variable_selector.join('.') : '';
      const bSel = Array.isArray(bv.variable_selector) ? bv.variable_selector.join('.') : '';
      if (aSel !== bSel) return false;
      const aVal = av.value;
      const bVal = bv.value;
      if (Array.isArray(aVal) && Array.isArray(bVal)) {
        if (aVal.length !== bVal.length) return false;
        for (let k = 0; k < aVal.length; k++) {
          if (aVal[k] !== bVal[k]) return false;
        }
      } else if (String(aVal ?? '') !== String(bVal ?? '')) {
        return false;
      }
    }
  }
  return true;
}

const IfElseManager: React.FC<IfElseManagerProps> = ({ id, readOnly = false }) => {
  const t = useT();
  const updateNodeInternals = useUpdateNodeInternals();
  // rAF bucket to coalesce expensive internals updates on structural edits
  const updateRafRef = useRef<number | null>(null);

  const { localData, setLocalData } = useLocalNodeData<
    Pick<IfElseNodeData, 'cases' | 'targetBranches'>
  >(id, {
    delay: 250,
    isEqual: eqLocalData,
    flushOnUnmount: false,
  });

  const branches = useMemo<BranchRef[]>(() => {
    return Array.isArray(localData.targetBranches) && localData.targetBranches.length >= 2
      ? localData.targetBranches
      : DEFAULT_BRANCHES;
  }, [localData.targetBranches]);

  const cases = useMemo<CaseItem[]>(() => localData.cases ?? [], [localData.cases]);

  const setCases = useCallback(
    (next: CaseItem[], opts?: { structural?: boolean }) => {
      if (opts?.structural) {
        const nextBranches = branchNameCorrect(next, branches);
        setLocalData({ cases: next, targetBranches: nextBranches });
        // Coalesce internals update into a single rAF
        if (updateRafRef.current) cancelAnimationFrame(updateRafRef.current);
        updateRafRef.current = requestAnimationFrame(() => {
          updateNodeInternals(id);
        });
        // Ensure active branch remains valid only when structure changes
        const branchIds = new Set(nextBranches.map(b => b.id));
        const setActiveOutputHandle = useWorkflowStore.getState().setActiveOutputHandle as (
          nodeId: string,
          outputHandle: string | null
        ) => void;
        const currentActive =
          (useWorkflowStore.getState().activeOutputHandleByNodeId || {})[id] || null;
        if (currentActive && !branchIds.has(currentActive)) setActiveOutputHandle(id, null);
      } else {
        // Non-structural edit: avoid touching targetBranches to prevent redundant updates
        setLocalData({ cases: next });
      }
    },
    [branches, id, setLocalData, updateNodeInternals]
  );

  // Ensure pending rAF gets cleaned on unmount
  React.useEffect(() => {
    return () => {
      if (updateRafRef.current) cancelAnimationFrame(updateRafRef.current);
    };
  }, []);

  const addCase = useCallback(() => {
    if (readOnly) return;
    const next = [...cases, newCase()];
    setCases(next, { structural: true });
  }, [cases, setCases, readOnly]);

  const removeCase = useCallback(
    (caseId: string) => {
      if (readOnly) return;
      const next = cases.filter(c => c.case_id !== caseId);
      setCases(next, { structural: true });
      // TODO: remove edges from this case's source handle if exist
    },
    [cases, setCases, readOnly]
  );

  const toggleCaseLogic = useCallback(
    (caseId: string) => {
      if (readOnly) return;
      const next = cases.map(c => {
        if (c.case_id !== caseId) return c;
        const newOp: 'and' | 'or' = c.logical_operator === 'and' ? 'or' : 'and';
        return { ...c, logical_operator: newOp } as CaseItem;
      });
      setCases(next);
    },
    [cases, setCases, readOnly]
  );

  const casesRef = useRef<CaseItem[]>(cases);
  React.useEffect(() => {
    casesRef.current = cases;
  }, [cases]);

  const addCondition = useCallback(
    (caseId: string, varType: VarType) => {
      if (readOnly) return;
      const prev = casesRef.current;
      const next = prev.map(c =>
        c.case_id === caseId ? { ...c, conditions: [...c.conditions, newCondition(varType)] } : c
      );
      setCases(next);
    },
    [setCases, readOnly]
  );

  const removeCondition = useCallback(
    (caseId: string, conditionId: string) => {
      if (readOnly) return;
      const prev = casesRef.current;
      const next = prev.map(c =>
        c.case_id === caseId
          ? { ...c, conditions: c.conditions.filter(cd => cd.id !== conditionId) }
          : c
      );
      setCases(next);
    },
    [setCases, readOnly]
  );

  const updateCondition = useCallback(
    (caseId: string, conditionId: string, patch: Partial<Condition>) => {
      if (readOnly) return;
      const prev = casesRef.current;
      const next = prev.map(c => {
        if (c.case_id !== caseId) return c;
        return {
          ...c,
          conditions: c.conditions.map(cd => (cd.id === conditionId ? { ...cd, ...patch } : cd)),
        };
      });
      setCases(next);
    },
    [setCases, readOnly]
  );

  // Row is extracted into a memoized component above


  return (
    <div className="space-y-4">
      {cases.map((ci, idx) => (
        <CasePanel
          key={ci.case_id}
          nodeId={id}
          item={ci}
          idx={idx}
          readOnly={readOnly}
          onToggleLogic={toggleCaseLogic}
          onAddCondition={addCondition}
          onRemoveCase={removeCase}
          onUpdateCondition={updateCondition}
          onRemoveCondition={removeCondition}
        />
      ))}
      <div>
        <Button
          className="h-8 w-full text-xs"
          variant="secondary"
          onClick={addCase}
          disabled={readOnly}
        >
          {t('nodes.ifElse.fields.addBranch')}
        </Button>
      </div>
    </div>
  );
};

export default IfElseManager;

