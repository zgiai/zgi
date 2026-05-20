'use client';

import * as React from 'react';
import { ChevronDown, ChevronUp, Clock3, RotateCw } from 'lucide-react';
import { useT } from '@/i18n';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Pagination } from '@/components/ui/pagination';
import { Skeleton } from '@/components/ui/skeleton';
import { actionTypeRegistry } from './registry';
import {
  formatTaskDateTime,
  formatRunDuration,
  getMostRelevantRunTimestamp,
  getRunStatusBadgeVariant,
  safeJson,
} from './utils';
import type { AutomationTaskRunItem } from '@/services/types/automation';

interface TaskRunsTimelineProps {
  runs: AutomationTaskRunItem[];
  total: number;
  page: number;
  pageSize: number;
  isLoading: boolean;
  timezone?: string;
  onPageChange: (page: number) => void;
}

function getRecordString(value: Record<string, unknown> | null | undefined, key: string): string {
  const rawValue = value?.[key];
  return typeof rawValue === 'string' ? rawValue : '';
}

/**
 * @component TaskRunsTimeline
 * @category Feature
 * @status Stable
 * @description Timeline-style run history with expandable action-run details.
 * @usage Render inside the task detail panel when the user switches to the runs tab.
 */
export function TaskRunsTimeline({
  runs,
  total,
  page,
  pageSize,
  isLoading,
  timezone,
  onPageChange,
}: TaskRunsTimelineProps) {
  const t = useT('automation');
  const [openRuns, setOpenRuns] = React.useState<Record<string, boolean>>({});
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  const toggleOpen = React.useCallback((runId: string) => {
    setOpenRuns(current => ({
      ...current,
      [runId]: !current[runId],
    }));
  }, []);

  if (isLoading) {
    return (
      <div className="space-y-4">
        {Array.from({ length: 3 }).map((_, index) => (
          <Card key={`run-skeleton-${index}`} className="border-border/70">
            <CardContent className="space-y-3 p-5">
              <Skeleton className="h-4 w-40" />
              <Skeleton className="h-4 w-72" />
              <Skeleton className="h-24 rounded-xl" />
            </CardContent>
          </Card>
        ))}
      </div>
    );
  }

  if (runs.length === 0) {
    return (
      <Card className="border-dashed border-border/80 bg-background/90">
        <CardContent className="flex flex-col items-center gap-3 py-14 text-center">
          <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary/10 text-primary">
            <RotateCw className="size-5" />
          </div>
          <div className="space-y-2">
            <h3 className="text-lg font-semibold text-foreground">
              {t('runsTimeline.emptyTitle')}
            </h3>
            <p className="text-sm leading-6 text-muted-foreground">
              {t('runsTimeline.emptyDescription')}
            </p>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      {runs.map(item => {
        const runTime = getMostRelevantRunTimestamp(item.run);
        const triggerTime =
          item.run.trigger_source === 'manual_run'
            ? (item.run.started_at ?? item.run.created_at ?? item.run.scheduled_for)
            : item.run.scheduled_for;
        const isOpen = Boolean(openRuns[item.run.id]);

        return (
          <Collapsible key={item.run.id} open={isOpen} onOpenChange={() => toggleOpen(item.run.id)}>
            <Card className="border-border/70">
              <CardContent className="p-0">
                <div className="border-b border-border/70 p-5">
                  <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                    <div className="space-y-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant={getRunStatusBadgeVariant(item.run.status)}>
                          {t(`status.${item.run.status}`)}
                        </Badge>
                        <Badge variant="outline">
                          {t(`triggerSource.${item.run.trigger_source}`)}
                        </Badge>
                      </div>
                      <p className="text-base font-semibold text-foreground">
                        {runTime ? formatTaskDateTime(runTime, timezone) : t('misc.notAvailable')}
                      </p>
                      <div className="grid gap-2 text-sm text-muted-foreground md:grid-cols-2">
                        <span>
                          {t(
                            item.run.trigger_source === 'manual_run'
                              ? 'runsTimeline.triggeredAt'
                              : 'runsTimeline.scheduledFor'
                          )}
                          :{' '}
                          {triggerTime
                            ? formatTaskDateTime(triggerTime, timezone)
                            : t('misc.notAvailable')}
                        </span>
                        <span>
                          {t('runsTimeline.startedAt')}:{' '}
                          {item.run.started_at
                            ? formatTaskDateTime(item.run.started_at, timezone)
                            : t('misc.notAvailable')}
                        </span>
                        <span>
                          {t('runsTimeline.finishedAt')}:{' '}
                          {item.run.finished_at
                            ? formatTaskDateTime(item.run.finished_at, timezone)
                            : t('misc.notAvailable')}
                        </span>
                        <span>
                          {t('runsTimeline.duration')}: {formatRunDuration(item.run)}
                        </span>
                      </div>
                    </div>

                    <CollapsibleTrigger asChild>
                      <Button variant="outline" size="sm">
                        <Clock3 className="size-4" />
                        {isOpen ? (
                          <ChevronUp className="size-4" />
                        ) : (
                          <ChevronDown className="size-4" />
                        )}
                      </Button>
                    </CollapsibleTrigger>
                  </div>
                </div>

                <CollapsibleContent>
                  <div className="space-y-4 p-5">
                    {item.run.error_summary ? (
                      <Alert variant="destructive">
                        <AlertTitle>{t('runsTimeline.errorSummary')}</AlertTitle>
                        <AlertDescription>{item.run.error_summary}</AlertDescription>
                      </Alert>
                    ) : null}

                    {item.run.runtime_context ? (
                      <div className="space-y-2">
                        <h4 className="text-sm font-medium text-foreground">
                          {t('runsTimeline.runtimeContext')}
                        </h4>
                        <pre className="overflow-x-auto rounded-xl border border-border/70 bg-muted/30 p-4 text-xs leading-6 text-muted-foreground">
                          {safeJson(item.run.runtime_context)}
                        </pre>
                      </div>
                    ) : null}

                    <div className="space-y-3">
                      {item.action_runs.map(actionRun => {
                        const actionMeta = actionTypeRegistry[actionRun.action_type];
                        const workflowRunId = getRecordString(
                          actionRun.response_payload,
                          'workflow_run_id'
                        );
                        const targetAgentId = getRecordString(
                          actionRun.response_payload,
                          'agent_id'
                        );

                        return (
                          <div
                            key={actionRun.id}
                            className="rounded-2xl border border-border/70 bg-muted/15 p-4"
                          >
                            <div className="mb-3 flex flex-wrap items-center gap-2">
                              <Badge variant={getRunStatusBadgeVariant(actionRun.status)}>
                                {t(`status.${actionRun.status}`)}
                              </Badge>
                              <span className="text-sm font-medium text-foreground">
                                {actionMeta
                                  ? t(actionMeta.labelKey as never)
                                  : t('runsTimeline.actionResult')}
                              </span>
                            </div>

                            {actionRun.action_type === 'run_workflow' && workflowRunId ? (
                              <div className="mb-3 grid gap-2 rounded-xl border border-border/70 bg-background/80 p-3 text-xs leading-5 text-muted-foreground md:grid-cols-2">
                                <span className="min-w-0 break-all">
                                  {t('actions.workflowRunId')}: {workflowRunId}
                                </span>
                                {targetAgentId ? (
                                  <span className="min-w-0 break-all">
                                    {t('actions.targetAgent')}: {targetAgentId}
                                  </span>
                                ) : null}
                              </div>
                            ) : null}

                            {actionRun.error_message ? (
                              <Alert variant="destructive" className="mb-3">
                                <AlertTitle>{t('runsTimeline.errorMessage')}</AlertTitle>
                                <AlertDescription>{actionRun.error_message}</AlertDescription>
                              </Alert>
                            ) : null}

                            <div className="grid gap-3 xl:grid-cols-2">
                              {actionRun.request_payload ? (
                                <div className="space-y-2">
                                  <h5 className="text-sm font-medium text-foreground">
                                    {t('runsTimeline.requestPayload')}
                                  </h5>
                                  <pre className="overflow-x-auto rounded-xl border border-border/70 bg-background/80 p-4 text-xs leading-6 text-muted-foreground">
                                    {safeJson(actionRun.request_payload)}
                                  </pre>
                                </div>
                              ) : null}

                              {actionRun.response_payload ? (
                                <div className="space-y-2">
                                  <h5 className="text-sm font-medium text-foreground">
                                    {t('runsTimeline.responsePayload')}
                                  </h5>
                                  <pre className="overflow-x-auto rounded-xl border border-border/70 bg-background/80 p-4 text-xs leading-6 text-muted-foreground">
                                    {safeJson(actionRun.response_payload)}
                                  </pre>
                                </div>
                              ) : null}
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                </CollapsibleContent>
              </CardContent>
            </Card>
          </Collapsible>
        );
      })}

      {totalPages > 1 ? (
        <Pagination
          currentPage={page}
          totalPages={totalPages}
          total={total}
          pageSize={pageSize}
          onPageChange={onPageChange}
          renderInfo={(start, end, totalItems) =>
            t('runsTimeline.paginationInfo', {
              start,
              end,
              total: totalItems,
            })
          }
        />
      ) : null}
    </div>
  );
}
