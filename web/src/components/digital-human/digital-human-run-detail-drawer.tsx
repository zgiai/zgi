'use client';

import { useMemo } from 'react';
import { Ban, Loader2 } from 'lucide-react';
import { AgentRuntimeLogDetailDrawer } from '@/app/console/agents/[agentId]/logs/_components/agent-runtime-log-detail-drawer';
import { Button } from '@/components/ui/button';
import {
  useDigitalHumanRun,
  useDigitalHumanRunRuntimeLog,
} from '@/hooks/digital-human/use-digital-humans';
import { useT } from '@/i18n';
import type { AgentRuntimeRunItem } from '@/services/types/agent-runtime-log';
import type { DigitalHumanRun } from '@/services/types/digital-human';

interface DigitalHumanRunDetailDrawerProps {
  digitalHumanId: string;
  runId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCancel: (runId: string) => Promise<void>;
  cancelPending: boolean;
}

export function DigitalHumanRunDetailDrawer({
  digitalHumanId,
  runId,
  open,
  onOpenChange,
  onCancel,
  cancelPending,
}: DigitalHumanRunDetailDrawerProps) {
  const t = useT('digitalHumans');
  const runQuery = useDigitalHumanRun(digitalHumanId, runId, open);
  const run = runQuery.data?.data ?? null;
  const active = run ? ['queued', 'running', 'canceling'].includes(run.status) : false;
  const runtimeQuery = useDigitalHumanRunRuntimeLog(digitalHumanId, runId, open, !run || active);
  const runtimeLog = runtimeQuery.data?.data;
  const runtimeSteps = useMemo(
    () =>
      (runtimeLog?.steps ?? []).map(step =>
        step.type === 'channel_context' ? { ...step, title: t('runs.channelContext.title') } : step
      ),
    [runtimeLog?.steps, t]
  );
  const selectedRun = useMemo(() => (run ? toRuntimeRunItem(run) : null), [run]);
  const error =
    runQuery.isError || runtimeQuery.isError ? t('runs.detailErrorDescription') : undefined;

  return (
    <AgentRuntimeLogDetailDrawer
      open={open}
      onOpenChange={onOpenChange}
      selectedRun={selectedRun}
      detail={runtimeLog?.detail ?? null}
      steps={runtimeSteps}
      isLoading={runQuery.isLoading || runtimeQuery.isLoading}
      error={error}
      sourceLabel={run ? runSourceLabel(run, t) : null}
      footer={
        run && active ? (
          <div className="flex justify-end">
            <Button
              variant="outline"
              onClick={() => void onCancel(run.id)}
              disabled={cancelPending || run.status === 'canceling'}
            >
              {cancelPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Ban className="size-4" />
              )}
              {run.status === 'canceling' ? t('runs.canceling') : t('runs.cancel')}
            </Button>
          </div>
        ) : null
      }
    />
  );
}

function toRuntimeRunItem(run: DigitalHumanRun): AgentRuntimeRunItem {
  return {
    id: run.message_id || run.id,
    conversation_id: run.conversation_id || '',
    status: run.status,
    query: run.query,
    answer_preview: run.result_summary,
    elapsed_time: runElapsedMilliseconds(run),
    created_at: unixSeconds(run.created_at),
    finished_at: run.completed_at ? unixSeconds(run.completed_at) : null,
    source: normalizedRunSource(run),
  };
}

function runElapsedMilliseconds(run: DigitalHumanRun) {
  const start = Date.parse(run.started_at ?? run.created_at);
  const end = Date.parse(run.completed_at ?? run.updated_at);
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return undefined;
  return end - start;
}

function unixSeconds(value: string) {
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) ? Math.floor(timestamp / 1_000) : undefined;
}

type DigitalHumanTranslator = ReturnType<typeof useT<'digitalHumans'>>;

function runSourceLabel(run: DigitalHumanRun, t: DigitalHumanTranslator) {
  return t(`runs.sources.${normalizedRunSource(run)}`);
}

function normalizedRunSource(run: DigitalHumanRun) {
  const origin = isRecord(run.invocation_event?.origin) ? run.invocation_event.origin : null;
  const source =
    (typeof origin?.kind === 'string' && origin.kind) ||
    (typeof run.invocation_event?.type === 'string' && run.invocation_event.type) ||
    'manual';
  if (source === 'schedule' || source === 'scheduled') return 'scheduled' as const;
  if (source === 'email') return 'email' as const;
  if (source === 'channel' || source === 'chat') return 'channel' as const;
  if (source === 'manual') return 'manual' as const;
  return 'external' as const;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}
