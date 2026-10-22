'use client';

import { useEffect, useRef, useState, type ReactNode } from 'react';
import Link from 'next/link';
import { useDigitalHumanObjectHref } from '@/components/digital-human/digital-human-object-link';
import { RequesterOrigin } from '@/components/digital-human/digital-human-task-requesters';
import { ArrowLeft, CheckCircle2, ChevronDown, CircleDashed, FileText, MessageSquare, Paperclip, RotateCcw, XCircle } from 'lucide-react';
import { toast } from 'sonner';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { ProtectedGeneratedFileCard } from '@/components/chat/variants/aichat/protected-generated-file-card';
import { digitalHumanService } from '@/services/digital-human.service';
import { Textarea } from '@/components/ui/textarea';
import { useDigitalHumanTaskReport, useDigitalHumanTaskReportReview, useDigitalHumanTaskReports } from '@/hooks/digital-human/use-digital-human-tasks';
import { useInboxMutations } from '@/hooks/use-inbox';
import { useLocale } from '@/hooks/use-locale';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { plainTextPreview } from '@/components/digital-human/digital-human-run-presentation';
import { formatTaskTime } from '@/components/digital-human/digital-human-task-presentation';
import type { ApiResponseData } from '@/services/types/common';
import type { AIChatGeneratedFile } from '@/services/types/aichat';
import type {
  DigitalHumanTaskReport,
  DigitalHumanTaskReportFile,
  DigitalHumanTaskReportList,
  ReviewDigitalHumanTaskReportRequest,
  DigitalHumanTaskReportOutcome,
  DigitalHumanTaskReportParams,
  DigitalHumanTaskReportReviewState,
  DigitalHumanTaskReportSummary,
} from '@/services/types/digital-human';

const outcomeVariant: Record<DigitalHumanTaskReportOutcome, 'success' | 'warning' | 'destructive' | 'subtle'> = {
  done: 'success',
  partial: 'warning',
  not_done: 'subtle',
  failed: 'destructive',
};

export function TaskReportOutcomeBadge({ outcome }: { outcome: DigitalHumanTaskReportOutcome }) {
  const t = useT('digitalHumans');
  return <Badge variant={outcomeVariant[outcome]}>{t(`tasks.reports.outcome.${outcome}`)}</Badge>;
}

function ReviewStateLabel({ state }: { state: DigitalHumanTaskReportReviewState }) {
  const t = useT('digitalHumans');
  if (state === 'new') {
    return <span className="text-xs font-medium text-primary">{t('tasks.reports.review.new')}</span>;
  }
  return <span className="text-xs text-muted-foreground">{t(`tasks.reports.review.${state}`)}</span>;
}

/** The latest report of a task, in one line, for task lists and cards. */
export function TaskLatestReport({ report, onOpen }: { report?: DigitalHumanTaskReportSummary; onOpen?: (reportId: string) => void }) {
  const t = useT('digitalHumans');
  if (!report) {
    return <span className="text-xs text-muted-foreground">{t('tasks.reports.none')}</span>;
  }
  const content = (
    <span className="flex min-w-0 items-center gap-2">
      <TaskReportOutcomeBadge outcome={report.outcome} />
      <span className="min-w-0 truncate text-xs text-muted-foreground">{report.summary}</span>
      {report.review_state === 'new' ? <span className="size-1.5 shrink-0 rounded-full bg-primary" aria-label={t('tasks.reports.review.new')} /> : null}
    </span>
  );
  if (!onOpen) return content;
  return (
    <button
      type="button"
      className="flex min-w-0 max-w-full items-center text-left hover:underline"
      onClick={event => {
        event.stopPropagation();
        onOpen(report.id);
      }}
    >
      {content}
    </button>
  );
}

const criterionIcon = {
  met: <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-success" />,
  not_met: <XCircle className="mt-0.5 size-4 shrink-0 text-destructive" />,
  unknown: <CircleDashed className="mt-0.5 size-4 shrink-0 text-muted-foreground" />,
};

