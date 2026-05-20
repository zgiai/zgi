'use client';

import * as React from 'react';
import {
  AlertCircle,
  Archive,
  Clock3,
  MoreHorizontal,
  Pause,
  PencilLine,
  RotateCw,
  X,
} from 'lucide-react';
import { useT } from '@/i18n';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useAutomationTaskRuns } from '@/hooks/automation/use-automation';
import type { AutomationTaskStatus } from '@/services/types/automation';
import { TaskOverviewTab } from './task-overview-tab';
import { TaskRunsTimeline } from './task-runs-timeline';
import {
  formatTaskDateTime,
  getScheduleSummary,
  getTaskNextRunLabel,
  getTaskStatusBadgeVariant,
  shouldShowArchivedNotice,
} from './utils';
import type { TaskDetailViewData, TaskPanelTab } from './types';

interface TaskDetailPanelProps {
  taskDetail?: TaskDetailViewData;
  workspaceId: string;
  tab: TaskPanelTab;
  selectedStatuses: AutomationTaskStatus[];
  isLoading: boolean;
  canManage: boolean;
  actionBusy: boolean;
  onClose: () => void;
  onEdit: () => void;
  onRunTask: () => void;
  onPauseTask: () => void;
  onResumeTask: () => void;
  onArchiveTask: () => void;
  onTabChange: (tab: TaskPanelTab) => void;
  onRefreshTask: () => void;
}

function TaskPanelSkeleton() {
  return (
    <div className="space-y-4 p-5">
      <Skeleton className="h-6 w-40" />
      <Skeleton className="h-20 rounded-2xl" />
      <Skeleton className="h-40 rounded-2xl" />
      <Skeleton className="h-60 rounded-2xl" />
    </div>
  );
}

/**
 * @component TaskDetailPanel
 * @category Feature
 * @status Stable
 * @description Detail panel for a scheduled task with summary cards, actions, and lazy run history loading.
 * @usage Render inside the task workbench when a task is selected.
 */
