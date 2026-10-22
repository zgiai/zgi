'use client';

import { Fragment, useEffect, useMemo, useState, type ReactNode } from 'react';
import Link from 'next/link';
import {
  Activity,
  AlertCircle,
  ArrowRight,
  CalendarClock,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CircleDashed,
  Clock3,
  CircleOff,
  EyeOff,
  MessageSquare,
  Package,
  RadioTower,
  RefreshCw,
  RotateCcw,
  Loader2,
  XCircle,
  Building2,
  KeyRound,
  MessagesSquare,
  UserRound,
  Users,
  type LucideIcon,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectSeparator, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useAuthStore } from '@/store/auth-store';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { getAgentTextIconDisplay } from '@/components/agents/agent-runtime/utils';
import { Skeleton } from '@/components/ui/skeleton';
import { digitalHumanChannelProvider } from '@/components/digital-human/digital-human-channel-providers';
import { digitalHumanTaskDisplayStatus, formatTaskTime } from '@/components/digital-human/digital-human-task-presentation';
import {
  digitalHumanRunStatusLabelKey,
  digitalHumanRunTaskId,
  isRunningDigitalHumanRunStatus,
  isWaitingDigitalHumanRunStatus,
  digitalHumanRunTaskName,
  plainTextPreview,
} from '@/components/digital-human/digital-human-run-presentation';
import {
  useDigitalHumanChannelMutations,
  useDigitalHumanChannels,
} from '@/hooks/digital-human/use-digital-human-channels';
import {
  useDigitalHumanMyTaskMutations,
  useDigitalHumanMyTasks,
  useDigitalHumanTaskMutations,
  useDigitalHumanTaskReports,
  useDigitalHumanTasks,
} from '@/hooks/digital-human/use-digital-human-tasks';
import { digitalHumanObjectHref } from '@/components/digital-human/digital-human-object-link';
import { toast } from 'sonner';
import { useDigitalHuman, useDigitalHumanRuns } from '@/hooks/digital-human/use-digital-humans';
import { useLocale } from '@/hooks/use-locale';
import { useMediaQuery } from '@/hooks/use-media-query';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { digitalHumanService } from '@/services/digital-human.service';
import { useQuery } from '@tanstack/react-query';
import type {
  DigitalHumanChannelDelivery,
  DigitalHumanChannelInstallation,
  DigitalHumanRun,
  DigitalHumanTask,
  DigitalHumanTaskStatus,
} from '@/services/types/digital-human';

const attentionPageSize = 3;
const taskStatusOrder: Record<DigitalHumanTaskStatus, number> = {
  proposed: 0,
  blocked: 1,
  in_progress: 2,
  waiting: 3,
  pending: 4,
  completed: 5,
  canceled: 6,
};