/** The full report with its review actions, for the task center. */
export function TaskReportDetail({
  digitalHumanId,
  report,
  mine,
  onOpenRun,
  stickyActions,
  actionsExtra,
  onReviewed,
}: {
  digitalHumanId: string;
  report: DigitalHumanTaskReport;
  mine: boolean;
  // Called once a review is saved, for example to move on to the next report.
  onReviewed?: (action: 'accept' | 'rework') => void;
  // Shows the run that produced the report in place, for readers of run logs.
  onOpenRun?: (runId: string) => void;
  stickyActions?: boolean;
  actionsExtra?: ReactNode;
}) {
  const review = useDigitalHumanTaskReportReview(digitalHumanId, mine);
  const runId = report.digital_human_run_id;
  return (
    <TaskReportDetailView
      report={report}
      pending={review.isPending}
      stickyActions={stickyActions}
      actionsExtra={actionsExtra}
      readFile={(fileId, signal) => digitalHumanService.downloadTaskReportFile(digitalHumanId, report.id, fileId, mine, signal)}
      followUpHref={(draft, conversationId) => digitalHumanChatFollowUpHref(digitalHumanId, draft, conversationId)}
      onOpenRun={runId && !mine && onOpenRun ? () => onOpenRun(runId) : undefined}
      onReview={async request => {
        await review.mutateAsync({ reportId: report.id, request });
        onReviewed?.(request.action);
      }}
    />
  );
}

// A report file shown with the card that previews and downloads protected
// generated files; its bytes always come from the report's own endpoint.
function reportGeneratedFile(file: DigitalHumanTaskReportFile): AIChatGeneratedFile {
  const extension = file.name.includes('.') ? `.${file.name.split('.').pop()}` : '';
  return {
    file_id: file.id, filename: file.name, mime_type: file.mime_type || 'application/octet-stream', size: file.size,
    artifact_type: 'file', access_mode: 'authenticated', skill_id: '', tool_name: '', extension, url: '', transfer_method: 'tool_file', created_at: 0,
  };
}

