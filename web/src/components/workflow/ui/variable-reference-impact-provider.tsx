'use client';

import React, { createContext, useCallback, useContext, useMemo, useState } from 'react';
import type { EdgeChange } from '@xyflow/react';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Checkbox } from '@/components/ui/checkbox';
import { useT } from '@/i18n';
import { useWorkflowStore } from '../store';
import type { WorkflowEdge } from '../store/type';
import {
  findNewlyInvalidWorkflowVariableReferences,
  simulateWorkflowNodeDeletion,
  type WorkflowVariableReferenceImpact,
} from '../common/variable-reference-health';

interface PendingImpact {
  impacts: WorkflowVariableReferenceImpact[];
  action: () => void;
}

interface WorkflowVariableImpactContextValue {
  requestNodeDeletion: (nodeIds: string[], action: () => void) => boolean;
  requestEdgeChanges: (changes: Array<EdgeChange<WorkflowEdge>>, action: () => void) => boolean;
}

const WorkflowVariableImpactContext = createContext<WorkflowVariableImpactContextValue>({
  requestNodeDeletion: (_nodeIds, action) => {
    action();
    return true;
  },
  requestEdgeChanges: (_changes, action) => {
    action();
    return true;
  },
});

export function useWorkflowVariableImpact() {
  return useContext(WorkflowVariableImpactContext);
}

export function WorkflowVariableImpactProvider({ children }: { children: React.ReactNode }) {
  const t = useT();
  const [pending, setPending] = useState<PendingImpact | null>(null);
  const [skipFutureWarnings, setSkipFutureWarnings] = useState(false);
  const [suppressNodeDeletionWarnings, setSuppressNodeDeletionWarnings] = useState(false);

  const inspect = useCallback(
    (
      afterNodes: ReturnType<typeof useWorkflowStore.getState>['nodes'],
      afterEdges: ReturnType<typeof useWorkflowStore.getState>['edges'],
      action: () => void
    ) => {
      const state = useWorkflowStore.getState();
      const impacts = findNewlyInvalidWorkflowVariableReferences({
        beforeNodes: state.nodes,
        beforeEdges: state.edges,
        afterNodes,
        afterEdges,
        agentType: state.agentType,
      });
      if (impacts.length === 0) {
        action();
        return true;
      }
      setSkipFutureWarnings(false);
      setPending({ impacts, action });
      return false;
    },
    []
  );

  const requestNodeDeletion = useCallback(
    (nodeIds: string[], action: () => void) => {
      if (suppressNodeDeletionWarnings) {
        action();
        return true;
      }
      const state = useWorkflowStore.getState();
      const after = simulateWorkflowNodeDeletion(state.nodes, state.edges, nodeIds);
      return inspect(after.nodes, after.edges, action);
    },
    [inspect, suppressNodeDeletionWarnings]
  );

  const requestEdgeChanges = useCallback(
    (_changes: Array<EdgeChange<WorkflowEdge>>, action: () => void) => {
      // Disconnecting is reversible. Preserve the selector and let the shared
      // variable badge/validation state explain that its source is unreachable.
      action();
      return true;
    },
    []
  );

  const value = useMemo(
    () => ({ requestNodeDeletion, requestEdgeChanges }),
    [requestEdgeChanges, requestNodeDeletion]
  );

  const preview = pending?.impacts.slice(0, 5) ?? [];
  const extraCount = pending ? Math.max(0, pending.impacts.length - preview.length) : 0;

  return (
    <WorkflowVariableImpactContext.Provider value={value}>
      {children}
      <ConfirmDialog
        open={Boolean(pending)}
        onOpenChange={open => {
          if (!open) {
            setPending(null);
            setSkipFutureWarnings(false);
          }
        }}
        variant="warning"
        title={t('nodes.validation.deleteVariableSourceTitle')}
        description={
          pending ? (
            <div className="space-y-3">
              <p>
                {t('nodes.validation.variableImpactDescription', { count: pending.impacts.length })}
              </p>
              <div className="max-h-48 space-y-1.5 overflow-y-auto rounded-lg border bg-background p-3 text-foreground">
                {preview.map(impact => (
                  <div
                    key={`${impact.consumerNodeId}:${impact.selector.join('.')}`}
                    className="flex min-w-0 items-center justify-between gap-3 text-sm"
                  >
                    <span className="truncate font-medium">
                      {impact.consumerTitle || t('nodes.validation.affectedNode')}
                    </span>
                    <span className="shrink-0 text-muted-foreground">
                      {impact.sourceNode?.data?.title ||
                        t('nodes.validation.deletedVariableSource')}{' '}
                      ({impact.keyPath.join('.')})
                    </span>
                  </div>
                ))}
                {extraCount > 0 ? (
                  <p className="text-xs text-muted-foreground">
                    {t('nodes.validation.moreVariableImpacts', { count: extraCount })}
                  </p>
                ) : null}
              </div>
              <p className="text-xs text-muted-foreground">
                {t('nodes.validation.variableImpactPreserved')}
              </p>
              <label className="flex cursor-pointer items-center gap-2 rounded-md border bg-muted/20 px-3 py-2 text-sm text-foreground">
                <Checkbox
                  checked={skipFutureWarnings}
                  onCheckedChange={checked => setSkipFutureWarnings(checked === true)}
                />
                <span>{t('nodes.validation.doNotWarnAgainThisEdit')}</span>
              </label>
            </div>
          ) : null
        }
        confirmText={t('nodes.validation.continueOperation')}
        cancelText={t('nodes.common.cancel')}
        onConfirm={() => {
          const action = pending?.action;
          if (skipFutureWarnings) setSuppressNodeDeletionWarnings(true);
          setPending(null);
          setSkipFutureWarnings(false);
          action?.();
        }}
      />
    </WorkflowVariableImpactContext.Provider>
  );
}