export function DigitalHumanDashboard({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const detail = useDigitalHuman(id);
  const permissions = useAccountPermissions();
  const accountId = useAuthStore.use.user()?.id;
  // Whose work the workspace shows, so many API users or channel chats do not
  // bury the rest. The server filters, so counts and pages stay true.
  const [scope, setScope] = useState<DashboardScope>(() => readDashboardScope(id));
  const requester = dashboardRequesterFilter(scope, accountId);
  const tasks = useDigitalHumanTasks(id, {
    ...currentLocalDayRunWindow(),
    ...(requester.kind ? { requester_kind: [requester.kind] } : {}),
    ...(requester.id ? { requester_id: requester.id } : {}),
  });
  // The reader's own tasks: private tasks are not in the task center list for
  // members, but their proposals, failures, and reports still need attention.
  const myTasks = useDigitalHumanMyTasks(
    id,
    {},
    permissions.hasPermission('digital_human.run') && (scope === 'all' || scope === 'mine' || scope === 'member')
  );
  const taskMutations = useDigitalHumanTaskMutations(id);
  const myTaskMutations = useDigitalHumanMyTaskMutations(id);
  const [pendingTaskAction, setPendingTaskAction] = useState<string | null>(null);
  const runs = useDigitalHumanRuns(
    id,
    true,
    requester.kind || requester.id
      ? { ...(requester.kind ? { 'requester_kind[]': [requester.kind] } : {}), ...(requester.id ? { requester_id: requester.id } : {}) }
      : undefined
  );
  const channels = useDigitalHumanChannels(id, true);
  const channelMutations = useDigitalHumanChannelMutations(id);
  const canManageDeliveries = permissions.hasPermission('digital_human.run');
  const [refreshState, setRefreshState] = useState<'idle' | 'refreshing' | 'success' | 'error'>(
    'idle'
  );
  const [attentionPage, setAttentionPage] = useState(1);
  const [ignoredAttentionKeys, setIgnoredAttentionKeys] = useState<Set<string>>(() =>
    readIgnoredAttentionKeys(id)
  );

  useEffect(() => {
    if (!['success', 'error'].includes(refreshState)) return;
    const timer = window.setTimeout(() => setRefreshState('idle'), 2_500);
    return () => window.clearTimeout(timer);
  }, [refreshState]);

  useEffect(() => {
    setIgnoredAttentionKeys(readIgnoredAttentionKeys(id));
    setAttentionPage(1);
    setScope(readDashboardScope(id));
  }, [id]);

  function changeScope(next: DashboardScope) {
    setScope(next);
    setAttentionPage(1);
    writeDashboardScope(id, next);
  }

  function ignoreAttention(key: string) {
    setIgnoredAttentionKeys(current => {
      const next = new Set(current);
      next.add(key);
      writeIgnoredAttentionKeys(id, next);
      return next;
    });
  }

  function restoreIgnoredAttention() {
    const next = new Set<string>();
    writeIgnoredAttentionKeys(id, next);
    setIgnoredAttentionKeys(next);
    setAttentionPage(1);
  }

  function dismissAttentionDelivery(deliveryId?: string) {
    if (!deliveryId) return;
    channelMutations.dismissDelivery.mutate(deliveryId);
  }

  async function refreshDashboard() {
    if (refreshState === 'refreshing') return;
    setRefreshState('refreshing');
    const results = await Promise.allSettled([
      detail.refetch(),
      tasks.refetch(),
      myTasks.refetch(),
      runs.refetch(),
      channels.installations.refetch(),
      channels.events.refetch(),
      channels.deliveries.refetch(),
    ]);
    setRefreshState(
      results.some(result => result.status === 'rejected' || result.value.isError)
        ? 'error'
        : 'success'
    );
  }

  if (detail.isLoading) return <DashboardSkeleton />;

  if (detail.isError || !detail.data?.data) {
    return (
      <main className="mx-auto flex min-h-full w-full max-w-[1600px] items-center justify-center px-4 py-10 md:px-8">
        <div className="max-w-md rounded-xl border bg-background p-8 text-center">
          <AlertCircle className="mx-auto size-6 text-destructive" />
          <h1 className="mt-3 text-lg font-semibold">{t('dashboard.loadError')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('dashboard.loadErrorDescription')}
          </p>
          <Button className="mt-5" variant="outline" onClick={() => void detail.refetch()}>
            <RefreshCw className="size-4" />
            {t('common.refresh')}
          </Button>
        </div>
      </main>
    );
  }

  const definition = detail.data.data.definition;
  const textIcon = getAgentTextIconDisplay(definition.icon_type, definition.icon, definition.name);
  const published = detail.data.data.published;
  const taskItems = tasks.data?.data.items ?? [];
  const taskCenterIds = new Set(taskItems.map(task => task.id));
  const ownTaskItems =
    scope === 'all' || scope === 'mine' || scope === 'member'
      ? (myTasks.data?.data.items ?? []).filter(task => !taskCenterIds.has(task.id))
      : [];
  const attentionCandidates = [...taskItems, ...ownTaskItems];
  // Task center tasks open over the workspace; a member's own private task
  // lives in their conversation panel.
  const taskHref = (task: DigitalHumanTask) =>
    taskCenterIds.has(task.id)
      ? digitalHumanObjectHref(`/console/digital-humans/${id}`, 'task', task.id)
      : `/console/digital-humans/${id}/chat?task=${task.id}`;
  const runHref = (runId: string) => digitalHumanObjectHref(`/console/digital-humans/${id}`, 'run', runId);
  // Confirm, decline, and retry are simple decisions, so they are made in the
  // row; the task drawer is for reading more first.
  const applyTaskAction = async (task: DigitalHumanTask, action: 'confirm' | 'decline' | 'retry') => {
    if (pendingTaskAction) return;
    setPendingTaskAction(task.id);
    try {
      const mutation = taskCenterIds.has(task.id) ? taskMutations.action : myTaskMutations.action;
      await mutation.mutateAsync({ taskId: task.id, action, revision: task.revision });
      toast.success(t('tasks.actionApplied'));
      await Promise.all([tasks.refetch(), myTasks.refetch()]);
    } catch {
      // The mutation reports its own error.
    } finally {
      setPendingTaskAction(null);
    }
  };
  const runItems = runs.data?.data.items ?? [];
  const installations = channels.installations.data?.data ?? [];
  const deliveries = channels.deliveries.data?.data.items ?? [];
  // Runs waiting on an approval or an answer are not working: they are shown
  // as waiting for you and never as running.
  const runningRun = runItems.find(run => isRunningDigitalHumanRunStatus(run.status));
  const waitingRun = runItems.find(run => isWaitingDigitalHumanRunStatus(run.status));
  const currentTask = currentDashboardTask(taskItems, runItems);
  // Channel deliveries belong to channel conversations.
  const failedDeliveries =
    scope === 'all' || scope === 'channel'
      ? deliveries.filter(delivery => ['failed', 'unknown'].includes(delivery.status))
      : [];
  const attentionTasks = attentionCandidates
    .filter(task => ['proposed', 'blocked'].includes(task.work_status))
    .sort((left, right) => taskStatusOrder[left.work_status] - taskStatusOrder[right.work_status]);
  const attentionTaskIds = new Set(attentionTasks.map(task => task.id));
  const failedTaskRuns = attentionCandidates.filter(
    task =>
      task.lifecycle_status !== 'archived' &&
      normalizeRunStatus(task.last_run_status ?? '') === 'failed' &&
      !attentionTaskIds.has(task.id)
  );
  const failedTaskRunIds = new Set(failedTaskRuns.map(task => task.id));
  const reportsToReview = attentionCandidates.filter(
    task => task.latest_report?.review_state === 'new' && !attentionTaskIds.has(task.id) && !failedTaskRunIds.has(task.id)
  );
  const allAttentionItems = buildDashboardAttentionItems({
    id,
    attentionTasks,
    failedTaskRuns,
    reportsToReview,
    failedDeliveries,
    taskHref,
    t,
  });
  const canDecide = (task: DigitalHumanTask) =>
    task.can_change !== false &&
    (taskCenterIds.has(task.id) ? permissions.hasPermission('digital_human.update') : permissions.hasPermission('digital_human.run'));
  const attentionItems = allAttentionItems.filter(item => !ignoredAttentionKeys.has(item.key));
  const ignoredAttentionCount = allAttentionItems.length - attentionItems.length;
  const ignoredAttentionRecordCount = ignoredAttentionKeys.size;
  const attentionCount = attentionItems.length;
  const attentionTotalPages = Math.max(1, Math.ceil(attentionCount / attentionPageSize));
  const currentAttentionPage = Math.min(attentionPage, attentionTotalPages);
  const pagedAttentionItems = attentionItems.slice(
    (currentAttentionPage - 1) * attentionPageSize,
    currentAttentionPage * attentionPageSize
  );
  const todayCompleted = tasks.data?.data.successful_run_count ?? 0;
  const dashboardStatus = resolveDashboardStatus({
    definitionStatus: definition.status,
    published: Boolean(published),
    attentionCount,
  });
  // The next scheduled work, soonest first.
  const upcomingTasks = [...taskItems]
    .filter(task => task.lifecycle_status === 'active' && Boolean(task.next_run_at))
    .sort((left, right) => taskNextTimestamp(left) - taskNextTimestamp(right))
    .slice(0, 5);
  const recentRuns = [...runItems]
    .sort(
      (left, right) => new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime()
    )
    .slice(0, 5);
  const lastUpdatedAt = Math.max(
    detail.dataUpdatedAt,
    tasks.dataUpdatedAt,
    runs.dataUpdatedAt,
    channels.installations.dataUpdatedAt,
    channels.events.dataUpdatedAt,
    channels.deliveries.dataUpdatedAt
  );
  const isWorking = Boolean(currentTask || runningRun);
  const focusRun = runningRun ?? (currentTask ? undefined : waitingRun);

  return (
    <main className="@container/dashboard mx-auto flex min-h-full w-full max-w-[1600px] flex-col gap-4 px-4 py-4 sm:gap-5 md:px-6 md:py-6 2xl:gap-6 2xl:px-8 2xl:py-8">
      <header className="flex flex-col gap-3 @[680px]/dashboard:flex-row @[680px]/dashboard:items-center @[680px]/dashboard:justify-between">
        <div className="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1">
          <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">
            {t('dashboard.title')}
          </h1>
          <p className="text-xs text-muted-foreground sm:text-sm">
            {formatFullDate(new Date(), locale)}
          </p>
        </div>
        <div className="flex items-center justify-between gap-3 @[680px]/dashboard:justify-end">
          <Select value={scope} onValueChange={value => changeScope(value as DashboardScope)}>
            {/* A narrowed workspace is tinted, so it is not mistaken for everything. */}
            <SelectTrigger
              className={cn(
                // The shared trigger lets its value grow from zero, so it needs a width.
                'h-8 w-[9.5rem] shrink-0 gap-1.5 px-2.5 text-xs font-medium',
                scope !== 'all' && 'border-primary/40 bg-primary/5 text-primary hover:border-primary/60 dark:bg-primary/10'
              )}
              aria-label={t('dashboard.scope.label')}
              title={t('dashboard.scope.label')}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent align="end" className="min-w-[11rem]">
              {dashboardScopeGroups.map((group, index) => (
                <Fragment key={group.join()}>
                  {index > 0 ? <SelectSeparator /> : null}
                  <SelectGroup>
                    {index === 0 ? (
                      <SelectLabel className="px-2 pb-1 pt-1 text-xs font-medium text-muted-foreground">
                        {t('dashboard.scope.label')}
                      </SelectLabel>
                    ) : null}
                    {group.map(value => {
                      const Icon = dashboardScopeIcons[value];
                      return (
                        <SelectItem key={value} value={value} disabled={value === 'mine' && !accountId}>
                          <span className="flex items-center gap-2">
                            <Icon className="size-3.5 shrink-0 text-muted-foreground" />
                            {t(`dashboard.scope.${value}`)}
                          </span>
                        </SelectItem>
                      );
                    })}
                  </SelectGroup>
                </Fragment>
              ))}
            </SelectContent>
          </Select>
          <p className="min-w-0 truncate text-xs text-muted-foreground" aria-live="polite">
            {refreshState === 'success'
              ? t('dashboard.refreshed')
              : refreshState === 'error'
                ? t('dashboard.refreshFailed')
                : t('dashboard.updatedAt', {
                    value: formatClock(new Date(lastUpdatedAt || Date.now()), locale),
                  })}
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="shrink-0"
            disabled={refreshState === 'refreshing'}
            aria-label={t('dashboard.refresh')}
            onClick={() => void refreshDashboard()}
          >
            <RefreshCw className={cn('size-4', refreshState === 'refreshing' && 'animate-spin')} />
            {refreshState === 'refreshing' ? t('dashboard.refreshing') : t('dashboard.refresh')}
          </Button>
        </div>
      </header>

      <DashboardLiveFeed id={id} runs={runItems} tasks={taskItems} locale={locale} />

      <section
        className="@container/dashboard-hero grid overflow-hidden rounded-xl border bg-background @[680px]/dashboard-hero:grid-cols-2 @[1120px]/dashboard-hero:grid-cols-[minmax(300px,1.2fr)_minmax(220px,0.8fr)_minmax(220px,0.8fr)]"
        aria-busy={isWorking}
      >
        <div className="flex min-w-0 items-center gap-3 p-4 sm:gap-4 sm:p-5 @[680px]/dashboard-hero:col-span-2 @[1120px]/dashboard-hero:col-span-1 @[1120px]/dashboard-hero:p-6">
          <IconPreview
            iconType={definition.icon_type || 'text'}
            icon={textIcon.text}
            iconBackground={textIcon.background}
            src={
              definition.icon_type === 'image' ? definition.icon_url || definition.avatar || '' : ''
            }
            alt={definition.name}
            size="md"
            editable={false}
            showUpload={false}
            showRemove={false}
            className="shrink-0"
          />
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate text-lg font-semibold">{definition.name}</h2>
              <DashboardStatusBadge status={dashboardStatus} working={isWorking} />
            </div>
            <p className="mt-1 line-clamp-2 text-sm font-medium">
              {currentTask
                ? t('dashboard.statusLine.working', { name: currentTask.name })
                : runningRun
                  ? t('dashboard.statusLine.responding')
                  : waitingRun
                    ? t('dashboard.statusLine.waitingApproval')
                    : attentionCount > 0
                    ? t('dashboard.statusLine.waitingOnYou', { count: attentionCount })
                    : t('dashboard.statusLine.idle')}
            </p>
            <p className="mt-1 truncate text-xs text-muted-foreground">
              {published?.version_number
                ? t('dashboard.version', { value: published.version_number })
                : t('dashboard.notPublished')}
            </p>
          </div>
          <Button asChild variant="outline" size="sm" className="hidden shrink-0 sm:inline-flex">
            {focusRun ? (
              <Link href={runHref(focusRun.id)} scroll={false}>
                <Activity className={cn('size-4', isWorking && 'animate-pulse')} />
                {t('dashboard.viewRun')}
              </Link>
            ) : (
              <Link href={`/console/digital-humans/${id}/chat`}>
                <MessageSquare className="size-4" />
                {t('dashboard.startRun')}
              </Link>
            )}
          </Button>
        </div>

        {/* The hero says what is running now; scheduled work is listed under
            "Up next" beside the attention list. */}
        {isWorking || focusRun ? (
        <FocusBlock
          label={isWorking ? t('dashboard.currentFocus') : t('dashboard.waitingFocus')}
          title={
            currentTask?.name ??
            (focusRun ? (digitalHumanRunTaskName(focusRun) ?? focusRun.query) : t('dashboard.noCurrentFocus'))
          }
          meta={
            currentTask
              ? t(`tasks.status.${digitalHumanTaskDisplayStatus(currentTask)}`)
              : focusRun
                ? t(digitalHumanRunStatusLabelKey(focusRun.status, focusRun.waiting_reason))
                : undefined
          }
          active={isWorking}
          className="@[680px]/dashboard-hero:border-l-0 @[1120px]/dashboard-hero:border-l"
        />
        ) : null}
        <div className="flex items-center border-t p-4 sm:hidden">
          {focusRun ? (
            <Button asChild variant="outline" size="sm" className="w-full">
              <Link href={runHref(focusRun.id)} scroll={false}>
                <Activity className={cn('size-4', isWorking && 'animate-pulse')} />
                {t('dashboard.viewRun')}
              </Link>
            </Button>
          ) : (
            <Button asChild variant="outline" size="sm" className="w-full">
              <Link href={`/console/digital-humans/${id}/chat`}>
                <MessageSquare className="size-4" />
                {t('dashboard.startRun')}
              </Link>
            </Button>
          )}
        </div>
      </section>

      <DashboardIndicators
        requester={requester}
        id={id}
        attentionCount={attentionCount}
        completedToday={todayCompleted}
      />

      {/* Four equal columns, shared with the indicators above: the main
          column takes three, the side column one. */}
      <div className="grid gap-5 @[1280px]/dashboard:grid-cols-4 2xl:gap-6">
        <div className="flex min-w-0 flex-col gap-5 @[1280px]/dashboard:col-span-3 2xl:gap-6">
          <section id="dashboard-attention" className="scroll-mt-4 overflow-hidden rounded-xl border bg-background">
            <div className="flex min-h-[61px] flex-wrap items-center justify-between gap-2 border-b px-5 py-3.5">
              <div className="flex items-center gap-2">
                <h2 className="font-semibold">{t('dashboard.waitingOnYou')}</h2>
                {attentionCount > 0 ? <Badge variant="warning">{attentionCount}</Badge> : null}
              </div>
              {ignoredAttentionRecordCount > 0 ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="xs"
                  className="-mr-2 text-muted-foreground"
                  onClick={restoreIgnoredAttention}
                >
                  <RotateCcw className="size-3.5" />
                  {t('dashboard.clearIgnored', { count: ignoredAttentionRecordCount })}
                </Button>
              ) : null}
            </div>
            {attentionCount === 0 ? (
              <InlineState
                compact
                icon={<CheckCircle2 className="size-5 text-success" />}
                title={
                  ignoredAttentionCount > 0
                    ? t('dashboard.allAttentionIgnored')
                    : t('dashboard.noAttention')
                }
                description={
                  ignoredAttentionCount > 0
                    ? t('dashboard.allAttentionIgnoredDescription')
                    : t('dashboard.noAttentionDescription')
                }
              />
            ) : (
              <>
                <div className="divide-y">
                  {pagedAttentionItems.map(item => (
                    <AttentionRow
                      key={item.key}
                      title={item.title}
                      label={item.label}
                      description={item.description}
                      href={item.href}
                      tone={item.tone}
                      actions={
                        item.task && canDecide(item.task) ? (
                          <AttentionTaskActions
                            task={item.task}
                            pending={pendingTaskAction === item.task.id}
                            canRetry={permissions.hasPermission('digital_human.run')}
                            onAction={action => item.task && void applyTaskAction(item.task, action)}
                          />
                        ) : null
                      }
                      onIgnore={() => ignoreAttention(item.key)}
                      onDismiss={
                        item.deliveryId && canManageDeliveries
                          ? () => dismissAttentionDelivery(item.deliveryId)
                          : undefined
                      }
                      dismissing={
                        channelMutations.dismissDelivery.isPending &&
                        channelMutations.dismissDelivery.variables === item.deliveryId
                      }
                    />
                  ))}
                </div>
                {attentionTotalPages > 1 ? (
                  <AttentionPagination
                    page={currentAttentionPage}
                    totalPages={attentionTotalPages}
                    total={attentionCount}
                    onPageChange={setAttentionPage}
                  />
                ) : null}
              </>
            )}
          </section>
        <section className="overflow-hidden rounded-xl border bg-background">
          <div className="flex items-center justify-between border-b px-5 py-4">
            <h2 className="font-semibold">{t('dashboard.recentActivity')}</h2>
            <Button asChild variant="ghost" size="sm" className="-mr-2">
              <Link href={`/console/digital-humans/${id}/runs`}>
                {t('dashboard.viewAllActivity')}
                <ArrowRight className="size-4" />
              </Link>
            </Button>
          </div>
          {runs.isLoading ? (
            <div className="space-y-3 p-5">
              {[0, 1, 2].map(item => (
                <Skeleton key={item} className="h-8 rounded-md" />
              ))}
            </div>
          ) : recentRuns.length === 0 ? (
            <InlineState
              compact
              icon={<Activity className="size-5 text-muted-foreground" />}
              title={t('dashboard.noActivity')}
              description={t('dashboard.noActivityDescription')}
            />
          ) : (
            <div className="divide-y px-5">
              {recentRuns.map(run => (
                <RecentRunRow key={run.id} id={id} run={run} locale={locale} />
              ))}
            </div>
          )}
        </section>
        </div>

        <aside className="flex min-w-0 flex-col gap-5 @[1280px]/dashboard:col-span-1 2xl:gap-6">
          <UpcomingTasks id={id} tasks={upcomingTasks} locale={locale} />
          <RecentArtifacts id={id} locale={locale} />
          <section className="overflow-hidden rounded-xl border bg-background">
            <div className="flex items-center justify-between border-b px-5 py-4">
              <h2 className="font-semibold">{t('dashboard.connectionsTitle')}</h2>
              <Button asChild variant="ghost" size="sm" className="-mr-2">
                <Link href={`/console/digital-humans/${id}/channels`}>
                  {t('dashboard.manageConnections')}
                </Link>
              </Button>
            </div>
            {channels.installations.isLoading ? (
              <div className="space-y-3 p-5">
                {[0, 1].map(item => (
                  <Skeleton key={item} className="h-10 rounded-lg" />
                ))}
              </div>
            ) : installations.length === 0 ? (
              <InlineState
                compact
                icon={<RadioTower className="size-5 text-muted-foreground" />}
                title={t('dashboard.noConnections')}
                description={t('dashboard.noConnectionsDescription')}
              />
            ) : (
              <div className="divide-y px-5">
                {installations.slice(0, 6).map(installation => (
                  <ConnectionRow key={installation.id} installation={installation} />
                ))}
              </div>
            )}
          </section>
        </aside>
      </div>

    </main>
  );
}

