'use client';

import React from 'react';
import { AlertTriangle, ArrowRight, CheckCircle2, Info, Play } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import type { ScopedTranslations } from '@/i18n/translations';
import { useRunWorkflowNodeDraft } from '@/hooks/workflow/use-run-workflow-node-draft';
import { getErrorMessage } from '@/utils/error-notifications';
import { AgentType } from '@/services/types/agent';
import type { WorkflowEdge, WorkflowNode } from '../store/type';
import { useWorkflowStore } from '../store';
import { useActivePanel } from '../hooks/use-active-panel';

const UNSUPPORTED_SINGLE_NODE_TYPES = new Set<string>([
  'start',
  'end',
  'answer',
  'if-else',
  'iteration',
  'iteration-start',
  'loop',
  'loop-start',
  'loop-end',
  'approval',
  'note',
]);

const SIDE_EFFECT_SINGLE_NODE_TYPES = new Set<string>([
  'http-request',
  'tools',
  'notification-sms',
  'create-scheduled-task',
  'call-database',
  'image-gen',
]);

interface NodeDebugActionsProps {
  agentId: string | null;
  node: WorkflowNode;
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  readOnly: boolean;
}

interface NodeDebugState {
  disabledReason: string | null;
  description: string;
  inputs: Record<string, unknown>;
}

function getNodeDebugState({
  agentId,
  node,
  nodes,
  edges,
  readOnly,
  lastDebugInputs,
  t,
}: NodeDebugActionsProps & {
  lastDebugInputs: Record<string, unknown> | null;
  t: ScopedTranslations<'agents'>;
}): NodeDebugState {
  const baseState: NodeDebugState = {
    disabledReason: null,
    description: t('workflow.nodeDebug.description'),
    inputs: {},
  };

  if (readOnly) return { ...baseState, disabledReason: t('workflow.nodeDebug.readonly') };
  if (!agentId) return { ...baseState, disabledReason: t('workflow.nodeDebug.missingAgent') };
  if (UNSUPPORTED_SINGLE_NODE_TYPES.has(node.data.type)) {
    return { ...baseState, disabledReason: t('workflow.nodeDebug.unsupported') };
  }
  if (SIDE_EFFECT_SINGLE_NODE_TYPES.has(node.data.type)) {
    return { ...baseState, disabledReason: t('workflow.nodeDebug.sideEffectUnsupported') };
  }

  const incomingEdges = edges.filter(edge => edge.target === node.id);
  if (incomingEdges.length === 0) {
    return baseState;
  }

  const nodeById = new Map(nodes.map(item => [item.id, item]));
  const onlyStartUpstream = incomingEdges.every(
    edge => nodeById.get(edge.source)?.data.type === 'start'
  );

  if (onlyStartUpstream) {
    if (!lastDebugInputs || Object.keys(lastDebugInputs).length === 0) {
      return { ...baseState, disabledReason: t('workflow.nodeDebug.missingStartContext') };
    }
    return {
      ...baseState,
      description: t('workflow.nodeDebug.reuseStartContext'),
      inputs: lastDebugInputs,
    };
  }

  return { ...baseState, disabledReason: t('workflow.nodeDebug.hasUpstream') };
}

