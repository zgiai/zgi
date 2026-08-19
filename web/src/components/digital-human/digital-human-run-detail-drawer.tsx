'use client';

import type { ComponentType } from 'react';
import {
  AlertCircle,
  Ban,
  Bot,
  CheckCircle2,
  Clock3,
  Copy,
  FileOutput,
  Loader2,
  MessageSquareText,
  PackageCheck,
  ShieldCheck,
  Workflow,
  Wrench,
} from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { RunStatusBadge } from '@/components/workflow/ui/run-status-badge';
import { useDigitalHumanRun } from '@/hooks/digital-human/use-digital-humans';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { DigitalHumanRun, DigitalHumanRunStatus } from '@/services/types/digital-human';

interface DigitalHumanRunDetailDrawerProps {
  digitalHumanId: string;
  runId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCancel: (runId: string) => Promise<void>;
  cancelPending: boolean;
}

interface TimelineStep {
  id: string;
  icon: ComponentType<{ className?: string }>;
  label: string;
  title: string;
  detail?: string;
  status?: string;
  tone?: 'default' | 'success' | 'error' | 'active';
}

interface RunSourceLabels {
  manual: string;
  scheduled: string;
  email: string;
  channel: string;
  external: string;
}

interface TimelineLabels {
  trigger: string;
  context: string;
  tool: string;
  workflow: string;
  artifact: string;
  result: string;
  publishedContext: string;
  unnamedTool: string;
  unnamedWorkflow: string;
  unnamedArtifact: string;
  resultPending: string;
  sources: RunSourceLabels;
  statuses: Record<DigitalHumanRunStatus, string>;
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
  const query = useDigitalHumanRun(digitalHumanId, runId, open);
  const run = query.data?.data;
  const labels: TimelineLabels = {
    trigger: t('runs.timeline.trigger'),
    context: t('runs.timeline.context'),
    tool: t('runs.timeline.tool'),
    workflow: t('runs.timeline.workflow'),
    artifact: t('runs.timeline.artifact'),
    result: t('runs.timeline.result'),
    publishedContext: t('runs.timeline.publishedContext'),
    unnamedTool: t('runs.timeline.unnamedTool'),
    unnamedWorkflow: t('runs.timeline.unnamedWorkflow'),
    unnamedArtifact: t('runs.timeline.unnamedArtifact'),
    resultPending: t('runs.timeline.resultPending'),
    sources: {
      manual: t('runs.sources.manual'),
      scheduled: t('runs.sources.scheduled'),
      email: t('runs.sources.email'),
      channel: t('runs.sources.channel'),
      external: t('runs.sources.external'),
    },
    statuses: {
      queued: t('status.queued'),
      running: t('status.running'),
      canceling: t('status.canceling'),
      succeeded: t('status.succeeded'),
      failed: t('status.failed'),
      canceled: t('status.canceled'),
      unknown: t('status.unknown'),
    },
  };
  const steps = run ? buildTimeline(run, labels) : [];
  const active = run ? ['queued', 'running', 'canceling'].includes(run.status) : false;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="flex w-full flex-col gap-0 p-0 sm:max-w-3xl" showClose={false}>
        <SheetHeader className="shrink-0 border-b px-5 py-4 text-left">
          <div className="flex min-w-0 items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <SheetTitle className="text-base">{t('runs.detailTitle')}</SheetTitle>
                {run ? (
                  <RunStatusBadge status={run.status} label={t(`status.${run.status}`)} />
                ) : null}
                {run ? <SourceBadge label={runSourceLabel(run, labels.sources)} /> : null}
              </div>
              <SheetDescription className="mt-1 truncate" title={runId}>
                {runId || t('runs.loadingDetail')}
              </SheetDescription>
            </div>
            <SheetClose asChild>
              <Button type="button" variant="outline" size="xs" className="shrink-0">
                {t('runs.close')}
              </Button>
            </SheetClose>
          </div>
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {query.isLoading || (!run && !query.isError) ? (
            <div className="flex min-h-80 items-center justify-center text-sm text-muted-foreground">
              <Loader2 className="mr-2 size-4 animate-spin" />
              {t('runs.loadingDetail')}
            </div>
          ) : query.isError || !run ? (
            <div className="m-5 flex min-h-64 flex-col items-center justify-center rounded-xl border border-dashed px-6 text-center">
              <AlertCircle className="size-8 text-destructive" />
              <div className="mt-4 text-sm font-semibold">{t('runs.detailErrorTitle')}</div>
              <div className="mt-1 text-sm text-muted-foreground">
                {t('runs.detailErrorDescription')}
              </div>
              <Button
                className="mt-4"
                size="sm"
                variant="outline"
                onClick={() => void query.refetch()}
              >
                {t('runs.retry')}
              </Button>
            </div>
          ) : (
            <>
              <section className="border-b px-5 py-5">
                <div className="rounded-xl border bg-muted/10 p-4">
                  <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {t('runs.query')}
                  </div>
                  <div className="mt-2 whitespace-pre-wrap break-words text-sm leading-6">
                    {run.query}
                  </div>
                </div>
                <div className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                  <Metric
                    icon={Clock3}
                    label={t('runs.columns.created')}
                    value={new Date(run.created_at).toLocaleString()}
                  />
                  <Metric
                    icon={PackageCheck}
                    label={t('runs.columns.steps')}
                    value={String(steps.length)}
                  />
                  <Metric
                    icon={Bot}
                    label={t('runs.columns.duration')}
                    value={formatRunDuration(run)}
                  />
                  <Metric
                    icon={MessageSquareText}
                    label={t('runs.columns.conversation')}
                    value={shortID(run.conversation_id)}
                  />
                </div>
              </section>

              <section className="border-b px-5 py-5">
                <div className="mb-4 text-sm font-semibold">{t('runs.executionTimeline')}</div>
                <div className="space-y-0">
                  {steps.map((step, index) => (
                    <TimelineItem key={step.id} step={step} last={index === steps.length - 1} />
                  ))}
                </div>
              </section>

              {run.result_summary || run.error_message ? (
                <section className="border-b px-5 py-5">
                  <div className="mb-3 text-sm font-semibold">
                    {run.error_message ? t('runs.error') : t('runs.result')}
                  </div>
                  <div
                    className={cn(
                      'whitespace-pre-wrap break-words rounded-xl border p-4 text-sm leading-6',
                      run.error_message
                        ? 'border-destructive/25 bg-destructive/5 text-destructive'
                        : 'bg-muted/10'
                    )}
                  >
                    {run.error_message || run.result_summary}
                  </div>
                </section>
              ) : null}

              <section className="px-5 py-5">
                <details className="group rounded-xl border bg-muted/10">
                  <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-4 py-3 text-sm font-semibold">
                    {t('common.technicalDetails')}
                    <span className="text-xs font-normal text-muted-foreground group-open:hidden">
                      {t('runs.expand')}
                    </span>
                  </summary>
                  <div className="grid gap-4 border-t p-4 sm:grid-cols-2">
                    <CopyableFact label={t('runs.identifiers.run')} value={run.id} />
                    <CopyableFact
                      label={t('runs.identifiers.version')}
                      value={run.digital_human_version_id}
                    />
                    <CopyableFact
                      label={t('runs.identifiers.runSpec')}
                      value={run.run_spec_revision}
                    />
                    <CopyableFact
                      label={t('runs.identifiers.snapshot')}
                      value={run.execution_snapshot_revision}
                    />
                    <CopyableFact
                      label={t('runs.identifiers.conversation')}
                      value={run.conversation_id}
                    />
                    <CopyableFact label={t('runs.identifiers.message')} value={run.message_id} />
                    <CopyableFact label={t('runs.identifiers.errorCode')} value={run.error_code} />
                    <CopyableFact
                      label={t('runs.identifiers.completedAt')}
                      value={
                        run.completed_at ? new Date(run.completed_at).toLocaleString() : undefined
                      }
                    />
                  </div>
                </details>
              </section>
            </>
          )}
        </div>