type DashboardStatus = 'healthy' | 'attention' | 'unpublished' | 'disabled';

function resolveDashboardStatus({
  definitionStatus,
  published,
  attentionCount,
}: {
  definitionStatus: string;
  published: boolean;
  attentionCount: number;
}): DashboardStatus {
  if (definitionStatus === 'disabled') return 'disabled';
  if (!published) return 'unpublished';
  if (attentionCount > 0) return 'attention';
  return 'healthy';
}

function DashboardStatusBadge({ status, working }: { status: DashboardStatus; working: boolean }) {
  const t = useT('digitalHumans');
  return (
    <Badge
      variant={
        status === 'healthy'
          ? 'success'
          : status === 'attention' || status === 'unpublished'
            ? 'warning'
            : 'subtle'
      }
    >
      {working ? (
        <CircleDashed className="size-3 animate-spin motion-reduce:animate-none" />
      ) : status === 'healthy' ? (
        <CheckCircle2 className="size-3" />
      ) : (
        <AlertCircle className="size-3" />
      )}
      {t(`dashboard.status.${status}`)}
    </Badge>
  );
}

function FocusBlock({
  label,
  title,
  meta,
  active = false,
  className,
}: {
  label: string;
  title: string;
  meta?: string;
  active?: boolean;
  className?: string;
}) {
  return (
    <div
      className={cn(
        'min-w-0 border-t p-4 sm:p-5 @[680px]/dashboard-hero:border-l @[1120px]/dashboard-hero:border-t-0 @[1120px]/dashboard-hero:p-6',
        className
      )}
    >
      <p className="flex items-center gap-2 text-xs text-muted-foreground">
        {active ? (
          <span className="relative flex size-2" aria-hidden="true">
            <span className="absolute inline-flex size-full animate-ping rounded-full bg-primary opacity-50 motion-reduce:hidden" />
            <span className="relative inline-flex size-2 rounded-full bg-primary" />
          </span>
        ) : null}
        {label}
      </p>
      <p className="mt-2 line-clamp-2 text-sm font-semibold">{title}</p>
      {meta ? <p className="mt-1 truncate text-xs text-muted-foreground">{meta}</p> : null}
    </div>
  );
}

