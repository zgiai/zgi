'use client';

import { useCallback, useDeferredValue, useEffect, useMemo, useRef, useState } from 'react';
import {
  Archive,
  ArrowRight,
  CalendarClock,
  CheckCircle2,
  ChevronRight,
  Inbox,
  Info,
  ListTodo,
  Loader2,
  Pause,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  SlidersHorizontal,
  X,
} from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import { getErrorMessage } from '@/utils/error-notifications';
import { translateTimezone } from '@/utils/timezone';
import { digitalHumanTaskDisplayStatus, digitalHumanTaskScheduleSummary } from '@/components/digital-human/digital-human-task-presentation';
import { DigitalHumanTaskActivityItem, DigitalHumanTaskHistoryPager, DigitalHumanTaskRunSummary } from '@/components/digital-human/digital-human-task-history';
import type {
  CreateDigitalHumanTaskRequest,
  DigitalHumanTask,
  DigitalHumanTaskActivity,
  DigitalHumanTaskList,
  DigitalHumanTaskPage,
  DigitalHumanTaskRun,
  UpdateDigitalHumanTaskRequest,
} from '@/services/types/digital-human';
import type { ApiResponseData } from '@/services/types/common';

const taskStatusFilters = [
  'open',
  'proposed',
  'pending',
  'in_progress',
  'waiting',
  'blocked',
  'completed',
  'canceled',
] as const;
const taskPriorityFilters = ['low', 'normal', 'high', 'urgent'] as const;

export interface DigitalHumanTaskPanelClient {
  list: (params?: Record<string, unknown>) => Promise<ApiResponseData<DigitalHumanTaskList>>;
  create: (request: CreateDigitalHumanTaskRequest) => Promise<ApiResponseData<DigitalHumanTask>>;
  get: (taskId: string) => Promise<ApiResponseData<DigitalHumanTask>>;
  update: (
    taskId: string,
    request: UpdateDigitalHumanTaskRequest
  ) => Promise<ApiResponseData<DigitalHumanTask>>;
  action: (
    taskId: string,
    action: string,
    revision: number
  ) => Promise<ApiResponseData<DigitalHumanTask>>;
  runs: (taskId: string, page: number) => Promise<ApiResponseData<DigitalHumanTaskPage<DigitalHumanTaskRun>>>;
  activities: (
    taskId: string,
    page: number
  ) => Promise<ApiResponseData<DigitalHumanTaskPage<DigitalHumanTaskActivity>>>;
}