export function NodeDebugActions({ agentId, node, nodes, edges, readOnly }: NodeDebugActionsProps) {
  const t = useT('agents');
  const { mutateAsync, isPending, isError, reset } = useRunWorkflowNodeDraft();
  const lastDebugInputs = useWorkflowStore.use.lastDebugInputs();
  const agentType = useWorkflowStore.use.agentType();
  const enterHistoryMode = useWorkflowStore.use.enterHistoryMode();
  const setActivePanel = useActivePanel(state => state.setActive);
  const [lastRunId, setLastRunId] = React.useState<string | null>(null);
  const [lastRunSucceeded, setLastRunSucceeded] = React.useState(false);
  const [lastRunFailed, setLastRunFailed] = React.useState(false);

  React.useEffect(() => {
    setLastRunId(null);
    setLastRunSucceeded(false);
    setLastRunFailed(false);
    reset();
  }, [node.id, reset]);

  const { disabledReason, description, inputs } = getNodeDebugState({
    agentId,
    node,
    nodes,
    edges,
    readOnly,
    lastDebugInputs,
    t,
  });
  const isDisabled = Boolean(disabledReason) || isPending;

  const handleViewRecord = React.useCallback(() => {
    if (!lastRunId) return;
    enterHistoryMode(lastRunId);
    setActivePanel(agentType === AgentType.CONVERSATIONAL_AGENT ? 'conversation-history' : 'run');
  }, [agentType, enterHistoryMode, lastRunId, setActivePanel]);

  const handleRun = async () => {
    if (!agentId || disabledReason) return;

    setLastRunId(null);
    setLastRunSucceeded(false);
    setLastRunFailed(false);

    try {
      const result = await mutateAsync({
        agentId,
        nodeId: node.id,
        payload: { inputs },
      });
      const status = String(result.status ?? '').toLowerCase();
      const failed = Boolean(result.error) || status === 'failed' || status === 'error';
      setLastRunId(result.workflow_run_id ?? null);
      setLastRunSucceeded(!failed);
      setLastRunFailed(failed);
      if (failed) {
        toast.error(result.error ?? t('workflow.nodeDebug.failed'));
        return;
      }
      toast.success(t('workflow.nodeDebug.success'));
    } catch (error) {
      setLastRunId(null);
      setLastRunSucceeded(false);
      setLastRunFailed(true);
      toast.error(getErrorMessage(error) || t('workflow.nodeDebug.failed'));
    }
  };

  return (
    <section
      className={cn(
        'rounded-lg border px-3 py-2.5',
        disabledReason ? 'border-border bg-muted/30' : 'border-primary/20 bg-primary/[0.03]'
      )}
    >
      <div className="flex items-start gap-2">
        <div
          className={cn(
            'mt-0.5 flex size-5 items-center justify-center rounded-full',
            disabledReason ? 'bg-muted text-muted-foreground' : 'bg-primary/10 text-primary'
          )}
        >
          {lastRunFailed ? (
            <AlertTriangle className="size-3.5" />
          ) : lastRunSucceeded ? (
            <CheckCircle2 className="size-3.5" />
          ) : (
            <Info className="size-3.5" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-2">
            <div className="min-w-0">
              <h3 className="text-xs font-semibold text-foreground">
                {t('workflow.nodeDebug.title')}
              </h3>
              <p className="mt-0.5 text-[11px] leading-4 text-muted-foreground">
                {isPending ? t('workflow.nodeDebug.running') : (disabledReason ?? description)}
              </p>
            </div>
            <Button
              type="button"
              size="xs"
              variant="outline"
              className="h-7 shrink-0"
              onClick={handleRun}
              loading={isPending}
              disabled={isDisabled}
            >
              <Play className="size-3.5" />
              {t('workflow.nodeDebug.run')}
            </Button>
          </div>
          {lastRunSucceeded && (
            <div className="mt-2 flex flex-wrap items-center justify-between gap-2 rounded-md border border-primary/15 bg-background px-2 py-1.5 text-[11px] leading-4 text-primary">
              <div className="flex min-w-0 items-center gap-1.5">
                <CheckCircle2 className="size-3.5 shrink-0" />
                <span className="min-w-0">
                  {lastRunId
                    ? t('workflow.nodeDebug.recordCreated')
                    : t('workflow.nodeDebug.recordUnavailable')}
                </span>
              </div>
              {lastRunId && (
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  className="h-6 shrink-0 px-2 text-[11px] text-primary hover:text-primary"
                  onClick={handleViewRecord}
                >
                  {t('workflow.nodeDebug.viewRecord')}
                  <ArrowRight className="size-3" />
                </Button>
              )}
            </div>
          )}
          {(lastRunFailed || (isError && !lastRunId)) && (
            <div className="mt-2 flex items-center gap-1.5 text-[11px] leading-4 text-destructive">
              <AlertTriangle className="size-3.5" />
              <span>{t('workflow.nodeDebug.failed')}</span>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

export default NodeDebugActions;