type DashboardLiveEventTone = 'active' | 'success' | 'warning' | 'danger' | 'neutral';

interface DashboardLiveEvent {
  id: string;
  title: string;
  description: string;
  timestamp: string;
  href: string;
  tone: DashboardLiveEventTone;
}

function DashboardLiveFeed({
  id,
  runs,
  tasks,
  locale,
}: {
  id: string;
  runs: DigitalHumanRun[];
  tasks: DigitalHumanTask[];
  locale: string;
}) {
  const t = useT('digitalHumans');
  const reduceMotion = useMediaQuery('(prefers-reduced-motion: reduce)');
  const events = useMemo(
    () => buildDashboardLiveEvents({ id, runs, tasks, t }),
    [id, runs, t, tasks]
  );
  const [activeIndex, setActiveIndex] = useState(0);
  const newestEventId = events[0]?.id;

  useEffect(() => setActiveIndex(0), [newestEventId]);
  useEffect(() => {
    if (reduceMotion || events.length < 2) return;
    const timer = window.setInterval(
      () => setActiveIndex(current => (current + 1) % events.length),
      4_500
    );
    return () => window.clearInterval(timer);
  }, [events.length, reduceMotion]);

  const event = events[activeIndex] ?? events[0];

  if (!event) return null;

  return (
    <section
      className="flex min-h-12 min-w-0 items-center gap-3 overflow-hidden rounded-xl border bg-muted/15 px-3 py-2.5 sm:px-4"
      aria-label={t('dashboard.live.title')}
    >
      <div className="flex shrink-0 items-center gap-2 border-r pr-3 text-xs font-medium sm:pr-4">
        <Activity className="size-4 animate-pulse text-primary motion-reduce:animate-none" />
        <span className="hidden sm:inline">{t('dashboard.live.title')}</span>
      </div>

      <Link
        key={`${event.id}-${activeIndex}`}
        href={event.href}
        scroll={false}
        className="group flex min-w-0 flex-1 items-center gap-2 rounded-sm outline-none animate-in fade-in slide-in-from-bottom-1 duration-300 motion-reduce:animate-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <span
          className={cn('size-1.5 shrink-0 rounded-full', liveEventToneClassName(event.tone))}
        />
        <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
          {formatClock(new Date(event.timestamp), locale)}
        </span>
        <span className="shrink-0 text-xs font-medium">{event.title}</span>
        <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground group-hover:text-foreground">
          {event.description}
        </span>
        <ArrowRight className="hidden size-3.5 shrink-0 text-muted-foreground sm:block" />
      </Link>

      {events.length > 1 ? (
        <div className="hidden shrink-0 items-center gap-1 sm:flex" aria-hidden="true">
          {events.map((item, index) => (
            <span
              key={item.id}
              className={cn(
                'h-1 rounded-full transition-[width,background-color] duration-300 motion-reduce:transition-none',
                index === activeIndex ? 'w-3 bg-primary' : 'w-1 bg-border'
              )}
            />
          ))}
        </div>
      ) : null}
    </section>
  );
}