        {run && active ? (
          <div className="flex shrink-0 justify-end border-t px-5 py-4">
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
        ) : null}
      </SheetContent>
    </Sheet>
  );
}

function TimelineItem({ step, last }: { step: TimelineStep; last: boolean }) {
  const Icon = step.icon;
  return (
    <div className="relative flex gap-3 pb-5 last:pb-0">
      {!last ? <div className="absolute bottom-0 left-[17px] top-9 w-px bg-border" /> : null}
      <div
        className={cn(
          'relative z-10 flex size-9 shrink-0 items-center justify-center rounded-full border bg-background shadow-sm',
          step.tone === 'success' && 'border-emerald-500/30 text-emerald-600',
          step.tone === 'error' && 'border-destructive/30 text-destructive',
          step.tone === 'active' && 'border-sky-500/30 text-sky-600',
          (!step.tone || step.tone === 'default') && 'text-muted-foreground'
        )}
      >
        <Icon className="size-4" />
      </div>
      <div className="min-w-0 flex-1 rounded-xl border bg-background p-3">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div className="min-w-0">
            <div className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
              {step.label}
            </div>
            <div className="mt-1 break-words text-sm font-medium">{step.title}</div>
          </div>
          {step.status ? <RunStatusBadge status={step.status} /> : null}
        </div>
        {step.detail ? (
          <div className="mt-1.5 whitespace-pre-wrap break-words text-xs leading-5 text-muted-foreground">
            {step.detail}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function Metric({
  icon: Icon,
  label,
  value,
}: {
  icon: ComponentType<{ className?: string }>;
  label: string;
  value: string;
}) {
  return (
    <div className="min-w-0">
      <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        <Icon className="size-3.5" />
        {label}
      </div>
      <div className="mt-1 truncate text-sm" title={value}>
        {value}
      </div>
    </div>
  );
}

function CopyableFact({ label, value }: { label: string; value?: string | null }) {
  const t = useT('digitalHumans');
  const displayValue = value || '—';
  return (
    <div className="min-w-0">
      <div className="text-xs font-medium text-muted-foreground">{label}</div>
      <div className="mt-1 flex items-start gap-1">
        <code className="min-w-0 flex-1 break-all text-xs leading-5">{displayValue}</code>
        {value ? (
          <Button
            type="button"
            size="xs"
            variant="ghost"
            isIcon
            className="shrink-0"
            aria-label={t('runs.copyValue')}
            onClick={() => {
              void navigator.clipboard
                .writeText(value)
                .then(() => toast.success(t('runs.valueCopied')));
            }}
          >
            <Copy className="size-3.5" />
          </Button>
        ) : null}
      </div>
    </div>
  );
}

function buildTimeline(run: DigitalHumanRun, labels: TimelineLabels): TimelineStep[] {
  const skills = recordArray(run.execution_trace?.skill_invocations);
  const workflows = recordArray(run.execution_trace?.workflow_runs);
  const artifacts = [
    ...recordArray(run.execution_trace?.conversation_artifacts),
    ...recordArray(run.execution_trace?.generated_files),
  ];
  const resultTone =
    run.status === 'succeeded'
      ? 'success'
      : run.status === 'failed' || run.status === 'unknown'
        ? 'error'
        : ['queued', 'running', 'canceling'].includes(run.status)
          ? 'active'
          : 'default';

  return [
    {
      id: 'trigger',
      icon: MessageSquareText,
      label: labels.trigger,
      title: runSourceLabel(run, labels.sources),
      detail: run.query,
    },
    {
      id: 'context',
      icon: ShieldCheck,
      label: labels.context,
      title: labels.publishedContext,
      detail: `${shortID(run.digital_human_version_id)} · ${shortID(run.run_spec_revision)}`,
    },
    ...skills.map((item, index) => ({
      id: `skill:${stringValue(item.runtime_id) ?? index}`,
      icon: Wrench,
      label: labels.tool,
      title:
        [stringValue(item.skill_id), stringValue(item.tool_name)].filter(Boolean).join(' · ') ||
        labels.unnamedTool,
      detail: stringValue(item.error_code) ?? undefined,
      status: stringValue(item.status) ?? undefined,
    })),
    ...workflows.map((item, index) => ({
      id: `workflow:${stringValue(item.workflow_run_id) ?? index}`,
      icon: Workflow,
      label: labels.workflow,
      title:
        [stringValue(item.workflow_id), stringValue(item.version)].filter(Boolean).join(' · ') ||
        labels.unnamedWorkflow,
      detail: stringValue(item.workflow_run_id) ?? undefined,
      status: stringValue(item.status) ?? undefined,
    })),
    ...artifacts.map((item, index) => ({
      id: `artifact:${stringValue(item.artifact_id) ?? stringValue(item.file_id) ?? index}`,
      icon: FileOutput,
      label: labels.artifact,
      title:
        stringValue(item.filename) ??
        stringValue(item.artifact_id) ??
        stringValue(item.file_id) ??
        labels.unnamedArtifact,
      detail: stringValue(item.mime_type) ?? stringValue(item.artifact_type) ?? undefined,
    })),
    {
      id: 'result',
      icon: run.status === 'failed' || run.status === 'unknown' ? AlertCircle : CheckCircle2,
      label: labels.result,
      title: labels.statuses[run.status],
      detail: run.error_message || run.result_summary || labels.resultPending,
      status: run.status,
      tone: resultTone,
    },
  ];
}

function SourceBadge({ label }: { label: string }) {
  return (
    <span className="inline-flex h-6 items-center rounded-full border bg-muted/30 px-2.5 text-xs font-medium text-muted-foreground">
      {label}
    </span>
  );
}

function runSourceLabel(run: DigitalHumanRun, labels: RunSourceLabels) {
  const origin = recordValue(run.invocation_event?.origin);
  const source = stringValue(origin?.kind) ?? stringValue(run.invocation_event?.type) ?? 'manual';
  if (source === 'manual') return labels.manual;
  if (source === 'schedule' || source === 'scheduled') return labels.scheduled;
  if (source === 'email') return labels.email;
  if (source === 'channel' || source === 'chat') return labels.channel;
  if (source === 'webhook' || source === 'external') return labels.external;
  return source;
}

function formatRunDuration(run: DigitalHumanRun) {
  const start = Date.parse(run.started_at ?? run.created_at);
  const end = Date.parse(run.completed_at ?? run.updated_at);
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return '—';
  const milliseconds = end - start;
  if (milliseconds < 1_000) return `${milliseconds} ms`;
  if (milliseconds < 60_000) return `${(milliseconds / 1_000).toFixed(1)} s`;
  return `${Math.floor(milliseconds / 60_000)}m ${Math.floor((milliseconds % 60_000) / 1_000)}s`;
}

function shortID(value?: string | null) {
  if (!value) return '—';
  if (value.length <= 13) return value;
  return `${value.slice(0, 8)}…${value.slice(-4)}`;
}

function recordArray(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value) ? value.filter(isRecord) : [];
}

function recordValue(value: unknown): Record<string, unknown> | null {
  return isRecord(value) ? value : null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function stringValue(value: unknown) {
  return typeof value === 'string' && value.trim() ? value.trim() : null;
}
