'use client';

import * as React from 'react';
import Link from 'next/link';
import { ArrowLeft, CheckCircle2, RefreshCcw, WandSparkles } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  useRetestWorkflowTestBatch,
  useWorkflowTestBatchItems,
  useWorkflowTestBatches,
  useWorkflowTestScenarios,
} from '@/hooks/workflow-test/use-workflow-test';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { getErrorMessage } from '@/utils/error-notifications';
import { formatWorkflowElapsedMs } from '@/utils/format';
import type { WorkflowTestBatch, WorkflowTestBatchItem } from '@/services/types/workflow-test';

interface BatchResultDetailProps {
  agentId: string;
  batchId: string;
  agentName?: string;
}

type BatchStatusKey = 'queued' | 'running' | 'completed' | 'stopped' | 'canceled';
type BatchItemStatusKey = 'pending' | 'running' | 'passed' | 'failed' | 'review' | 'canceled';
type SummaryKey = 'running' | 'allPassed' | 'hasIssues';

function itemStatusLabel(status: string, t: (key: BatchItemStatusKey) => string, none: string) {
  const map: Record<string, string> = {
    pending: t('pending'),
    running: t('running'),
    passed: t('passed'),
    failed: t('failed'),
    review: t('review'),
    canceled: t('canceled'),
  };
  return map[status] || status || none;
}

function itemStatusClass(status: string) {
  if (status === 'passed') return 'bg-emerald-50 text-emerald-700';
  if (status === 'failed') return 'bg-red-50 text-red-700';
  if (status === 'review') return 'bg-amber-50 text-amber-700';
  if (status === 'running') return 'bg-blue-50 text-blue-700';
  return 'bg-slate-100 text-slate-500';
}

function batchStatusLabel(status: string, t: (key: BatchStatusKey) => string, none: string) {
  const map: Record<string, string> = {
    queued: t('queued'),
    running: t('running'),
    completed: t('completed'),
    stopped: t('stopped'),
    canceled: t('canceled'),
  };
  return map[status] || status || none;
}

function stringifyOutput(outputs: Record<string, unknown>, none: string) {
  if (!outputs || Object.keys(outputs).length === 0) {
    return none;
  }
  const preferredKeys = ['answer', 'text', 'result', 'output'];
  for (const key of preferredKeys) {
    const value = outputs[key];
    if (typeof value === 'string' && value.trim()) {
      return value;
    }
  }
  return JSON.stringify(outputs, null, 2);
}

function formatResponseTime(item: WorkflowTestBatchItem, none: string) {
  const outputs = item.outputs || {};
  const elapsedKeys = ['elapsed_time', 'elapsed_ms', 'duration_ms', 'latency_ms', 'response_time_ms'];
  for (const key of elapsedKeys) {
    const value = outputs[key];
    if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
      return formatWorkflowElapsedMs(value);
    }
  }
  return none;
}

function stringifyValue(value: unknown, none: string) {
  if (value === undefined || value === null || value === '') {
    return none;
  }
  if (typeof value === 'string') {
    return value;
  }
  return JSON.stringify(value, null, 2);
}

function hasAttachments(item: WorkflowTestBatchItem) {
  return item.case_snapshot.turns?.some(turn => turn.attachments?.length) ?? false;
}

function buildSummary(
  batch: WorkflowTestBatch,
  items: WorkflowTestBatchItem[],
  t: (key: SummaryKey, values?: Record<string, string | number | Date>) => string
) {
  if (batch.summary) {
    return batch.summary;
  }
  if (batch.status !== 'completed') {
    return t('running');
  }
  const reviewItems = items.filter(item => item.status === 'review').length;
  if (batch.failed_count === 0 && reviewItems === 0) {
    return t('allPassed');
  }
  return t('hasIssues', { failed: batch.failed_count, review: reviewItems });
}