function buildDashboardLiveEvents({
  id,
  runs,
  tasks,
  t,
}: {
  id: string;
  runs: DigitalHumanRun[];
  tasks: DigitalHumanTask[];
  t: ReturnType<typeof useT<'digitalHumans'>>;
}) {
  const events: DashboardLiveEvent[] = [];
  const currentTask = currentDashboardTask(tasks, runs);
  if (currentTask) {
    events.push({
      id: `task-${currentTask.id}-${currentTask.updated_at}`,
      title: t('dashboard.live.taskRunning'),
      description: currentTask.name,
      timestamp: currentTask.updated_at,
      href: digitalHumanObjectHref(`/console/digital-humans/${id}`, 'task', currentTask.id),
      tone: 'active',
    });
  }

  for (const run of newestFirst(runs.filter(item => isWaitingDigitalHumanRunStatus(item.status))).slice(0, 2)) {
    events.push({
      id: `run-${run.id}-${run.status}-${run.updated_at}`,
      title: t(digitalHumanRunStatusLabelKey(run.status, run.waiting_reason)),
      description: digitalHumanRunTaskName(run) ?? dashboardRunSummary(run, t('dashboard.automaticRun')),
      timestamp: run.updated_at,
      href: digitalHumanObjectHref(`/console/digital-humans/${id}`, 'run', run.id),
      tone: 'warning',
    });
  }

  for (const run of newestFirst(
    runs.filter(item => isRunningDigitalHumanRunStatus(item.status))
  ).slice(0, 4)) {
    events.push({
      id: `run-${run.id}-${run.status}-${run.updated_at}`,
      title: t('dashboard.live.runActive'),
      description: currentTask
        ? currentTask.name
        : dashboardRunSummary(run, t('dashboard.automaticRun')),
      timestamp: run.updated_at,
      href: digitalHumanObjectHref(`/console/digital-humans/${id}`, 'run', run.id),
      tone: 'active',
    });
  }

  return events
    .filter(event => Number.isFinite(Date.parse(event.timestamp)))
    .sort((left, right) => Date.parse(right.timestamp) - Date.parse(left.timestamp))
    .slice(0, 6);
}

function newestFirst<T extends { updated_at: string }>(items: T[]) {
  return [...items].sort(
    (left, right) => Date.parse(right.updated_at) - Date.parse(left.updated_at)
  );
}

function dashboardRunSummary(run: DigitalHumanRun, scheduledFallback: string) {
  const origin = isRecord(run.invocation_event?.origin) ? run.invocation_event.origin : null;
  const source =
    (typeof origin?.kind === 'string' && origin.kind) ||
    (typeof run.invocation_event?.type === 'string' && run.invocation_event.type) ||
    'manual';
  const task = digitalHumanRunTaskName(run);
  if (task) return task;
  if (source === 'schedule' || source === 'scheduled') return scheduledFallback;
  return run.query || plainTextPreview(run.result_summary) || run.error_message || scheduledFallback;
}