export function DigitalHumanTaskPanel({
  client,
  title = 'my',
  compact = false,
  refreshKey = 0,
}: {
  client: DigitalHumanTaskPanelClient;
  title?: 'my' | 'shared';
  compact?: boolean;
  refreshKey?: number;
}) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const [tasks, setTasks] = useState<DigitalHumanTaskList | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);
  const [runs, setRuns] = useState<DigitalHumanTaskPage<DigitalHumanTaskRun> | null>(null);
  const [activities, setActivities] = useState<DigitalHumanTaskPage<DigitalHumanTaskActivity> | null>(null);
  const [runPage, setRunPage] = useState(1);
  const [activityPage, setActivityPage] = useState(1);
  const [historyRefresh, setHistoryRefresh] = useState(0);
  const [historyError, setHistoryError] = useState(false);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState({ name: '', goal: '' });
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('open');
  const [priority, setPriority] = useState('all');
  const [page, setPage] = useState(1);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const loadSequenceRef = useRef(0);
  const deferredSearch = useDeferredValue(search);
  const loadErrorText = t('tasks.loadError');
  const clientRef = useRef(client);
  const loadErrorTextRef = useRef(loadErrorText);
  const lastLoadFilterKeyRef = useRef<string | null>(null);
  clientRef.current = client;
  loadErrorTextRef.current = loadErrorText;
  const selected = useMemo(
    () => tasks?.items.find(item => item.id === selectedId) ?? null,
    [selectedId, tasks]
  );

  const filtersActive = search.trim() !== '' || status !== 'open' || priority !== 'all';
  // A short personal list does not need permanent filter controls.
  const filtersVisible = !compact || filtersOpen || filtersActive || (tasks?.total ?? 0) > 5;

  const load = useCallback(async () => {
    const sequence = ++loadSequenceRef.current;
    try {
      const statusValues =
        status === 'open'
          ? ['proposed', 'pending', 'in_progress', 'waiting', 'blocked']
          : status !== 'all'
            ? [status]
            : undefined;
      const response = await clientRef.current.list({
        page,
        page_size: 100,
        ...(deferredSearch.trim() ? { query: deferredSearch.trim() } : {}),
        ...(statusValues ? { status: statusValues } : {}),
        ...(priority !== 'all' ? { priority: [priority] } : {}),
      });
      if (sequence !== loadSequenceRef.current) return;
      setTasks(response.data);
      setSelectedId(current =>
        response.data.items.some(item => item.id === current)
          ? current
          : (response.data.items[0]?.id ?? null)
      );
    } catch (error) {
      if (sequence === loadSequenceRef.current) toast.error(getErrorMessage(error) || loadErrorTextRef.current);
    } finally {
      if (sequence === loadSequenceRef.current) {
        setLoading(false);
        setRefreshing(false);
      }
    }
  }, [deferredSearch, page, priority, status]);

  const loadFilterKey = `${deferredSearch}\u0000${priority}\u0000${status}\u0000${page}\u0000${refreshKey}`;
  useEffect(() => {
    if (lastLoadFilterKeyRef.current === loadFilterKey) return;
    if (refreshKey > 0) setRefreshing(true);
    lastLoadFilterKeyRef.current = loadFilterKey;
    void load();
  }, [load, loadFilterKey, refreshKey]);

  // Scheduler state changes happen outside the chat turn. Keep the panel
  // live while a task can still transition, then stop polling once it reaches
  // a terminal/blocked state so completed history does not create background
  // traffic.
  const shouldPoll = useMemo(
    () =>
      tasks?.items.some(task =>
        task.work_status === 'in_progress' ||
        ((task.work_status === 'pending' || task.work_status === 'waiting') &&
          Boolean(task.next_run_at))
      ) ?? false,
    [tasks]
  );

  useEffect(() => {
    if (!shouldPoll) return;
    const timer = window.setInterval(() => {
      void load();
    }, 5000);
    return () => window.clearInterval(timer);
  }, [load, shouldPoll]);

  useEffect(() => {
    setRunPage(1);
    setActivityPage(1);
    setRuns(null);
    setActivities(null);
    setHistoryError(false);
  }, [selectedId]);

  useEffect(() => {
    if (!selected) {
      setRuns(null);
      setActivities(null);
      return;
    }
    let active = true;
    void Promise.all([
      clientRef.current.runs(selected.id, runPage),
      clientRef.current.activities(selected.id, activityPage),
    ])
      .then(([runResponse, activityResponse]) => {
        if (!active) return;
        setRuns(runResponse.data);
        setActivities(activityResponse.data);
        setHistoryError(false);
      })
      .catch(() => {
        if (!active) return;
        setRuns(null);
        setActivities(null);
        setHistoryError(true);
      });
    return () => { active = false; };
  }, [selectedId, selected, runPage, activityPage, historyRefresh]);

  const visibleTasks = useMemo(() => tasks?.items ?? [], [tasks]);

  const select = (task: DigitalHumanTask) => {
    setSelectedId(task.id);
    setEditing(false);
    setDraft({ name: task.name, goal: task.goal });
    if (compact) setDetailOpen(true);
  };

  const create = async () => {
    if (!draft.name.trim() || !draft.goal.trim()) return;
    try {
      await client.create({
        name: draft.name.trim(),
        goal: draft.goal.trim(),
        schedule: { type: 'manual' },
        delivery_mode: 'task_only',
      });
      toast.success(t('tasks.created'));
      setDraft({ name: '', goal: '' });
      setEditing(false);
      await load();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('tasks.loadError'));
    }
  };

  const update = async () => {
    if (!selected || !draft.name.trim() || !draft.goal.trim()) return;
    try {
      await clientRef.current.update(selected.id, {
        name: draft.name.trim(),
        goal: draft.goal.trim(),
        expected_revision: selected.revision,
      });
      toast.success(t('tasks.updated'));
      setEditing(false);
      await load();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('tasks.loadError'));
      await load();
    }
  };

  const action = async (name: string) => {
    if (!selected) return;
    try {
      await clientRef.current.action(selected.id, name, selected.revision);
      toast.success(t('tasks.actionApplied'));
      await load();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('tasks.loadError'));
      await load();
    }
  };

  const titleText = title === 'my' ? t('tasks.myTitle') : t('tasks.title');
  // Lifecycle operations are deterministic user controls. Keep content edits
  // and proposal confirmation in the conversation, where the model can
  // explain the change and collect any missing details.
  // A shared task shown in the personal view is read-only for this reader.
  const selectedChangeable = selected?.can_change !== false;
  const allowDirectMutations = (title === 'shared' || title === 'my') && selectedChangeable;
  const allowDirectEdit = (title === 'shared' || !compact) && selectedChangeable;
  return (
    <section
      className={
        compact ? 'flex h-full min-h-0 flex-col bg-card' : 'rounded-xl border bg-background'
      }
    >
      <div className="flex items-start justify-between gap-3 border-b px-4 py-3.5">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <ListTodo className="size-4" />
            </span>
            <h2 className="truncate font-semibold">{titleText}</h2>
            {tasks && tasks.total > 0 ? (
              <span className="shrink-0 rounded-full bg-muted px-1.5 text-xs tabular-nums text-muted-foreground">
                {tasks.total}
              </span>
            ) : null}
            {compact && title === 'my' ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    className="inline-flex size-5 shrink-0 items-center justify-center rounded-full text-muted-foreground/80 transition-colors hover:bg-muted hover:text-foreground"
                    aria-label={t('tasks.createFromChat')}
                  >
                    <Info className="size-3.5" />
                  </button>
                </TooltipTrigger>
                <TooltipContent side="left" className="max-w-64 leading-5">
                  <span className="block">{t('tasks.myDescription')}</span>
                  <span className="mt-1 block">{t('tasks.createFromChat')}</span>
                </TooltipContent>
              </Tooltip>
            ) : null}
          </div>
          {!compact ? (
            <p className="mt-1 text-xs text-muted-foreground">
              {title === 'my' ? t('tasks.myDescription') : t('tasks.description')}
            </p>
          ) : null}
        </div>
        <div className="flex shrink-0 gap-1">
          {compact ? (
            <Button
              variant="ghost"
              size="sm"
              className={filtersVisible ? 'h-8 w-8 bg-muted p-0' : 'h-8 w-8 p-0'}
              onClick={() => setFiltersOpen(value => !value)}
              aria-label={t('tasks.filters.toggle')}
              aria-expanded={filtersVisible}
            >
              <SlidersHorizontal className="size-4" />
            </Button>
          ) : null}
          <Button
            variant="ghost"
            size="sm"
            className="h-8 w-8 p-0"
            onClick={() => {
              setRefreshing(true);
              void load();
            }}
            disabled={refreshing}
            aria-label={t('common.refresh')}
          >
            <RefreshCw className={refreshing ? 'size-4 animate-spin' : 'size-4'} />
          </Button>
          {!compact ? (
            <Button
              variant="ghost"
              size="sm"
              className="h-8 w-8 p-0"
              onClick={() => {
                setSelectedId(null);
                setEditing(true);
                setDraft({ name: '', goal: '' });
              }}
              aria-label={t('tasks.create')}
            >
              <Plus className="size-4" />
            </Button>
          ) : null}
        </div>
      </div>
      {filtersVisible ? (
        <div
          className={
            compact
              ? 'grid gap-2 border-b p-3'
              : 'grid gap-2 border-b p-3 sm:grid-cols-[minmax(0,1fr)_auto_auto]'
          }
        >
          <Input
            value={search}
            onChange={event => {
              setSearch(event.target.value);
              setPage(1);
            }}
            placeholder={t('tasks.filters.search')}
            aria-label={t('tasks.filters.search')}
          />
          <div className={compact ? 'grid grid-cols-2 gap-2' : 'contents'}>
            <Select value={status} onValueChange={value => {
              setStatus(value);
              setPage(1);
            }}>
              <SelectTrigger className="h-9 min-w-0" aria-label={t('tasks.filters.status')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('tasks.filters.allStatuses')}</SelectItem>
                {taskStatusFilters.map(value => (
                  <SelectItem key={value} value={value}>
                    {t(`tasks.status.${value}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={priority} onValueChange={value => {
              setPriority(value);
              setPage(1);
            }}>
              <SelectTrigger className="h-9 min-w-0" aria-label={t('tasks.filters.priority')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('tasks.filters.allPriorities')}</SelectItem>
                {taskPriorityFilters.map(value => (
                  <SelectItem key={value} value={value}>
                    {t(`tasks.priority.${value}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      ) : null}
      <div
        className={
          compact
            ? 'flex min-h-0 flex-1 flex-col'
            : 'grid gap-4 p-4 md:grid-cols-[minmax(12rem,18rem)_minmax(0,1fr)]'
        }
      >
        <div
          className={
            compact
              ? 'min-h-0 flex-1 overflow-y-auto border-b p-2'
              : 'min-h-0 overflow-y-auto border-r p-2'
          }
        >
          {loading ? (
            <Loader2 className="mx-auto mt-6 size-4 animate-spin" />
          ) : visibleTasks.length === 0 ? (
            <div className="flex min-h-28 flex-col items-center justify-center gap-2 px-3 py-5 text-center">
              <Inbox className="size-6 text-muted-foreground/60" />
              <p className="text-xs text-muted-foreground">
                {title === 'my' ? t('tasks.myEmpty') : t('tasks.empty')}
              </p>
              {!compact ? (
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-8 gap-1.5 text-xs"
                  onClick={() => {
                    setSelectedId(null);
                    setEditing(true);
                    setDraft({ name: '', goal: '' });
                  }}
                >
                  <Plus className="size-3.5" />
                  {t('tasks.create')}
                </Button>
              ) : null}
            </div>
          ) : (
            visibleTasks.map(task => {
              const displayStatus = digitalHumanTaskDisplayStatus(task);
              const statusTone = taskStatusTone(displayStatus);
              return (
                <button
                  key={task.id}
                  type="button"
                  onClick={() => select(task)}
                  className={`group mb-1 w-full rounded-lg border p-2.5 text-left text-sm transition-colors hover:border-primary/30 hover:bg-muted/70 ${selected?.id === task.id ? 'border-primary/40 bg-muted shadow-sm' : 'border-transparent'}`}
                  aria-label={`${task.name}, ${t(`tasks.status.${displayStatus}`)}`}
                >
                  <div className="flex min-w-0 items-start gap-2">
                    <div className="min-w-0 flex-1">
                      <div className="truncate font-medium">{task.name}</div>
                      <div className="mt-1 flex flex-wrap items-center gap-1.5">
                        <span className={`inline-flex items-center gap-1.5 text-xs font-medium ${statusTone.text}`}>
                          <span className={`size-1.5 shrink-0 rounded-full ${statusTone.dot}`} aria-hidden="true" />
                          {t(`tasks.status.${displayStatus}`)}
                        </span>
                        {task.priority !== 'normal' ? (
                          <Badge variant="secondary" className="text-[11px]" title={t('tasks.priorityNote')}>
                            {t(`tasks.priority.${task.priority}`)}
                          </Badge>
                        ) : null}
                        {title === 'my' && task.can_change === false ? (
                          <Badge variant="subtle" className="text-[11px]" title={t('tasks.sharedReadOnly')}>
                            {t('tasks.sharedBadge')}
                          </Badge>
                        ) : null}
                      </div>
                    </div>
                    <ChevronRight className="mt-0.5 size-4 shrink-0 text-muted-foreground/50 transition-transform group-hover:translate-x-0.5 group-hover:text-foreground" />
                  </div>
                  <div className="mt-1.5 flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
                    <CalendarClock className="size-3.5 shrink-0" />
                    <span className="truncate">{digitalHumanTaskScheduleSummary(task, t, locale)}</span>
                  </div>
                  {task.next_run_at ? (
                    <div className="mt-0.5 truncate pl-[1.125rem] text-xs text-muted-foreground">
                      {t('tasks.fields.nextRun')} · {formatTaskDate(task.next_run_at, task.schedule.timezone)}
                    </div>
                  ) : null}
                  {task.last_run_at ? (
                    <div className="mt-0.5 truncate pl-[1.125rem] text-xs text-muted-foreground/80">
                      {t('tasks.fields.lastRun')} · {formatTaskDate(task.last_run_at, task.schedule.timezone)}
                    </div>
                  ) : null}
                </button>
              );
            })
          )}
          {tasks && (page > 1 || tasks.has_more) ? (
            <div className="mt-3 flex flex-wrap items-center justify-between gap-2 border-t px-1 pt-3 text-xs text-muted-foreground">
              <span>{t('tasks.pagination.range', {
                from: tasks.items.length ? (page - 1) * 100 + 1 : 0,
                to: tasks.items.length ? (page - 1) * 100 + tasks.items.length : 0,
                total: tasks.total,
              })}</span>
              <div className="flex gap-1">
                <Button variant="outline" size="sm" disabled={page <= 1 || refreshing}
                  onClick={() => { setPage(page - 1); setLoading(true); setDetailOpen(false); }}>
                  {t('tasks.pagination.previous')}
                </Button>
                <Button variant="outline" size="sm" disabled={!tasks.has_more || refreshing}
                  onClick={() => { setPage(page + 1); setLoading(true); setDetailOpen(false); }}>
                  {t('tasks.pagination.next')}
                </Button>
              </div>
            </div>
          ) : null}
        </div>
        {!compact ? (
          <div className="min-w-0 p-4">
            <TaskDetailContent
              selected={selected}
              editing={editing}
              draft={draft}
              history={{ runs, activities, runPage, activityPage, historyError,
                onRunPage: page => { setRunPage(page); setRuns(null); },
                onActivityPage: page => { setActivityPage(page); setActivities(null); },
                onRetry: () => setHistoryRefresh(value => value + 1) }}
              title={title}
              onDraftChange={setDraft}
              onCreate={create}
              onUpdate={update}
              onCancelEdit={() => setEditing(false)}
              onAction={action}
              allowProposalApproval
              allowDirectMutations={allowDirectMutations}
              allowDirectEdit={allowDirectEdit}
              onStartEdit={task => {
                setDraft({ name: task.name, goal: task.goal });
                setEditing(true);
              }}
            />
          </div>
        ) : null}
      </div>
      {compact ? (
        <Sheet open={detailOpen && Boolean(selected)} onOpenChange={setDetailOpen}>
          <SheetContent side="right" className="w-full overflow-y-auto p-0 sm:max-w-lg">
            <SheetTitle className="sr-only">{selected?.name ?? t('tasks.myTitle')}</SheetTitle>
            <div className="p-4">
              <TaskDetailContent
                selected={selected}
                editing={editing}
                draft={draft}
                history={{ runs, activities, runPage, activityPage, historyError,
                  onRunPage: page => { setRunPage(page); setRuns(null); },
                onActivityPage: page => { setActivityPage(page); setActivities(null); },
                  onRetry: () => setHistoryRefresh(value => value + 1) }}
                title={title}
                onDraftChange={setDraft}
                onCreate={create}
                onUpdate={update}
                onCancelEdit={() => setEditing(false)}
                onAction={action}
                allowProposalApproval={false}
                allowDirectMutations={allowDirectMutations}
                allowDirectEdit={allowDirectEdit}
                onStartEdit={task => {
                  setDraft({ name: task.name, goal: task.goal });
                  setEditing(true);
                }}
              />
            </div>
          </SheetContent>
        </Sheet>
      ) : null}
    </section>
  );
}

interface TaskDraft {
  name: string;
  goal: string;
}

interface TaskHistoryState {
  runs: DigitalHumanTaskPage<DigitalHumanTaskRun> | null;
  activities: DigitalHumanTaskPage<DigitalHumanTaskActivity> | null;
  runPage: number;
  activityPage: number;
  historyError: boolean;
  onRunPage: (page: number) => void;
  onActivityPage: (page: number) => void;
  onRetry: () => void;
}

function TaskDetailContent({
  selected,
  editing,
  draft,
  history,
  title,
  onDraftChange,
  onCreate,
  onUpdate,
  onCancelEdit,
  onAction,
  allowProposalApproval,
  allowDirectMutations,
  allowDirectEdit,
  onStartEdit,
}: {
  selected: DigitalHumanTask | null;
  editing: boolean;
  draft: TaskDraft;
  history: TaskHistoryState;
  title: 'my' | 'shared';
  onDraftChange: (draft: TaskDraft) => void;
  onCreate: () => Promise<void>;
  onUpdate: () => Promise<void>;
  onCancelEdit: () => void;
  onAction: (name: string) => Promise<void>;
  allowProposalApproval: boolean;
  allowDirectMutations: boolean;
  allowDirectEdit: boolean;
  onStartEdit: (task: DigitalHumanTask) => void;
}) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const runs = history.runs?.items ?? [];
  const activities = history.activities?.items ?? [];
  const [confirmingAction, setConfirmingAction] = useState<'cancel' | 'archive' | null>(null);

  if (editing) {
    return (
      <div className="space-y-4">
        <div>
          <p className="text-sm font-semibold">
            {selected ? t('tasks.editor.editTitle') : t('tasks.editor.createTitle')}
          </p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t('tasks.editor.description')}
          </p>
        </div>
        <Input
          value={draft.name}
          onChange={event => onDraftChange({ ...draft, name: event.target.value })}
          placeholder={t('tasks.editor.namePlaceholder')}
        />
        <Textarea
          value={draft.goal}
          onChange={event => onDraftChange({ ...draft, goal: event.target.value })}
          placeholder={t('tasks.editor.goalPlaceholder')}
          className="min-h-28 resize-none"
        />
        <div className="flex gap-2">
          <Button className="flex-1" onClick={() => void (selected ? onUpdate() : onCreate())}>
            {selected ? t('common.save') : t('tasks.create')}
          </Button>
          <Button variant="ghost" onClick={onCancelEdit}>
            <X className="mr-1 size-4" />
            {t('common.cancel')}
          </Button>
        </div>
      </div>
    );
  }

  if (!selected) {
    return (
      <div className="flex h-full min-h-40 flex-col items-center justify-center gap-2 text-center">
        <CheckCircle2 className="size-7 text-muted-foreground/50" />
        <p className="text-sm font-medium text-foreground/80">{t('tasks.empty')}</p>
        <p className="max-w-[18rem] text-xs leading-5 text-muted-foreground">
          {title === 'my' ? t('tasks.myEmptyDescription') : t('tasks.emptyDescription')}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div>
        <h3 className="font-semibold">{selected.name}</h3>
        <p className="mt-1 text-sm text-muted-foreground">{selected.goal}</p>
      </div>
      <div className="flex flex-wrap gap-2">
        <Badge>{t(`tasks.status.${digitalHumanTaskDisplayStatus(selected)}`)}</Badge>
        {allowDirectMutations && selected.work_status === 'proposed' && allowProposalApproval ? (
          <Button size="sm" onClick={() => void onAction('confirm')}>
            {t('tasks.actions.confirm')}
          </Button>
        ) : null}
        {!allowDirectMutations && selected.work_status === 'proposed' ? (
          <span className="self-center text-xs text-muted-foreground">
            {t('tasks.approvalInChat')}
          </span>
        ) : null}
        {allowDirectMutations &&
        ['pending', 'waiting'].includes(selected.work_status) ? (
          <Button size="sm" onClick={() => void onAction('run_now')}>
            <Play className="mr-1 size-3" />
            {t('tasks.actions.runNow')}
          </Button>
        ) : null}
        {allowDirectMutations && selected.lifecycle_status === 'active' ? (
          <Button size="sm" variant="outline" onClick={() => void onAction('pause')}>
            <Pause className="mr-1 size-3" />
            {t('tasks.actions.pause')}
          </Button>
        ) : allowDirectMutations && selected.lifecycle_status === 'paused' ? (
          <Button size="sm" variant="outline" onClick={() => void onAction('resume')}>
            <Play className="mr-1 size-3" />
            {t('tasks.actions.resume')}
          </Button>
        ) : null}
        {allowDirectMutations && selected.work_status === 'blocked' ? (
          <Button size="sm" variant="outline" onClick={() => void onAction('retry')}>
            <RotateCcw className="mr-1 size-3" />
            {t('tasks.actions.retry')}
          </Button>
        ) : null}
        {allowDirectMutations &&
        !['completed', 'canceled'].includes(selected.work_status) &&
        selected.lifecycle_status !== 'archived' ? (
          <Button size="sm" variant="outline" onClick={() => setConfirmingAction('cancel')}>
            {t('tasks.actions.cancel')}
          </Button>
        ) : null}
        {allowDirectMutations && selected.lifecycle_status !== 'archived' ? (
          <Button size="sm" variant="ghost" onClick={() => setConfirmingAction('archive')}>
            <Archive className="mr-1 size-3" />
            {title === 'my' ? t('tasks.actions.remove') : t('tasks.actions.archive')}
          </Button>
        ) : null}
        {allowDirectEdit ? (
          <Button size="sm" variant="ghost" onClick={() => onStartEdit(selected)}>
            {t('tasks.editor.editTitle')}
          </Button>
        ) : null}
      </div>
      <ConfirmDialog
        open={confirmingAction !== null}
        onOpenChange={open => {
          if (!open) setConfirmingAction(null);
        }}
        title={
          confirmingAction === 'cancel'
            ? t('tasks.confirm.cancelTitle')
            : t(title === 'my' ? 'tasks.confirm.removeTitle' : 'tasks.confirm.archiveTitle')
        }
        description={
          confirmingAction === 'cancel'
            ? t('tasks.confirm.cancelDescription')
            : t(title === 'my' ? 'tasks.confirm.removeDescription' : 'tasks.confirm.archiveDescription')
        }
        confirmText={
          confirmingAction === 'cancel'
            ? t('tasks.actions.cancel')
            : t(title === 'my' ? 'tasks.actions.remove' : 'tasks.actions.archive')
        }
        variant="warning"
        onConfirm={() => {
          const action = confirmingAction;
          setConfirmingAction(null);
          if (action) void onAction(action);
        }}
      />
      <section className="rounded-lg border bg-muted/20 p-3">
        <div className="flex items-center gap-2 text-sm font-medium">
          <CalendarClock className="size-4 text-primary" />
          {t('tasks.fields.schedule')}
        </div>
        <div className="mt-3 grid gap-2 text-xs sm:grid-cols-2">
          <TaskScheduleDetail
            label={t('tasks.fields.schedule')}
            value={digitalHumanTaskScheduleSummary(selected, t, locale)}
          />
          {selected.schedule.type === 'once' && selected.schedule.run_at ? (
            <TaskScheduleDetail
              label={t('tasks.fields.runAt')}
              value={formatTaskDate(selected.schedule.run_at)}
            />
          ) : null}
          {selected.schedule.timezone ? (
            <TaskScheduleDetail
              label={t('tasks.fields.timezone')}
              value={translateTimezone(selected.schedule.timezone, locale)}
            />
          ) : null}
          {selected.next_run_at ? (
            <TaskScheduleDetail
              label={t('tasks.fields.nextRun')}
              value={formatTaskDate(selected.next_run_at)}
            />
          ) : null}
        </div>
      </section>
      <section className="rounded-lg border bg-muted/20 p-3">
        <div className="flex items-center gap-2 text-sm font-medium">
          <ArrowRight className="size-4 text-primary" />
          {t('tasks.fields.delivery')}
        </div>
        <div className="mt-3 grid gap-2 text-xs sm:grid-cols-2">
          <TaskScheduleDetail
            label={t('tasks.fields.delivery')}
            value={t(
              selected.delivery_target === 'source_conversation' ||
                selected.delivery_mode === 'source_conversation'
                ? 'tasks.delivery.source_conversation'
                : 'tasks.delivery.task_only'
            )}
          />
          {selected.delivery_fallback === 'task_center' ||
          selected.delivery_mode === 'source_conversation' ? (
            <TaskScheduleDetail
              label={t('tasks.fields.deliveryFallback')}
              value={t('tasks.delivery.task_only')}
            />
          ) : null}
        </div>
      </section>
      <div className="border-t pt-3">
        <h4 className="text-xs font-medium text-muted-foreground">{t('tasks.detail.runs')}</h4>
        {history.historyError ? (
          <Button variant="outline" size="sm" onClick={history.onRetry}>{t('tasks.retry')}</Button>
        ) : !history.runs ? (
          <Loader2 className="mt-2 size-4 animate-spin" aria-label={t('tasks.detail.runs')} />
        ) : runs.length === 0 ? (
          <p className="mt-2 text-xs text-muted-foreground">{t('tasks.detail.noRuns')}</p>
        ) : runs.map(run => (
          <div key={run.id} className="mt-2"><DigitalHumanTaskRunSummary run={run} /></div>
        ))}
        <DigitalHumanTaskHistoryPager data={history.runs} page={history.runPage} onPage={history.onRunPage} />
      </div>
      <div className="border-t pt-4">
        <div className="flex items-center justify-between gap-2">
          <h4 className="text-xs font-medium text-muted-foreground">{t('tasks.detail.activities')}</h4>
          {history.activities ? <span className="text-[11px] text-muted-foreground">{history.activities.total}</span> : null}
        </div>
        {history.historyError ? (
          <Button variant="outline" size="sm" onClick={history.onRetry}>{t('tasks.retry')}</Button>
        ) : !history.activities ? (
          <Loader2 className="mt-2 size-4 animate-spin" aria-label={t('tasks.detail.activities')} />
        ) : activities.length === 0 ? (
          <p className="mt-2 text-xs text-muted-foreground">{t('tasks.detail.noActivities')}</p>
        ) : (
          <ol className="mt-3 space-y-0">
            {activities.map((activity, index) => (
              <DigitalHumanTaskActivityItem key={activity.id} activity={activity} locale={locale}
                isLast={index === activities.length - 1} defaultOpen={index === 0 && history.activityPage === 1} />
            ))}
          </ol>
        )}
        <DigitalHumanTaskHistoryPager data={history.activities} page={history.activityPage} onPage={history.onActivityPage} />
      </div>
    </div>
  );
}

function TaskScheduleDetail({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <div className="text-muted-foreground">{label}</div>
      <div className="mt-0.5 break-words font-medium text-foreground">{value}</div>
    </div>
  );
}

function formatTaskDate(value: string, timezone?: string): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return value;
  const options: Intl.DateTimeFormatOptions = { dateStyle: 'medium', timeStyle: 'short' };
  if (timezone) {
    // Show the task's own zone so the time matches its schedule label.
    try {
      // dateStyle/timeStyle cannot be combined with timeZoneName.
      return new Date(timestamp).toLocaleString(undefined, {
        year: 'numeric', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit',
        timeZone: timezone, timeZoneName: 'short',
      });
    } catch {
      // Unknown zone in this browser: fall back to the viewer's zone.
    }
  }
  return new Date(timestamp).toLocaleString(undefined, options);
}

function taskStatusTone(status: string): { dot: string; text: string } {
  switch (status) {
    case 'proposed':
    case 'waiting':
      return { dot: 'bg-warning', text: 'text-warning-foreground' };
    case 'blocked':
      return { dot: 'bg-destructive', text: 'text-destructive' };
    case 'in_progress':
      return { dot: 'bg-primary', text: 'text-primary' };
    case 'recurring_active':
    case 'manual_ready':
    case 'pending':
      return { dot: 'bg-success', text: 'text-success-foreground' };
    default:
      return { dot: 'bg-muted-foreground/50', text: 'text-muted-foreground' };
  }
}