export function TaskDetailPanel({
  taskDetail,
  workspaceId,
  tab,
  selectedStatuses,
  isLoading,
  canManage,
  actionBusy,
  onClose,
  onEdit,
  onRunTask,
  onPauseTask,
  onResumeTask,
  onArchiveTask,
  onTabChange,
  onRefreshTask,
}: TaskDetailPanelProps) {
  const t = useT('automation');
  const tCommon = useT('common');
  const [runsPage, setRunsPage] = React.useState(1);
  const translate = React.useCallback(
    (key: string, values?: Record<string, string | number>) => t(key as never, values as never),
    [t]
  );

  React.useEffect(() => {
    setRunsPage(1);
  }, [taskDetail?.task.id]);

  const runs = useAutomationTaskRuns(
    taskDetail?.task.id,
    {
      workspace_id: workspaceId,
      page: runsPage,
      limit: 10,
    },
    tab === 'runs' && Boolean(taskDetail?.task.id)
  );
  const lastSyncedRunRef = React.useRef('');

  React.useEffect(() => {
    if (tab !== 'runs') {
      return;
    }

    const latestRun = runs.runs[0]?.run;
    if (!latestRun || runs.isFetching) {
      return;
    }

    if (latestRun.status === 'queued' || latestRun.status === 'running') {
      return;
    }

    const fingerprint = `${latestRun.id}:${latestRun.status}:${latestRun.finished_at ?? ''}`;
    if (lastSyncedRunRef.current !== fingerprint) {
      lastSyncedRunRef.current = fingerprint;
      onRefreshTask();
    }
  }, [onRefreshTask, runs.isFetching, runs.runs, tab]);

  if (isLoading) {
    return <TaskPanelSkeleton />;
  }

  if (!taskDetail) {
    return (
      <div className="flex h-full min-h-0 flex-col">
        <div className="flex items-center justify-between border-b border-border/70 px-5 py-4">
          <h2 className="text-base font-semibold text-foreground">{t('detail.details')}</h2>
          <Button variant="ghost" size="sm" isIcon onClick={onClose}>
            <X className="size-4" />
          </Button>
        </div>
        <div className="p-5">
          <Alert variant="destructive">
            <AlertCircle className="size-4" />
            <AlertTitle>{t('detail.details')}</AlertTitle>
            <AlertDescription>{tCommon('errorBoundary.somethingWentWrong')}</AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  const { task } = taskDetail;
  const isArchived = task.status === 'archived';
  const canEdit = canManage && !isArchived;
  const canRun = canManage && !isArchived;
  const canPause = canManage && task.status === 'active';
  const canResume = canManage && task.status === 'paused';
  const canArchive = canManage && !isArchived;
  const showArchivedNotice = shouldShowArchivedNotice(task, selectedStatuses);
  const schedule = getScheduleSummary(task, translate);
  const showUnsupportedHint =
    (task.schedule_type !== 'once' && task.schedule_type !== 'cron') ||
    taskDetail.actions.some(action => {
      if (action.action_type === 'send_notification') {
        return action.config.channel_type !== 'email' && action.config.channel_type !== 'sms';
      }

      return action.action_type !== 'run_workflow';
    });

  return (
    <div className="flex h-full min-h-0 w-full flex-col">
      <div className="border-b border-border/70 px-5 py-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 flex-1 space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="min-w-0 break-words text-lg font-semibold leading-7 text-foreground">
                {task.name}
              </h2>
              <Badge variant={getTaskStatusBadgeVariant(task.status)}>
                {t(`status.${task.status}`)}
              </Badge>
              {!canManage || isArchived ? (
                <Badge variant="outline">{t('operations.readOnly')}</Badge>
              ) : null}
            </div>
            <p className="break-words text-sm leading-6 text-muted-foreground">
              {task.description || t('detail.noDescription')}
            </p>
          </div>

          <Button variant="ghost" size="sm" isIcon onClick={onClose}>
            <X className="size-4" />
          </Button>
        </div>

        <div className="mt-4 flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={onEdit}
            disabled={!canEdit || actionBusy}
            title={!canEdit && !canManage ? t('noManagePermission') : undefined}
          >
            <PencilLine className="size-4" />
            {t('operations.edit')}
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={onRunTask}
            disabled={!canRun || actionBusy}
            title={!canRun && !canManage ? t('noManagePermission') : undefined}
          >
            <RotateCw className="size-4" />
            {t('operations.runNow')}
          </Button>
          {canPause ? (
            <Button size="sm" variant="outline" onClick={onPauseTask} disabled={actionBusy}>
              <Pause className="size-4" />
              {t('operations.pause')}
            </Button>
          ) : null}
          {canResume ? (
            <Button size="sm" variant="outline" onClick={onResumeTask} disabled={actionBusy}>
              <RotateCw className="size-4" />
              {t('operations.resume')}
            </Button>
          ) : null}
          {canArchive ? (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button size="sm" variant="outline" disabled={actionBusy} aria-label={t('more')}>
                  <MoreHorizontal className="size-4" />
                  {t('more')}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-40">
                <DropdownMenuItem variant="destructive" onSelect={onArchiveTask}>
                  <Archive className="size-4" />
                  {t('operations.archive')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          ) : null}
        </div>
      </div>

      <ScrollArea className="h-0 grow">
        <div className="space-y-4 p-5">
          {!canManage ? (
            <Alert>
              <AlertTitle>{t('operations.readOnly')}</AlertTitle>
              <AlertDescription>{t('noManagePermission')}</AlertDescription>
            </Alert>
          ) : null}

          {showArchivedNotice ? (
            <Alert>
              <AlertTitle>{t('status.archived')}</AlertTitle>
              <AlertDescription>{t('detail.archivedNotice')}</AlertDescription>
            </Alert>
          ) : null}

          <div className="grid gap-3">
            <Card className="border-border/70" padding="none">
              <CardContent className="p-4">
                <div className="flex items-start gap-3">
                  <div className="flex size-10 shrink-0 items-center justify-center rounded-2xl bg-primary/10 text-primary">
                    <Clock3 className="size-4" />
                  </div>
                  <div className="min-w-0 space-y-1">
                    <p className="text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                      {t('schedule.title')}
                    </p>
                    <p className="text-base font-semibold text-foreground">{schedule.title}</p>
                    <p className="break-words text-sm leading-6 text-muted-foreground">
                      {schedule.description}
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>

            <div className="grid gap-3 md:grid-cols-2">
              <Card className="border-border/70" padding="none">
                <CardContent className="space-y-2 p-4">
                  <p className="text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                    {t('detail.nextRun')}
                  </p>
                  <p className="break-words text-sm font-medium leading-6 text-foreground">
                    {getTaskNextRunLabel(task, key => t(key as never))}
                  </p>
                </CardContent>
              </Card>

              <Card className="border-border/70" padding="none">
                <CardContent className="space-y-2 p-4">
                  <p className="text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                    {t('detail.lastRun')}
                  </p>
                  <p className="break-words text-sm font-medium leading-6 text-foreground">
                    {task.last_run_at ? formatTaskDateTime(task.last_run_at) : t('misc.notRunYet')}
                  </p>
                </CardContent>
              </Card>

              <Card className="border-border/70" padding="none">
                <CardContent className="space-y-2 p-4">
                  <p className="text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                    {t('detail.currentStatus')}
                  </p>
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant={getTaskStatusBadgeVariant(task.status)}>
                      {t(`status.${task.status}`)}
                    </Badge>
                  </div>
                </CardContent>
              </Card>

              <Card className="border-border/70" padding="none">
                <CardContent className="space-y-2 p-4">
                  <p className="text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                    {t('detail.createdAt')}
                  </p>
                  <p className="break-words text-sm font-medium leading-6 text-foreground">
                    {task.created_at ? formatTaskDateTime(task.created_at) : t('misc.notAvailable')}
                  </p>
                </CardContent>
              </Card>
            </div>
          </div>

          <Tabs value={tab} onValueChange={value => onTabChange(value as TaskPanelTab)}>
            <TabsList>
              <TabsTrigger value="overview">{t('overview')}</TabsTrigger>
              <TabsTrigger value="runs">{t('runs')}</TabsTrigger>
            </TabsList>
            <TabsContent value="overview">
              <TaskOverviewTab taskDetail={taskDetail} showUnsupportedHint={showUnsupportedHint} />
            </TabsContent>
            <TabsContent value="runs">
              <TaskRunsTimeline
                runs={runs.runs}
                total={runs.total}
                page={runs.page}
                pageSize={runs.limit}
                isLoading={runs.isLoading}
                timezone={task.timezone}
                onPageChange={setRunsPage}
              />
            </TabsContent>
          </Tabs>
        </div>
      </ScrollArea>
    </div>
  );
}