function liveEventToneClassName(tone: DashboardLiveEventTone) {
  if (tone === 'active') return 'bg-primary';
  if (tone === 'success') return 'bg-success';
  if (tone === 'warning') return 'bg-warning';
  if (tone === 'danger') return 'bg-destructive';
  return 'bg-muted-foreground';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

interface DashboardAttentionItem {
  key: string;
  title: string;
  // What kind of thing waits: a proposal, a blocked or failed task, a report.
  label?: string;
  description: string;
  href: string;
  tone: 'warning' | 'danger';
  occurredAt: string;
  deliveryId?: string;
  // A proposed or blocked task, decided in its row.
  task?: DigitalHumanTask;
}

function buildDashboardAttentionItems({
  id,
  attentionTasks,
  failedTaskRuns,
  reportsToReview,
  failedDeliveries,
  taskHref,
  t,
}: {
  id: string;
  attentionTasks: DigitalHumanTask[];
  failedTaskRuns: DigitalHumanTask[];
  reportsToReview: DigitalHumanTask[];
  failedDeliveries: DigitalHumanChannelDelivery[];
  taskHref: (task: DigitalHumanTask) => string;
  t: ReturnType<typeof useT<'digitalHumans'>>;
}) {
  const items: DashboardAttentionItem[] = [
    ...attentionTasks.map(task => ({
      key: `task-${task.id}-${task.work_status}-${task.revision}`,
      title: task.name,
      label: task.work_status === 'proposed' ? t('dashboard.kind.proposal') : t('dashboard.kind.blocked'),
      // A blocked task says what blocked it when its last report knows.
      description:
        task.work_status === 'proposed'
          ? t('dashboard.proposalNeedsDecision')
          : plainTextPreview(task.latest_report?.summary) || t('dashboard.blockedNeedsHelp'),
      href: taskHref(task),
      tone: task.work_status === 'blocked' ? ('danger' as const) : ('warning' as const),
      occurredAt: task.updated_at,
      task,
    })),
    ...failedTaskRuns.map(task => ({
      key: `failed-run-${task.id}-${task.last_run_at ?? task.updated_at}`,
      title: task.name,
      label: t('dashboard.kind.failed'),
      description: plainTextPreview(task.latest_report?.summary) || t('dashboard.taskRunFailedNeedsReview'),
      href: taskHref(task),
      tone: 'danger' as const,
      occurredAt: task.last_run_at ?? task.updated_at,
    })),
    ...reportsToReview.map(task => ({
      key: `report-${task.latest_report?.id}`,
      title: task.name,
      label: t('dashboard.kind.report'),
      description: plainTextPreview(task.latest_report?.summary) || t('dashboard.reportToReview'),
      // A report opens in the report drawer over the workspace.
      href: `/console/digital-humans/${id}?report=${task.latest_report?.id ?? ''}`,
      tone: 'warning' as const,
      occurredAt: task.latest_report?.created_at ?? task.updated_at,
    })),
    ...failedDeliveries.map(delivery => ({
      key: `failed-delivery-${delivery.id}-${delivery.status}-${delivery.updated_at}`,
      title: t('dashboard.deliveryFailed'),
      label: t('dashboard.kind.delivery'),
      description: delivery.error_message || t('dashboard.deliveryFailedDescription'),
      href: `/console/digital-humans/${id}/channels`,
      tone: 'danger' as const,
      occurredAt: delivery.updated_at,
      deliveryId: delivery.id,
    })),
  ];

  return items.sort((left, right) => {
    if (left.tone !== right.tone) return left.tone === 'danger' ? -1 : 1;
    return Date.parse(right.occurredAt) - Date.parse(left.occurredAt);
  });
}

function AttentionRow({
  title,
  label,
  description,
  href,
  tone,
  actions,
  onIgnore,
  onDismiss,
  dismissing = false,
}: {
  title: string;
  label?: string;
  description: string;
  href: string;
  tone: 'warning' | 'danger';
  actions?: ReactNode;
  onIgnore: () => void;
  onDismiss?: () => void;
  dismissing?: boolean;
}) {
  const t = useT('digitalHumans');
  return (
    <article className="group flex flex-col gap-2 px-5 py-3.5 transition-colors hover:bg-muted/20 @[680px]/dashboard:flex-row @[680px]/dashboard:items-center @[680px]/dashboard:gap-4">
      <Link
        href={href}
        scroll={false}
        className="flex min-w-0 flex-1 gap-3 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <AlertCircle
          className={cn(
            'mt-0.5 size-4 shrink-0',
            tone === 'danger' ? 'text-destructive' : 'text-warning'
          )}
        />
        <div className="min-w-0 flex-1">
          <p className="flex min-w-0 items-center gap-2">
            <span className="line-clamp-1 text-sm font-medium">{title}</span>
            {label ? (
              <Badge variant={tone === 'danger' ? 'destructive' : 'warning'} className="shrink-0 text-[11px]">
                {label}
              </Badge>
            ) : null}
          </p>
          <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">{description}</p>
        </div>
      </Link>
      {/* Actions sit beside the item they act on. */}
      <div className="flex shrink-0 items-center justify-end gap-1 pl-7 @[680px]/dashboard:pl-0">
        {actions}
        <Button asChild variant="ghost" size="xs">
          <Link href={href} scroll={false}>
            {actions ? t('dashboard.view') : t('dashboard.handle')}
            <ArrowRight className="size-3.5" />
          </Link>
        </Button>
        {onDismiss ? (
          <ConfirmDialog
            trigger={
              <Button type="button" variant="ghost" size="xs" disabled={dismissing}>
                <CircleOff className="size-3.5" />
                {t('channels.deliveries.dismiss')}
              </Button>
            }
            title={t('channels.deliveries.dismissDialog.title')}
            description={t('channels.deliveries.dismissDialog.description')}
            confirmText={t('channels.deliveries.dismissDialog.confirm')}
            cancelText={t('common.cancel')}
            variant="warning"
            loading={dismissing}
            onConfirm={onDismiss}
          />
        ) : null}
        <Button
          type="button"
          variant="ghost"
          size="xs"
          className="text-muted-foreground"
          onClick={onIgnore}
        >
          <EyeOff className="size-3.5" />
          {t('dashboard.ignore')}
        </Button>
      </div>
    </article>
  );
}

function AttentionPagination({
  page,
  totalPages,
  total,
  onPageChange,
}: {
  page: number;
  totalPages: number;
  total: number;
  onPageChange: (page: number) => void;
}) {
  const t = useT('digitalHumans');
  return (
    <div className="flex items-center justify-between gap-3 border-t bg-muted/10 px-4 py-2.5">
      <span className="text-xs tabular-nums text-muted-foreground">
        {t('dashboard.attentionPagination', { page, totalPages, total })}
      </span>
      <div className="flex items-center gap-1">
        <Button
          type="button"
          variant="ghost"
          size="xs"
          isIcon
          aria-label={t('dashboard.previousAttentionPage')}
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          <ChevronLeft className="size-4" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="xs"
          isIcon
          aria-label={t('dashboard.nextAttentionPage')}
          disabled={page >= totalPages}
          onClick={() => onPageChange(page + 1)}
        >
          <ChevronRight className="size-4" />
        </Button>
      </div>
    </div>
  );
}

function ignoredAttentionStorageKey(id: string) {
  return `zgi:digital-human:${id}:dashboard-attention-ignored`;
}

function readIgnoredAttentionKeys(id: string) {
  if (typeof window === 'undefined') return new Set<string>();
  try {
    const value: unknown = JSON.parse(
      window.localStorage.getItem(ignoredAttentionStorageKey(id)) ?? '[]'
    );
    if (!Array.isArray(value)) return new Set<string>();
    return new Set(value.filter((item): item is string => typeof item === 'string'));
  } catch {
    return new Set<string>();
  }
}

function writeIgnoredAttentionKeys(id: string, keys: Set<string>) {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(
      ignoredAttentionStorageKey(id),
      JSON.stringify([...keys].slice(-200))
    );
  } catch {
    // The dashboard remains usable when browser storage is unavailable.
  }
}

function ConnectionRow({ installation }: { installation: DigitalHumanChannelInstallation }) {
  const t = useT('digitalHumans');
  const provider = digitalHumanChannelProvider(installation.provider);
  const connected =
    installation.status === 'active' &&
    (!installation.receiver ||
      ['connected', 'not_applicable'].includes(installation.receiver.state));
  return (
    <div className="flex items-center gap-3 py-3">
      <span
        className={cn(
          'flex size-8 shrink-0 items-center justify-center rounded-lg',
          provider?.accentClassName ?? 'bg-muted text-muted-foreground'
        )}
      >
        <RadioTower className="size-4" />
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{installation.name}</p>
        <p className="truncate text-xs text-muted-foreground">
          {provider ? t(provider.nameKey) : installation.provider}
        </p>
      </div>
      <span className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
        <span className={cn('size-1.5 rounded-full', connected ? 'bg-success' : 'bg-warning')} />
        {connected ? t('dashboard.connectionHealthy') : t('dashboard.connectionAttention')}
      </span>
    </div>
  );
}