/** A report and its review actions, independent of where it was loaded. */
export function TaskReportDetailView({
  report,
  pending = false,
  onOpenRun,
  readFile,
  stickyActions = false,
  actionsExtra,
  followUpHref,
  onReview,
}: {
  report: DigitalHumanTaskReport;
  pending?: boolean;
  // Where a holder follows the report up in a conversation, with a draft;
  // without it only supplementing in place is offered.
  followUpHref?: (draft: string, conversationId?: string) => string;
  stickyActions?: boolean;
  // Shown at the end of the action bar, such as a link to the next report.
  actionsExtra?: ReactNode;
  onOpenRun?: () => void;
  // Reads a file the report carries through the surface's own API.
  readFile?: (fileId: string, signal?: AbortSignal) => Promise<Blob>;
  onReview: (request: { action: 'accept' } | { action: 'rework'; feedback: string }) => Promise<void>;
}) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const [reworkOpen, setReworkOpen] = useState(false);
  const [feedback, setFeedback] = useState('');
  const body = report.body?.trim() && report.body.trim() !== report.summary.trim() ? report.body : '';
  // A failed run's issue is often its summary; show it once.
  const issues = report.issues?.trim() && report.issues.trim() !== report.summary.trim() ? report.issues : '';
  const metCount = report.criteria.filter(criterion => criterion.status === 'met').length;

  const submit = async (action: 'accept' | 'rework') => {
    try {
      await onReview(action === 'accept' ? { action } : { action, feedback: feedback.trim() });
    } catch {
      return;
    }
    toast.success(action === 'accept' ? t('tasks.reports.accepted') : t('tasks.reports.reworkRequested'));
    setReworkOpen(false);
    setFeedback('');
  };

  return (
    <div className="space-y-5">
      <div className="space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <TaskReportOutcomeBadge outcome={report.outcome} />
          <ReviewStateLabel state={report.review_state} />
          <span className="text-xs text-muted-foreground" title={new Date(report.created_at).toLocaleString(locale)}>
            {formatTaskTime(report.created_at, locale)}
            {report.attempt > 1 ? ` · ${t('tasks.reports.attempt', { value: report.attempt })}` : ''}
          </span>
        </div>
        <RequesterOrigin requester={report.requester} />
        {/* The summary is what the reader needs first. */}
        <p className="rounded-lg border-l-2 border-primary/60 bg-muted/40 px-3 py-2.5 text-sm font-medium leading-6">{report.summary}</p>
      </div>
      {report.criteria.length ? (
        <section>
          <h4 className="flex items-center gap-2 text-sm font-semibold">
            {t('tasks.reports.criteria')}
            <span className={cn('text-xs font-normal tabular-nums', metCount === report.criteria.length ? 'text-success' : 'text-muted-foreground')}>
              {t('tasks.reports.criteriaMet', { met: metCount, total: report.criteria.length })}
            </span>
          </h4>
          <ul className="mt-2 space-y-2">
            {report.criteria.map((criterion, index) => (
              <li key={`${index}-${criterion.criterion}`} className="flex gap-2 text-sm">
                {criterionIcon[criterion.status]}
                <span>
                  {criterion.criterion}
                  <span className="ml-1 text-xs text-muted-foreground">· {t(`tasks.reports.criterion.${criterion.status}`)}</span>
                  {criterion.note ? <span className="block text-xs text-muted-foreground">{criterion.note}</span> : null}
                </span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {body ? (
        <section>
          <h4 className="text-sm font-semibold">{t('tasks.reports.result')}</h4>
          <div className="mt-2 rounded-lg border px-4 py-3 text-sm">
            <MarkdownViewer content={body} />
          </div>
        </section>
      ) : null}
      {report.files?.length ? (
        <section>
          <h4 className="text-sm font-semibold">{t('tasks.reports.files')}</h4>
          <div className="mt-2 space-y-2">
            {report.files.map(file => (
              <ProtectedGeneratedFileCard
                key={file.id}
                conversationId={report.id}
                readFile={readFile ? (_, id, signal) => readFile(id, signal) : undefined}
                file={reportGeneratedFile(file)}
              />
            ))}
          </div>
        </section>
      ) : null}
      {issues ? (
        <section>
          <h4 className="text-sm font-semibold">{t('tasks.reports.issues')}</h4>
          <p className="mt-2 whitespace-pre-wrap text-sm text-muted-foreground">{issues}</p>
        </section>
      ) : null}
      <TaskReportFollowUp report={report} pending={pending} followUpHref={followUpHref} onReview={onReview} />
      {report.review_feedback ? (
        <section className="rounded-md border border-warning/30 bg-warning/5 p-3">
          <h4 className="text-xs font-semibold text-warning">{t('tasks.reports.feedback')}</h4>
          <p className="mt-1 whitespace-pre-wrap text-sm">{report.review_feedback}</p>
        </section>
      ) : null}
      <div
        className={cn(
          'flex flex-wrap items-center gap-2',
          // Pinned to the bottom of a scrolling pane, so reviewing never needs
          // a scroll past a long result.
          stickyActions && 'sticky bottom-0 -mx-4 border-t bg-background px-4 py-3'
        )}
      >
        {report.can_review && report.review_state === 'new' ? (
          <Button size="sm" disabled={pending} onClick={() => void submit('accept')}>
            <CheckCircle2 className="mr-1 size-4" />
            {t('tasks.reports.accept')}
          </Button>
        ) : null}
        {report.can_review && report.review_state !== 'rework_requested' ? (
          <Button size="sm" variant="outline" disabled={pending} onClick={() => setReworkOpen(true)}>
            <RotateCcw className="mr-1 size-4" />
            {t('tasks.reports.rework')}
          </Button>
        ) : null}
        {onOpenRun ? (
          <Button size="sm" variant="ghost" onClick={onOpenRun}>
            <FileText className="mr-1 size-4" />
            {t('tasks.runTrace.open')}
          </Button>
        ) : null}
        {actionsExtra ? <div className="ml-auto flex items-center gap-2">{actionsExtra}</div> : null}
      </div>
      <Dialog open={reworkOpen} onOpenChange={setReworkOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('tasks.reports.reworkTitle')}</DialogTitle>
            <DialogDescription>{t('tasks.reports.reworkDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody>
            <Textarea
              value={feedback}
              maxLength={2000}
              onChange={event => setFeedback(event.target.value)}
              placeholder={t('tasks.reports.reworkPlaceholder')}
              className="min-h-28"
            />
          </DialogBody>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setReworkOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button disabled={!feedback.trim() || pending} onClick={() => void submit('rework')}>
              {t('tasks.reports.reworkSubmit')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

/** Loads one report, marks its notification read, and shows it in place. */
export function TaskReportView({
  digitalHumanId,
  reportId,
  mine,
  onOpenRun,
  stickyActions,
  withHeading = false,
  navigation,
  onReviewed,
}: {
  digitalHumanId: string;
  reportId: string;
  mine: boolean;
  onOpenRun?: (runId: string) => void;
  stickyActions?: boolean;
  // Shown beside the heading, such as previous and next buttons.
  navigation?: ReactNode;
  onReviewed?: (action: 'accept' | 'rework') => void;
  // Shows the task name with a link to the task, where nothing else does.
  withHeading?: boolean;
}) {
  const t = useT('digitalHumans');
  const objectHref = useDigitalHumanObjectHref();
  const query = useDigitalHumanTaskReport(digitalHumanId, reportId, mine);
  const { markRead } = useInboxMutations();
  const report = query.data?.data;
  const markReportRead = markRead.mutate;
  useEffect(() => {
    markReportRead({ source_type: 'digital_human_task_report', source_id: reportId });
  }, [reportId, markReportRead]);
  if (query.isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-6 w-40" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }
  return report ? (
    <div className="space-y-4">
      {withHeading ? (
        <div className="flex flex-wrap items-start justify-between gap-2 border-b pb-3">
          <div className="min-w-0">
            <h3 className="text-lg font-semibold leading-7">{report.task_name}</h3>
            {mine ? null : (
              <Link href={objectHref('task', report.task_id, digitalHumanId)} scroll={false} className="text-xs text-primary hover:underline">
                {t('tasks.reports.openTask')}
              </Link>
            )}
          </div>
          {navigation}
        </div>
      ) : null}
      <TaskReportDetail
        digitalHumanId={digitalHumanId}
        report={report}
        mine={mine}
        onOpenRun={onOpenRun}
        stickyActions={stickyActions}
        onReviewed={onReviewed}
      />
    </div>
  ) : (
    <p className="text-sm text-muted-foreground">{t('tasks.reports.loadError')}</p>
  );
}

type FeedFilter = 'all' | 'to_review' | 'attention' | 'accepted';

const feedFilters: Record<FeedFilter, DigitalHumanTaskReportParams> = {
  all: {},
  to_review: { 'review_state[]': ['new'] },
  attention: { 'outcome[]': ['failed', 'not_done', 'partial'] },
  accepted: { 'review_state[]': ['accepted'] },
};

/** The Reports tab: final reports of every task the reader can see. */
export function TaskReportFeed({
  digitalHumanId,
  mine = false,
  taskId,
  selectedId,
  onOpen,
  onItemsChange,
}: {
  digitalHumanId: string;
  mine?: boolean;
  taskId?: string;
  // Highlights the report open beside the list.
  selectedId?: string | null;
  onOpen: (reportId: string) => void;
  // The reports on screen, in order, for moving between them.
  onItemsChange?: (ids: string[]) => void;
}) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const [filter, setFilter] = useState<FeedFilter>('all');
  const [page, setPage] = useState(1);
  const query = useDigitalHumanTaskReports(digitalHumanId, { ...feedFilters[filter], task_id: taskId, page, page_size: 20 }, mine);
  // How many wait in the two filters that ask for action.
  const toReview = useDigitalHumanTaskReports(digitalHumanId, { ...feedFilters.to_review, task_id: taskId, page_size: 1 }, mine);
  const attention = useDigitalHumanTaskReports(digitalHumanId, { ...feedFilters.attention, task_id: taskId, page_size: 1 }, mine);
  const counts: Partial<Record<FeedFilter, number | undefined>> = {
    to_review: toReview.data?.data.total,
    attention: attention.data?.data.total,
  };
  const list = query.data?.data;
  const items = list?.items ?? [];
  const pages = list ? Math.max(1, Math.ceil(list.total / list.page_size)) : 1;
  const idsKey = items.map(item => item.id).join(',');
  useEffect(() => {
    onItemsChange?.(idsKey ? idsKey.split(',') : []);
  }, [idsKey, onItemsChange]);
  return (
    <div className="space-y-3">
      {taskId ? null : (
        <div className="flex flex-wrap gap-2">
          {(Object.keys(feedFilters) as FeedFilter[]).map(key => (
            <Button
              key={key}
              size="sm"
              variant={filter === key ? 'default' : 'outline'}
              onClick={() => {
                setFilter(key);
                setPage(1);
              }}
            >
              {t(`tasks.reports.filter.${key}`)}
              {counts[key] ? <span className="ml-1.5 tabular-nums opacity-70">{counts[key]}</span> : null}
            </Button>
          ))}
        </div>
      )}
      {query.isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      ) : items.length === 0 ? (
        <div className="rounded-lg border border-dashed p-8 text-center">
          <FileText className="mx-auto size-7 text-muted-foreground/50" />
          <p className="mt-2 text-sm font-medium">{t('tasks.reports.empty')}</p>
          <p className="mt-1 text-xs text-muted-foreground">{t('tasks.reports.emptyDescription')}</p>
        </div>
      ) : (
        <ul className="divide-y overflow-hidden rounded-xl border bg-background">
          {items.map(report => {
            const unread = report.review_state === 'new';
            return (
              <li key={report.id}>
                <button
                  type="button"
                  aria-current={selectedId === report.id ? 'true' : undefined}
                  className={cn(
                    'relative w-full px-4 py-3 text-left transition-colors hover:bg-muted/40',
                    selectedId === report.id && 'bg-primary/5 hover:bg-primary/5'
                  )}
                  onClick={() => onOpen(report.id)}
                >
                  {/* Unread reports carry a bar, so they stand out in a long list. */}
                  {unread ? <span className="absolute inset-y-2 left-0 w-0.5 rounded-full bg-primary" aria-hidden /> : null}
                  <div className="flex items-center gap-2">
                    <span className={cn('min-w-0 flex-1 truncate text-sm', unread ? 'font-semibold' : 'font-medium text-foreground/80')}>
                      {taskId ? t(`tasks.reports.outcome.${report.outcome}`) : report.task_name}
                    </span>
                    <span className="shrink-0 text-xs tabular-nums text-muted-foreground" title={new Date(report.created_at).toLocaleString(locale)}>
                      {formatTaskTime(report.created_at, locale)}
                    </span>
                  </div>
                  <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">{plainTextPreview(report.summary)}</p>
                  <div className="mt-2 flex flex-wrap items-center gap-2">
                    <TaskReportOutcomeBadge outcome={report.outcome} />
                    <ReviewStateLabel state={report.review_state} />
                    {report.files?.length ? (
                      <span className="inline-flex items-center gap-0.5 text-xs text-muted-foreground">
                        <Paperclip className="size-3" />
                        {report.files.length}
                      </span>
                    ) : null}
                  </div>
                </button>
              </li>
            );
          })}
        </ul>
      )}
      {pages > 1 ? (
        <div className="flex items-center justify-end gap-2 text-xs text-muted-foreground">
          <Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage(value => value - 1)}>
            {t('tasks.reports.previous')}
          </Button>
          <span>
            {page} / {pages}
          </span>
          <Button size="sm" variant="outline" disabled={page >= pages} onClick={() => setPage(value => value + 1)}>
            {t('tasks.reports.next')}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

/** Loads and reviews reports through the surface's own API (console or WebApp). */
export interface TaskReportClient {
  readFile?: (reportId: string, fileId: string, signal?: AbortSignal) => Promise<Blob>;
  list: (taskId: string) => Promise<ApiResponseData<DigitalHumanTaskReportList>>;
  // The reports of all the reader's tasks, newest first. Without it the panel
  // keeps reports inside each task.
  feed?: (params: DigitalHumanTaskReportParams) => Promise<ApiResponseData<DigitalHumanTaskReportList>>;
  review: (reportId: string, request: ReviewDigitalHumanTaskReportRequest) => Promise<ApiResponseData<DigitalHumanTaskReport>>;
  // Called when the reader opens a report, for example to mark its notice read.
  viewed?: (reportId: string) => void;
  // Where a holder follows a report up in a conversation, when the surface has one.
  followUpHref?: (draft: string, conversationId?: string) => string;
}

/** The reports of one task inside the My tasks panel. */
export function TaskPanelReports({
  client,
  taskId,
  refreshKey = 0,
  onChanged,
}: {
  client: TaskReportClient;
  taskId: string;
  refreshKey?: number;
  onChanged?: () => void;
}) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const [reports, setReports] = useState<DigitalHumanTaskReport[] | null>(null);
  const [openId, setOpenId] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [reload, setReload] = useState(0);
  const clientRef = useRef(client);
  clientRef.current = client;
  const readReportFile = client.readFile;

  useEffect(() => {
    let active = true;
    setReports(null);
    clientRef.current
      .list(taskId)
      .then(response => {
        if (!active) return;
        const items = response.data.items;
        setReports(items);
        setOpenId(current => current ?? (items[0]?.review_state === 'new' ? items[0].id : null));
      })
      .catch(() => {
        if (active) setReports([]);
      });
    return () => {
      active = false;
    };
  }, [taskId, refreshKey, reload]);

  useEffect(() => {
    if (openId) clientRef.current.viewed?.(openId);
  }, [openId]);

  if (reports === null) return <Skeleton className="h-12 w-full" />;
  if (reports.length === 0) return <p className="text-xs text-muted-foreground">{t('tasks.reports.none')}</p>;
  return (
    <ul className="space-y-2">
      {reports.map(report => (
        <li key={report.id} className="rounded-md border">
          <button
            type="button"
            className="w-full px-3 py-2 text-left hover:bg-muted/40"
            aria-expanded={openId === report.id}
            onClick={() => setOpenId(current => (current === report.id ? null : report.id))}
          >
            {/* An open report shows its own outcome and time below. */}
            {openId === report.id ? (
              <span className="flex items-center gap-1 text-xs text-muted-foreground">
                <ChevronDown className="size-3.5" />
                {t('tasks.reports.collapse')}
              </span>
            ) : (
              <>
                <span className="flex flex-wrap items-center gap-2">
                  <TaskReportOutcomeBadge outcome={report.outcome} />
                  <ReviewStateLabel state={report.review_state} />
                  <span className="ml-auto text-[11px] text-muted-foreground">{new Date(report.created_at).toLocaleString(locale)}</span>
                </span>
                <span className="mt-1 line-clamp-2 block text-xs text-muted-foreground">{report.summary}</span>
              </>
            )}
          </button>
          {openId === report.id ? (
            <div className="border-t px-3 py-3">
              <TaskReportDetailView
                report={report}
                pending={pending}
                readFile={readReportFile ? (fileId, signal) => readReportFile(report.id, fileId, signal) : undefined}
                followUpHref={client.followUpHref}
                onReview={async request => {
                  setPending(true);
                  try {
                    await clientRef.current.review(report.id, request);
                  } catch (error) {
                    toast.error(getErrorMessage(error) || t('tasks.reports.loadError'));
                    throw error;
                  } finally {
                    setPending(false);
                  }
                  setReload(value => value + 1);
                  onChanged?.();
                }}
              />
            </div>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

type PanelFeedFilter = 'to_review' | 'all';

/**
 * The Reports tab of the My tasks panel: what the reader's tasks handed back,
 * reports to review first. A report opens in place of the list, with its
 * review actions, through the surface's own API.
 */
export function TaskPanelReportFeed({
  client,
  refreshKey = 0,
  openReportId,
  onOpenHandled,
  onOpenChange,
  onChanged,
}: {
  client: TaskReportClient;
  refreshKey?: number;
  // A report to open, e.g. from a task's "New report"; handled once listed.
  openReportId?: string | null;
  onOpenHandled?: () => void;
  // A report opened or closed, for a panel that widens to read it.
  onOpenChange?: (open: boolean) => void;
  onChanged?: () => void;
}) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const [filter, setFilter] = useState<PanelFeedFilter>('to_review');
  const [page, setPage] = useState(1);
  const [list, setList] = useState<DigitalHumanTaskReportList | null>(null);
  const [error, setError] = useState(false);
  const [open, setOpen] = useState<DigitalHumanTaskReport | null>(null);
  const [pending, setPending] = useState(false);
  const [reload, setReload] = useState(0);
  const clientRef = useRef(client);
  clientRef.current = client;
  const readReportFile = client.readFile;

  useEffect(() => {
    const feed = clientRef.current.feed;
    if (!feed) return;
    let active = true;
    setError(false);
    feed({ ...feedFilters[filter], page, page_size: 20 })
      .then(response => {
        if (active) setList(response.data);
      })
      .catch(() => {
        if (active) {
          setList(null);
          setError(true);
        }
      });
    return () => {
      active = false;
    };
  }, [filter, page, refreshKey, reload]);

  // A named report may already be reviewed, so it is looked for among all.
  useEffect(() => {
    if (!openReportId || !list) return;
    const report = list.items.find(item => item.id === openReportId);
    if (report) {
      setOpen(report);
      onOpenHandled?.();
    } else if (filter !== 'all' || page !== 1) {
      setFilter('all');
      setPage(1);
    } else {
      onOpenHandled?.();
    }
  }, [filter, list, onOpenHandled, openReportId, page]);

  useEffect(() => {
    if (open) clientRef.current.viewed?.(open.id);
  }, [open]);
  const isOpen = open !== null;
  useEffect(() => {
    onOpenChange?.(isOpen);
  }, [isOpen, onOpenChange]);

  if (open) {
    return (
      <div className="space-y-3">
        <Button variant="ghost" size="sm" className="-ml-2 h-7 px-2 text-xs" onClick={() => setOpen(null)}>
          <ArrowLeft className="size-3.5" />
          {t('tasks.panel.backToReports')}
        </Button>
        <div>
          <h3 className="text-sm font-semibold leading-6">{open.task_name}</h3>
          <p className="text-xs text-muted-foreground">{new Date(open.created_at).toLocaleString(locale)}</p>
        </div>
        <TaskReportDetailView
          report={open}
          pending={pending}
          readFile={readReportFile ? (fileId, signal) => readReportFile(open.id, fileId, signal) : undefined}
          followUpHref={client.followUpHref}
          onReview={async request => {
            setPending(true);
            try {
              const response = await clientRef.current.review(open.id, request);
              setOpen(response.data);
            } catch (reviewError) {
              toast.error(getErrorMessage(reviewError) || t('tasks.reports.loadError'));
              throw reviewError;
            } finally {
              setPending(false);
            }
            setReload(value => value + 1);
            onChanged?.();
          }}
        />
      </div>
    );
  }

  const items = list?.items ?? [];
  const pages = list ? Math.max(1, Math.ceil(list.total / list.page_size)) : 1;
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-1">
        {(['to_review', 'all'] as const).map(key => (
          <Button
            key={key}
            type="button"
            size="sm"
            variant={filter === key ? 'secondary' : 'ghost'}
            className="h-7 rounded-full px-3 text-xs"
            aria-pressed={filter === key}
            onClick={() => {
              setFilter(key);
              setPage(1);
            }}
          >
            {t(`tasks.reports.filter.${key}`)}
          </Button>
        ))}
      </div>
      {error ? (
        <div className="rounded-lg border border-dashed p-6 text-center text-xs text-muted-foreground">
          {t('tasks.reports.loadError')}
          <Button variant="link" size="sm" className="h-auto px-1 text-xs" onClick={() => setReload(value => value + 1)}>
            {t('tasks.retry')}
          </Button>
        </div>
      ) : list === null ? (
        <div className="space-y-2">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      ) : items.length === 0 ? (
        <div className="flex min-h-28 flex-col items-center justify-center gap-2 px-3 py-5 text-center">
          <FileText className="size-6 text-muted-foreground/60" />
          <p className="text-xs text-muted-foreground">
            {filter === 'to_review' ? t('tasks.panel.noReportsToReview') : t('tasks.reports.none')}
          </p>
        </div>
      ) : (
        <ul className="space-y-1">
          {items.map(report => {
            const unread = report.review_state === 'new';
            return (
              <li key={report.id}>
                <button
                  type="button"
                  className="w-full rounded-lg border border-transparent px-3 py-2.5 text-left transition-colors hover:border-primary/30 hover:bg-muted/60"
                  onClick={() => setOpen(report)}
                >
                  <span className="flex min-w-0 items-center gap-2">
                    {unread ? <span className="size-1.5 shrink-0 rounded-full bg-primary" aria-label={t('tasks.reports.review.new')} /> : null}
                    <span className={cn('min-w-0 flex-1 truncate text-sm', unread ? 'font-semibold' : 'font-medium text-foreground/80')}>
                      {report.task_name}
                    </span>
                    <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{formatTaskTime(report.created_at, locale)}</span>
                  </span>
                  <span className="mt-1 line-clamp-2 block text-xs leading-5 text-muted-foreground">{plainTextPreview(report.summary)}</span>
                  <span className="mt-1.5 flex flex-wrap items-center gap-2">
                    <TaskReportOutcomeBadge outcome={report.outcome} />
                    {unread ? null : <ReviewStateLabel state={report.review_state} />}
                    {report.files?.length ? (
                      <span className="inline-flex items-center gap-0.5 text-xs text-muted-foreground">
                        <Paperclip className="size-3" />
                        {report.files.length}
                      </span>
                    ) : null}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
      {pages > 1 ? (
        <div className="flex items-center justify-end gap-2 text-xs text-muted-foreground">
          <Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage(value => value - 1)}>
            {t('tasks.reports.previous')}
          </Button>
          <span>
            {page} / {pages}
          </span>
          <Button size="sm" variant="outline" disabled={page >= pages} onClick={() => setPage(value => value + 1)}>
            {t('tasks.reports.next')}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

const problemOutcomes: ReadonlySet<DigitalHumanTaskReportOutcome> = new Set(['partial', 'not_done', 'failed']);

/** The console chat, opened with a follow-up draft, in the task's conversation when known. */
export function digitalHumanChatFollowUpHref(digitalHumanId: string, draft: string, conversationId?: string) {
  const params = new URLSearchParams({ draft });
  if (conversationId) params.set('conversation', conversationId);
  return `/console/digital-humans/${digitalHumanId}/chat?${params.toString()}`;
}

/**
 * A report that did not fully succeed, for the reader who holds its task:
 * supplement what was missing and run again, or take it to the conversation
 * the task came from. A channel task is answered by reply in its chat.
 */
function TaskReportFollowUp({
  report,
  pending,
  followUpHref,
  onReview,
}: {
  report: DigitalHumanTaskReport;
  pending: boolean;
  followUpHref?: (draft: string, conversationId?: string) => string;
  onReview: (request: { action: 'accept' } | { action: 'rework'; feedback: string }) => Promise<void>;
}) {
  const t = useT('digitalHumans');
  const [supplement, setSupplement] = useState('');
  if (!problemOutcomes.has(report.outcome) || report.review_state === 'rework_requested') return null;
  const channel = report.origin?.kind === 'channel';
  // Whoever holds the task handles it; a channel task is held by its chat.
  if (!report.requested_by_reader && !channel) return null;
  const problem = (report.issues?.trim() || report.summary).trim();
  const draft = t('tasks.reports.followUp.draft', { name: report.task_name, problem });
  const conversationId = report.origin?.kind === 'conversation' ? report.origin.conversation_id : undefined;
  return (
    <section className="space-y-3 rounded-lg border border-warning/30 bg-warning/5 p-3">
      <div>
        <h4 className="text-sm font-semibold">{t('tasks.reports.followUp.title')}</h4>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">
          {channel
            ? t(report.origin?.conversation_type === 'group' ? 'tasks.reports.followUp.channelGroup' : 'tasks.reports.followUp.channelDirect')
            : t('tasks.reports.followUp.description')}
        </p>
      </div>
      {report.requested_by_reader && report.can_review ? (
        <>
          <Textarea
            value={supplement}
            maxLength={2000}
            onChange={event => setSupplement(event.target.value)}
            placeholder={t('tasks.reports.followUp.placeholder')}
            className="min-h-20 bg-background"
          />
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              disabled={!supplement.trim() || pending}
              onClick={async () => {
                try {
                  await onReview({ action: 'rework', feedback: supplement.trim() });
                } catch {
                  return;
                }
                toast.success(t('tasks.reports.followUp.submitted'));
                setSupplement('');
              }}
            >
              <RotateCcw className="mr-1 size-4" />
              {t('tasks.reports.followUp.submit')}
            </Button>
            {followUpHref && !channel ? (
              <Button size="sm" variant="ghost" asChild>
                <Link href={followUpHref(draft, conversationId)}>
                  <MessageSquare className="mr-1 size-4" />
                  {conversationId ? t('tasks.reports.followUp.openConversation') : t('tasks.reports.followUp.newConversation')}
                </Link>
              </Button>
            ) : null}
          </div>
        </>
      ) : null}
    </section>
  );
}