export function BatchResultDetail({ agentId, batchId, agentName }: BatchResultDetailProps) {
  const t = useT('agents.workflowTest.detail');
  const commonT = useT('agents.workflowTest.common');
  const batchStatusT = useT('agents.workflowTest.batchStatus');
  const summaryT = useT('agents.workflowTest.detail.summary');
  const itemStatusT = useT('agents.workflowTest.detail.itemStatus');
  const {
    data: batchesData,
    isLoading: batchesLoading,
    error: batchesError,
    refetch: refetchBatches,
  } = useWorkflowTestBatches(agentId);
  const {
    data: itemsData,
    isLoading: itemsLoading,
    error: itemsError,
    refetch: refetchItems,
  } = useWorkflowTestBatchItems(agentId, batchId);
  const { data: scenariosData } = useWorkflowTestScenarios(agentId);
  const retestBatch = useRetestWorkflowTestBatch(agentId);
  const [retestConfirmOpen, setRetestConfirmOpen] = React.useState(false);
  const batches = batchesData?.data?.items ?? [];
  const batch = batches.find(item => item.id === batchId);
  const items = itemsData?.data?.items ?? [];
  const scenarios = scenariosData?.data?.items ?? [];
  const isLoading = batchesLoading || itemsLoading;
  const error = batchesError || itemsError;
  const scenarioNameById = React.useMemo(() => {
    return new Map(scenarios.map(scenario => [scenario.id, scenario.name]));
  }, [scenarios]);
  const getScenarioName = React.useCallback(
    (item: WorkflowTestBatchItem) => {
      const scenarioId = item.case_snapshot.scenario_id;
      return scenarioId ? scenarioNameById.get(scenarioId) || commonT('none') : commonT('none');
    },
    [commonT, scenarioNameById]
  );
  const buildRetestName = React.useCallback(
    (batchName: string) => {
      const baseName = batchName.replace(/\s+重新测试\d*$/, '');
      const escapedBaseName = baseName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      const retestPattern = new RegExp(`^${escapedBaseName}\\s+重新测试(\\d+)?$`);
      const maxIndex = batches.reduce((max, existingBatch) => {
        const match = existingBatch.name.match(retestPattern);
        if (!match) return max;
        return Math.max(max, Number(match[1] || 1));
      }, 1);

      return commonT('retestName', { name: baseName, index: maxIndex + 1 });
    },
    [batches, commonT]
  );
  const executionErrorCount = items.filter(item => item.error).length;

  if (isLoading) {
    return (
      <div className="min-h-full bg-slate-50 p-8">
        <div className="mx-auto max-w-[1600px] space-y-6">
          <Skeleton className="h-56 rounded-2xl" />
          <Skeleton className="h-96 rounded-2xl" />
        </div>
      </div>
    );
  }

  if (error || !batch) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="w-full max-w-xl">
          <Alert variant="destructive">
            <AlertTitle>{t('loadFailedTitle')}</AlertTitle>
            <AlertDescription>
              {error ? getErrorMessage(error) || t('loadFailedFallback') : t('notFound')}
            </AlertDescription>
          </Alert>
          <Button
            className="mt-4"
            onClick={() => {
              void refetchBatches();
              void refetchItems();
            }}
          >
            <RefreshCcw className="mr-2 size-4" />
            {t('retry')}
          </Button>
        </div>
      </div>
    );
  }

  const finishedCount =
    batch.passed_count + batch.failed_count + batch.review_count + executionErrorCount;

  return (
    <div className="min-h-full bg-slate-50 px-8 py-8">
      <div className="mx-auto flex max-w-[1600px] flex-col gap-6">
        <div className="text-sm text-slate-500">
          {t('breadcrumb', { agentName: agentName || commonT('agentFallback') })}
        </div>

        <Card className="rounded-2xl">
          <CardContent className="p-6">
            <Button variant="ghost" size="sm" asChild className="mb-4 px-0 text-slate-600">
              <Link href={`/console/agents/${agentId}/batch-test/batches`}>
                <ArrowLeft className="mr-2 size-4" />
                {t('back')}
              </Link>
            </Button>
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-3">
                  <h1 className="text-2xl font-semibold text-slate-950">{batch.name}</h1>
                  <Badge className="bg-blue-50 text-blue-700">
                    {batchStatusLabel(batch.status, batchStatusT, commonT('none'))}
                  </Badge>
                </div>
                <div className="mt-4 grid max-w-[760px] grid-cols-3 gap-12 rounded-xl border border-slate-200 bg-slate-50 px-5 py-4 text-sm">
                  <div>
                    <div className="text-slate-500">{t('createdAt')}</div>
                    <div className="mt-1 font-semibold text-slate-950">
                      {new Date(batch.created_at).toLocaleString()}
                    </div>
                  </div>
                  <div>
                    <div className="text-slate-500">{t('questionCount')}</div>
                    <div className="mt-1 font-semibold text-slate-950">{batch.case_count}</div>
                  </div>
                  <div>
                    <div className="text-slate-500">{t('testScope')}</div>
                    <div className="mt-1 font-semibold text-slate-950">
                      {t('selectedFromLibrary')}
                    </div>
                  </div>
                </div>
              </div>
              <Button
                variant="outline"
                className="text-blue-600 hover:text-blue-700"
                disabled={
                  batch.status === 'queued' || batch.status === 'running' || retestBatch.isPending
                }
                onClick={() => setRetestConfirmOpen(true)}
              >
                <RefreshCcw className="mr-2 size-4" />
                {commonT('retest')}
              </Button>
            </div>

            <div className="mt-5 grid grid-cols-2 rounded-xl border border-slate-200 bg-white px-5 py-6 text-sm text-slate-700">
              <div className="pr-8">
                <div className="text-slate-500">{t('businessResult')}</div>
                <div className="mt-3 flex flex-wrap gap-2">
                  <span className="inline-flex items-center rounded-full bg-emerald-50 px-3 py-1 text-sm font-medium text-emerald-700">
                    {t('passed', { count: batch.passed_count })}
                  </span>
                  <span className="inline-flex items-center rounded-full bg-red-50 px-3 py-1 text-sm font-medium text-red-700">
                    {t('failed', { count: batch.failed_count })}
                  </span>
                  <span className="inline-flex items-center rounded-full bg-amber-50 px-3 py-1 text-sm font-medium text-amber-700">
                    {t('review', { count: batch.review_count })}
                  </span>
                </div>
              </div>
              <div className="border-l border-slate-200 pl-8">
                <div className="text-slate-500">{t('executionStatus')}</div>
                <div className="mt-3">
                  <span
                    className={cn(
                      'inline-flex items-center gap-2 rounded-full px-3 py-1 text-sm font-medium',
                      executionErrorCount > 0
                        ? 'bg-red-50 text-red-700'
                        : 'bg-emerald-50 text-emerald-700'
                    )}
                  >
                    {executionErrorCount === 0 ? <CheckCircle2 className="size-4" /> : null}
                    {executionErrorCount > 0
                      ? t('executionStatusWithErrors', {
                          done: finishedCount,
                          total: batch.case_count,
                          count: executionErrorCount,
                        })
                      : t('executionStatusNormal', {
                          done: finishedCount,
                          total: batch.case_count,
                        })}
                  </span>
                </div>
                {executionErrorCount > 0 ? (
                  <div className="mt-2 text-xs text-red-600">
                    {t('executionErrorsDescription')}
                  </div>
                ) : null}
              </div>
            </div>

            <div className="mt-5 rounded-xl border border-blue-100 bg-blue-50 p-4 text-sm text-slate-800">
              <div className="flex items-start gap-3">
                <span className="rounded-full bg-white p-2 text-blue-600">
                  <WandSparkles className="size-4" />
                </span>
                <p>
                  <span className="font-semibold">{t('aiSummary')}</span>
                  {buildSummary(batch, items, summaryT)}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
        <ConfirmDialog
          open={retestConfirmOpen}
          onOpenChange={open => {
            if (!open && !retestBatch.isPending) setRetestConfirmOpen(false);
          }}
          title={t('retestConfirmTitle')}
          description={t('retestConfirmDescription', {
            name: batch.name,
            count: batch.case_count,
          })}
          confirmText={t('retestConfirmButton')}
          cancelText={commonT('cancel')}
          loading={retestBatch.isPending}
          contentClassName="max-w-2xl rounded-2xl"
          footerClassName="justify-end bg-white px-8 py-6"
          cancelClassName="border border-slate-200 bg-white hover:bg-slate-50"
          confirmClassName="bg-slate-950 text-white hover:bg-slate-800"
          onConfirm={() =>
            retestBatch.mutate(
              {
                batchId: batch.id,
                data: { name: buildRetestName(batch.name) },
              },
              { onSuccess: () => setRetestConfirmOpen(false) }
            )
          }
        />

        <Card className="rounded-2xl">
          <CardHeader>
            <CardTitle>{t('itemListTitle')}</CardTitle>
            <p className="text-sm text-slate-600">{t('itemListDescription')}</p>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[45%]">{t('table.questionContent')}</TableHead>
                  <TableHead>{t('table.scenario')}</TableHead>
                  <TableHead>{t('table.testResult')}</TableHead>
                  <TableHead>{t('table.responseTime')}</TableHead>
                  <TableHead className="text-right">{t('table.actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="h-28 text-center text-slate-500">
                      {t('emptyItems')}
                    </TableCell>
                  </TableRow>
                ) : (
                  items.map(item => (
                    <TableRow key={item.id}>
                      <TableCell className="max-w-md align-top">
                        <div className="line-clamp-2 font-medium text-slate-950">
                          {item.case_snapshot.content}
                        </div>
                        <div className="mt-2 flex flex-wrap gap-2 text-xs text-slate-500">
                          {(item.case_snapshot.turns?.length ?? 0) > 1 ? (
                            <span>{t('turnCount', { count: item.case_snapshot.turns.length })}</span>
                          ) : null}
                          {hasAttachments(item) ? <span>{commonT('attachmentsIncluded')}</span> : null}
                        </div>
                      </TableCell>
                      <TableCell className="align-top text-sm text-slate-700">
                        {getScenarioName(item)}
                      </TableCell>
                      <TableCell className="align-top">
                        <Badge className={itemStatusClass(item.status)}>
                          {itemStatusLabel(item.status, itemStatusT, commonT('none'))}
                        </Badge>
                      </TableCell>
                      <TableCell className="align-top text-sm text-slate-700">
                        {formatResponseTime(item, commonT('none'))}
                      </TableCell>
                      <TableCell className="align-top text-right">
                        <Button variant="ghost" size="sm" asChild>
                          <Link href={`/console/agents/${agentId}/batch-test/${batchId}/items/${item.id}`}>
                            {t('viewDetail')}
                          </Link>
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