function RecentRunRow({ id, run, locale }: { id: string; run: DigitalHumanRun; locale: string }) {
  const t = useT('digitalHumans');
  const active = isRunningDigitalHumanRunStatus(run.status);
  const waiting = isWaitingDigitalHumanRunStatus(run.status);
  const statusLabel = t(digitalHumanRunStatusLabelKey(run.status, run.waiting_reason));
  const Icon = run.status === 'succeeded' ? CheckCircle2 : active ? CircleDashed : waiting ? Clock3 : AlertCircle;
  return (
    <Link
      href={digitalHumanObjectHref(`/console/digital-humans/${id}`, 'run', run.id)}
      scroll={false}
      className="grid gap-2 py-3 text-sm outline-none transition-colors hover:text-primary focus-visible:ring-2 focus-visible:ring-ring sm:grid-cols-[70px_20px_minmax(0,1fr)_auto] sm:items-center"
    >
      <time className="text-xs tabular-nums text-muted-foreground">
        {formatClock(new Date(run.updated_at), locale)}
      </time>
      <Icon
        className={cn(
          'size-4',
          run.status === 'succeeded' ? 'text-success' : active ? 'text-primary' : waiting ? 'text-warning' : 'text-destructive'
        )}
      />
      <div className="min-w-0">
        <p className="truncate font-medium text-foreground">
          {digitalHumanRunTaskName(run)
            ? t('runs.taskRun', { name: digitalHumanRunTaskName(run) ?? '' })
            : run.query || t('dashboard.automaticRun')}
        </p>
        <p className="mt-0.5 truncate text-xs text-muted-foreground">
          {plainTextPreview(run.result_summary) || run.error_message || statusLabel}
        </p>
      </div>
      <span className="hidden text-xs text-muted-foreground sm:block">{statusLabel}</span>
    </Link>
  );
}

function InlineState({
  icon,
  title,
  description,
  action,
  compact = false,
}: {
  icon: ReactNode;
  title: string;
  description: string;
  action?: ReactNode;
  compact?: boolean;
}) {
  return (
    <div className={cn('flex flex-col items-center px-6 text-center', compact ? 'py-7' : 'py-10')}>
      {icon}
      <p className="mt-2 text-sm font-medium">{title}</p>
      <p className="mt-1 max-w-sm text-xs leading-5 text-muted-foreground">{description}</p>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

function DashboardSkeleton() {
  return (
    <main className="mx-auto flex min-h-full w-full max-w-[1600px] flex-col gap-5 px-4 py-5 md:px-8 md:py-7">
      <div className="space-y-2">
        <Skeleton className="h-8 w-40" />
        <Skeleton className="h-4 w-28" />
      </div>
      <Skeleton className="h-32 rounded-xl" />
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_340px]">
        <Skeleton className="h-[440px] rounded-xl" />
        <div className="space-y-5">
          <Skeleton className="h-56 rounded-xl" />
          <Skeleton className="h-48 rounded-xl" />
        </div>
      </div>
      <Skeleton className="h-52 rounded-xl" />
    </main>
  );
}

function taskNextTimestamp(task: DigitalHumanTask) {
  const value = task.next_run_at ?? task.schedule.run_at;
  if (!value) return Number.MAX_SAFE_INTEGER;
  const timestamp = new Date(value).getTime();
  return Number.isFinite(timestamp) ? timestamp : Number.MAX_SAFE_INTEGER;
}

function normalizeRunStatus(
  status: string
): 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'unknown' {
  if (['pending', 'processing', 'running'].includes(status)) {
    return 'running';
  }
  if (['succeeded', 'success', 'completed'].includes(status)) {
    return 'succeeded';
  }
  if (status === 'failed') {
    return 'failed';
  }
  if (status === 'canceled') {
    return 'canceled';
  }
  return 'queued';
}

function currentLocalDayRunWindow() {
  const now = new Date();
  const start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const end = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
  return {
    run_finished_after: start.toISOString(),
    run_finished_before: end.toISOString(),
  };
}

function formatFullDate(value: Date, locale: string) {
  return new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    weekday: 'long',
  }).format(value);
}

function formatClock(value: Date, locale: string) {
  return new Intl.DateTimeFormat(locale, { hour: '2-digit', minute: '2-digit' }).format(value);
}

/** Counts that lead to the work: reports to read, items to handle, today's results, artifacts. */
function DashboardIndicators({
  id,
  attentionCount,
  completedToday,
  requester,
}: {
  id: string;
  attentionCount: number;
  completedToday: number;
  requester: DashboardRequesterFilter;
}) {
  const t = useT('digitalHumans');
  const reports = useDigitalHumanTaskReports(id, {
    'review_state[]': ['new'],
    page_size: 1,
    ...(requester.kind ? { 'requester_kind[]': [requester.kind] } : {}),
    ...(requester.id ? { requester_id: requester.id } : {}),
  });
  const artifacts = useQuery({
    queryKey: ['digital-human-artifacts', id, 'count'],
    queryFn: () => digitalHumanService.listArtifacts(id, { page_size: 1 }),
  });
  const tiles = [
    { label: t('dashboard.indicators.reports'), value: reports.data?.data.total ?? 0, href: `/console/digital-humans/${id}/reports`, tone: 'primary' },
    { label: t('dashboard.indicators.attention'), value: attentionCount, href: '#dashboard-attention', tone: attentionCount > 0 ? 'warning' : 'muted' },
    { label: t('dashboard.indicators.completed'), value: completedToday, href: `/console/digital-humans/${id}/tasks?status=completed`, tone: 'muted' },
    { label: t('dashboard.indicators.artifacts'), value: artifacts.data?.data.total ?? 0, href: `/console/digital-humans/${id}/artifacts`, tone: 'muted' },
  ] as const;
  const renderTile = (tile: (typeof tiles)[number]) => (
        <Link
          key={tile.label}
          href={tile.href}
          className="rounded-xl border bg-background px-4 py-3 outline-none transition-colors hover:border-primary/30 hover:bg-primary/[0.02] focus-visible:ring-2 focus-visible:ring-ring"
        >
          <p className="text-xs text-muted-foreground">{tile.label}</p>
          <p
            className={cn(
              'mt-1 text-2xl font-semibold tabular-nums',
              tile.tone === 'warning' && 'text-warning',
              tile.tone === 'primary' && tile.value > 0 && 'text-primary'
            )}
          >
            {tile.value}
          </p>
        </Link>
  );
  // Four equal tiles over the four columns below, with the same gaps.
  return (
    <nav
      aria-label={t('dashboard.indicators.label')}
      className="grid grid-cols-2 gap-3 @[680px]/dashboard:grid-cols-4 @[1280px]/dashboard:gap-5 2xl:gap-6"
    >
      {tiles.map(renderTile)}
    </nav>
  );
}

