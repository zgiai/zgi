'use client';

import { useState } from 'react';
import { Ban, Bot, BookOpenText, Brain, Hash, ListChecks, Loader2, MessageSquareText, ScrollText, ShieldAlert, Sparkles, Workflow, Wrench } from 'lucide-react';
import { getAgentRuntimeStepDisplay } from '@/app/console/agents/[agentId]/logs/_components/agent-runtime-step-display';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { RunStatusBadge } from '@/components/workflow/ui/run-status-badge';
import { DigitalHumanBackgroundRunFiles } from '@/components/digital-human/digital-human-background-run-files';
import { DigitalHumanRunDetailDrawer } from '@/components/digital-human/digital-human-run-detail-drawer';
import { isActiveDigitalHumanRunStatus } from '@/components/digital-human/digital-human-run-presentation';
import {
  useDigitalHumanMutations,
  useDigitalHumanRun,
  useDigitalHumanRunRuntimeLog,
} from '@/hooks/digital-human/use-digital-humans';
import { useT } from '@/i18n';
import { formatDate, formatWorkflowElapsedMs } from '@/utils/format';

function stepIcon(type: string) {
  if (type === 'user_input') return MessageSquareText;
  if (type === 'model_call' || type === 'model_answer') return Bot;
  if (type === 'tool_call' || type === 'tool') return Wrench;
  if (type === 'skill_load' || type === 'skill') return Sparkles;
  if (type === 'workflow_run' || type === 'workflow_node' || type === 'workflow_approval') return Workflow;
  if (type === 'task_approval') return ListChecks;
  if (type === 'reference_read') return BookOpenText;
  if (type === 'memory_mutation') return Brain;
  if (type === 'guardrail') return ShieldAlert;
  return Hash;
}

/**
 * A task run's progress where the task is read: its steps, result files, and
 * whether it is still working. The full log, with model inputs and approvals,
 * opens over the page for those who need to dig in.
 */
export function DigitalHumanRunTimeline({ digitalHumanId, runId }: { digitalHumanId: string; runId: string }) {
  const t = useT('digitalHumans');
  const tLogs = useT('webapp');
  const runQuery = useDigitalHumanRun(digitalHumanId, runId);
  const run = runQuery.data?.data ?? null;
  const active = run ? isActiveDigitalHumanRunStatus(run.status) : false;
  // While a run is active the log keeps refreshing, so progress shows live.
  const logQuery = useDigitalHumanRunRuntimeLog(digitalHumanId, runId, true, !run || active);
  const steps = logQuery.data?.data?.steps ?? [];
  const { cancelRun } = useDigitalHumanMutations(digitalHumanId);
  const [fullLogOpen, setFullLogOpen] = useState(false);
  const cancel = async (id: string) => {
    await cancelRun.mutateAsync({ digitalHumanId, runId: id });
  };

  if (runQuery.isLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-6 w-40" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }
  if (!run) return <p className="text-sm text-muted-foreground">{t('runs.detailErrorDescription')}</p>;

  const waitingApproval = run.status === 'waiting' && run.waiting_reason === 'waiting_approval';
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <RunStatusBadge status={run.status} />
        <span>{formatDate(run.started_at ?? run.created_at)}</span>
        {active ? <Loader2 className="size-3.5 animate-spin" /> : null}
      </div>
      {run.error_message ? (
        <p className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{run.error_message}</p>
      ) : null}
      {waitingApproval ? (
        <p className="rounded-md border border-warning/25 bg-warning/10 p-3 text-xs">{t('tasks.runTrace.waitingApproval')}</p>
      ) : null}

      <section>
        <h4 className="text-sm font-semibold">{t('tasks.runTrace.steps')}</h4>
        {logQuery.isLoading && steps.length === 0 ? (
          <Skeleton className="mt-2 h-20 w-full" />
        ) : steps.length === 0 ? (
          <p className="mt-2 text-xs text-muted-foreground">{t('tasks.runTrace.noSteps')}</p>
        ) : (
          <ol className="mt-2 space-y-1.5">
            {steps.map(step => {
              const Icon = stepIcon(step.type);
              const display = getAgentRuntimeStepDisplay(step, tLogs);
              return (
                <li key={step.id} className="flex items-start gap-2 rounded-md border px-3 py-2">
                  <span className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-md bg-muted">
                    <Icon className="size-3.5 text-muted-foreground" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm">{display.title}</span>
                    {display.subtitle || typeof step.elapsed_time === 'number' ? (
                      <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                        {display.subtitle}
                        {display.subtitle && typeof step.elapsed_time === 'number' ? ' · ' : ''}
                        {typeof step.elapsed_time === 'number' ? formatWorkflowElapsedMs(step.elapsed_time) : ''}
                      </span>
                    ) : null}
                    {step.error ? <span className="mt-1 block text-xs text-destructive">{step.error}</span> : null}
                  </span>
                  <RunStatusBadge status={step.status} />
                </li>
              );
            })}
          </ol>
        )}
      </section>

      {!active ? <DigitalHumanBackgroundRunFiles digitalHumanId={digitalHumanId} run={run} /> : null}

      <div className="flex flex-wrap gap-2">
        {active ? (
          <Button size="sm" variant="outline" disabled={cancelRun.isPending || run.status === 'canceling'} onClick={() => void cancel(run.id)}>
            <Ban className="size-4" />
            {run.status === 'canceling' ? t('runs.canceling') : t('runs.cancel')}
          </Button>
        ) : null}
        <Button size="sm" variant="ghost" onClick={() => setFullLogOpen(true)}>
          <ScrollText className="size-4" />
          {t('tasks.runTrace.fullLog')}
        </Button>
      </div>
      <DigitalHumanRunDetailDrawer
        digitalHumanId={digitalHumanId}
        runId={runId}
        open={fullLogOpen}
        onOpenChange={setFullLogOpen}
        onCancel={cancel}
        cancelPending={cancelRun.isPending}
      />
    </div>
  );
}