function UpcomingTasks({ id, tasks, locale }: { id: string; tasks: DigitalHumanTask[]; locale: string }) {
  const t = useT('digitalHumans');
  return (
    <section className="overflow-hidden rounded-xl border bg-background">
      <div className="flex items-center justify-between border-b px-5 py-4">
        <h2 className="font-semibold">{t('dashboard.upcoming')}</h2>
        <Button asChild variant="ghost" size="sm" className="-mr-2">
          <Link href={`/console/digital-humans/${id}/tasks`}>
            {t('dashboard.viewAllTasks')}
            <ArrowRight className="size-4" />
          </Link>
        </Button>
      </div>
      {tasks.length === 0 ? (
        <InlineState compact icon={<CalendarClock className="size-5 text-muted-foreground" />} title={t('dashboard.noUpcoming')} description="" />
      ) : (
        <ul className="divide-y px-5">
          {tasks.map(task => (
            <li key={task.id}>
              <Link href={digitalHumanObjectHref(`/console/digital-humans/${id}`, 'task', task.id)} scroll={false} className="flex items-center gap-3 py-2.5 text-sm hover:text-primary">
                <span className="w-24 shrink-0 text-xs tabular-nums text-muted-foreground">{formatTaskTime(task.next_run_at, locale)}</span>
                <span className="truncate">{task.name}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function RecentArtifacts({ id, locale }: { id: string; locale: string }) {
  const t = useT('digitalHumans');
  const query = useQuery({
    queryKey: ['digital-human-artifacts', id, 'recent'],
    queryFn: () => digitalHumanService.listArtifacts(id, { page_size: 3 }),
  });
  const items = query.data?.data.items ?? [];
  if (!query.isLoading && items.length === 0) return null;
  return (
    <section className="overflow-hidden rounded-xl border bg-background">
      <div className="flex items-center justify-between border-b px-5 py-4">
        <h2 className="font-semibold">{t('dashboard.recentArtifacts')}</h2>
        <Button asChild variant="ghost" size="sm" className="-mr-2">
          <Link href={`/console/digital-humans/${id}/artifacts`}>
            {t('dashboard.viewAllArtifacts')}
            <ArrowRight className="size-4" />
          </Link>
        </Button>
      </div>
      <ul className="divide-y px-5">
        {items.map(item => (
          <li key={item.id}>
            <Link href={digitalHumanObjectHref(`/console/digital-humans/${id}`, 'artifact', item.id)} scroll={false} className="flex items-center gap-3 py-2.5 text-sm hover:text-primary">
              <Package className="size-4 shrink-0 text-muted-foreground" />
              <span className="min-w-0 flex-1 truncate">{item.title}</span>
              <span className="shrink-0 text-xs text-muted-foreground">
                v{item.current_version} · {formatTaskTime(item.updated_at, locale)}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * The task being worked on now. A task whose run is parked on an approval or
 * an answer stays in progress on the server, but it is waiting for someone
 * and does not count as current work.
 */
function currentDashboardTask(tasks: DigitalHumanTask[], runs: DigitalHumanRun[]) {
  const waitingTaskIds = new Set(
    runs
      .filter(run => isWaitingDigitalHumanRunStatus(run.status))
      .map(run => digitalHumanRunTaskId(run))
      .filter((taskId): taskId is string => Boolean(taskId))
  );
  return tasks.find(task => task.work_status === 'in_progress' && !waitingTaskIds.has(task.id));
}

/** Confirm or decline a proposal, or retry a blocked task, without leaving the row. */
function AttentionTaskActions({
  task,
  pending,
  canRetry,
  onAction,
}: {
  task: DigitalHumanTask;
  pending: boolean;
  canRetry: boolean;
  onAction: (action: 'confirm' | 'decline' | 'retry') => void;
}) {
  const t = useT('digitalHumans');
  if (task.work_status === 'proposed') {
    return (
      <>
        <ConfirmDialog
          trigger={
            <Button type="button" variant="ghost" size="xs" disabled={pending}>
              <XCircle className="size-3.5" />
              {t('tasks.actions.decline')}
            </Button>
          }
          title={t('tasks.confirm.declineTitle')}
          description={t('tasks.confirm.declineDescription')}
          confirmText={t('tasks.actions.decline')}
          cancelText={t('common.cancel')}
          variant="danger"
          onConfirm={() => onAction('decline')}
        />
        <Button type="button" variant="outline" size="xs" disabled={pending} onClick={() => onAction('confirm')}>
          {pending ? <Loader2 className="size-3.5 animate-spin" /> : <CheckCircle2 className="size-3.5" />}
          {t('tasks.actions.confirm')}
        </Button>
      </>
    );
  }
  if (task.work_status === 'blocked' && canRetry) {
    return (
      <Button type="button" variant="outline" size="xs" disabled={pending} onClick={() => onAction('retry')}>
        {pending ? <Loader2 className="size-3.5 animate-spin" /> : <RotateCcw className="size-3.5" />}
        {t('tasks.actions.retry')}
      </Button>
    );
  }
  return null;
}

type DashboardScope = 'all' | 'mine' | 'member' | 'api_user' | 'channel';
const dashboardScopes: readonly DashboardScope[] = ['all', 'mine', 'member', 'api_user', 'channel'];
// Whose work, then which kind of requester.
const dashboardScopeGroups: ReadonlyArray<readonly DashboardScope[]> = [['all', 'mine'], ['member', 'api_user', 'channel']];
const dashboardScopeIcons: Record<DashboardScope, LucideIcon> = {
  all: Users,
  mine: UserRound,
  member: Building2,
  api_user: KeyRound,
  channel: MessagesSquare,
};

interface DashboardRequesterFilter {
  kind?: 'member' | 'api_user' | 'channel';
  id?: string;
}

function dashboardRequesterFilter(scope: DashboardScope, accountId?: string): DashboardRequesterFilter {
  if (scope === 'mine') return accountId ? { id: accountId } : {};
  if (scope === 'all') return {};
  return { kind: scope };
}

// The chosen scope is a per-reader convenience, so it lives in this browser.
function dashboardScopeKey(id: string) {
  return `zgi:digital-human:${id}:dashboard-scope`;
}

function readDashboardScope(id: string): DashboardScope {
  try {
    const value = window.localStorage.getItem(dashboardScopeKey(id));
    return dashboardScopes.includes(value as DashboardScope) ? (value as DashboardScope) : 'all';
  } catch {
    return 'all';
  }
}

function writeDashboardScope(id: string, scope: DashboardScope) {
  try {
    if (scope === 'all') window.localStorage.removeItem(dashboardScopeKey(id));
    else window.localStorage.setItem(dashboardScopeKey(id), scope);
  } catch {
    // The scope then lasts for this visit only.
  }
}
