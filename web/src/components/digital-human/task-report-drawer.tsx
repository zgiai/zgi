'use client';

import { useCallback, useEffect, useState } from 'react';
import Link from 'next/link';
import { useDigitalHumanObjectHref } from '@/components/digital-human/digital-human-object-link';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { ArrowLeft, ArrowRight, ExternalLink } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import { TaskReportDetail } from '@/components/digital-human/digital-human-task-reports';
import { DigitalHumanRunTimeline } from '@/components/digital-human/digital-human-run-timeline';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useDigitalHumanTaskReport } from '@/hooks/digital-human/use-digital-human-tasks';
import { useInboxList, useInboxMutations } from '@/hooks/use-inbox';
import { useT } from '@/i18n';

// A report opens over whatever console page the reader is on. The URL keeps
// it, so a refresh or a shared link reopens it: `report` names the report and
// `report_dh` its DigitalHuman, which pages under a DigitalHuman may omit.
const REPORT_PARAM = 'report';
const REPORT_DH_PARAM = 'report_dh';
const digitalHumanPath = /^\/console\/digital-humans\/([0-9a-f-]{36})(?:\/|$)/i;

function pathDigitalHuman(pathname: string | null) {
  return pathname?.match(digitalHumanPath)?.[1] ?? null;
}

/** Opens a task report in the console's report drawer without leaving the page. */
export function useOpenTaskReport() {
  const router = useRouter();
  const pathname = usePathname();
  return useCallback(
    (reportId: string, digitalHumanId?: string) => {
      // Read the query when opening, so callers such as the header need no
      // Suspense boundary for useSearchParams.
      const params = new URLSearchParams(window.location.search);
      params.set(REPORT_PARAM, reportId);
      if (digitalHumanId && digitalHumanId !== pathDigitalHuman(pathname)) params.set(REPORT_DH_PARAM, digitalHumanId);
      else params.delete(REPORT_DH_PARAM);
      router.push(`${pathname}?${params.toString()}`, { scroll: false });
    },
    [pathname, router]
  );
}

/**
 * Reads a report the way the reader can see it: through the task center, or,
 * for a member's own private task, through their personal tasks.
 */
function useReaderTaskReport(digitalHumanId: string, reportId: string | null) {
  const center = useDigitalHumanTaskReport(digitalHumanId, reportId, false, { retry: false });
  const mine = useDigitalHumanTaskReport(digitalHumanId, center.isError ? reportId : null, true, { retry: false });
  const viaMine = center.isError;
  const active = viaMine ? mine : center;
  return { report: active.data?.data, mine: viaMine, isLoading: center.isLoading || (viaMine && mine.isLoading), isError: viaMine && mine.isError };
}

/** The console-wide report drawer, mounted once in the console layout. */
export function TaskReportDrawer() {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const router = useRouter();
  const reportId = searchParams?.get(REPORT_PARAM) ?? null;
  const digitalHumanId = searchParams?.get(REPORT_DH_PARAM) ?? pathDigitalHuman(pathname);
  const open = Boolean(reportId && digitalHumanId);

  const close = () => {
    const params = new URLSearchParams(searchParams?.toString());
    params.delete(REPORT_PARAM);
    params.delete(REPORT_DH_PARAM);
    const next = params.toString();
    router.replace(next ? `${pathname}?${next}` : (pathname ?? '/console'), { scroll: false });
  };

  return (
    <Sheet open={open} onOpenChange={value => !value && close()}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
        {open ? <TaskReportDrawerBody key={reportId} digitalHumanId={digitalHumanId ?? ''} reportId={reportId ?? ''} /> : null}
      </SheetContent>
    </Sheet>
  );
}

function TaskReportDrawerBody({ digitalHumanId, reportId }: { digitalHumanId: string; reportId: string }) {
  const t = useT('digitalHumans');
  const objectHref = useDigitalHumanObjectHref();
  const { report, mine, isLoading, isError } = useReaderTaskReport(digitalHumanId, reportId);
  // The run behind the report opens in place of it, for readers of run logs.
  const [runView, setRunView] = useState<string | null>(null);
  const canViewLogs = useAccountPermissions().hasPermission('digital_human.logs.view');
  const { markRead } = useInboxMutations();
  const markReportRead = markRead.mutate;
  useEffect(() => {
    markReportRead({ source_type: 'digital_human_task_report', source_id: reportId });
  }, [reportId, markReportRead]);
  // Unread reports form a queue, so a backlog can be worked through here.
  const openReport = useOpenTaskReport();
  const unread = useInboxList({ unread_only: true });
  const queue = (unread.data?.data.items ?? []).filter(
    item => item.source_type === 'digital_human_task_report' && item.source_id !== reportId && typeof item.context.digital_human_id === 'string'
  );
  const next = queue[0];
  // A member's own private task lives in their conversation panel; every
  // other task opens in the task center.
  const taskHref = report
    ? mine
      ? `/console/digital-humans/${digitalHumanId}/chat?task=${report.task_id}`
      : objectHref('task', report.task_id, digitalHumanId)
    : null;
  return (
    <>
      <SheetHeader>
        <SheetTitle>{report?.task_name || t('tasks.reports.title')}</SheetTitle>
        <SheetDescription>{t('tasks.reports.sheetDescription')}</SheetDescription>
        {taskHref ? (
          <Button variant="link" size="sm" className="h-auto w-fit p-0 text-xs" asChild>
            <Link href={taskHref} scroll={false}>
              <ExternalLink className="mr-1 size-3.5" />
              {t('tasks.reports.openTask')}
            </Link>
          </Button>
        ) : null}
      </SheetHeader>
      <div className="px-4 pb-6">
        {isLoading ? (
          <div className="space-y-3">
            <Skeleton className="h-6 w-40" />
            <Skeleton className="h-24 w-full" />
          </div>
        ) : report && runView ? (
          <div className="space-y-4">
            <Button variant="ghost" size="sm" className="-ml-2" onClick={() => setRunView(null)}>
              <ArrowLeft className="size-4" />
              {t('tasks.runTrace.backToReport')}
            </Button>
            <DigitalHumanRunTimeline digitalHumanId={digitalHumanId} runId={runView} />
          </div>
        ) : report ? (
          <TaskReportDetail
            digitalHumanId={digitalHumanId}
            report={report}
            mine={mine}
            onOpenRun={canViewLogs ? setRunView : undefined}
            stickyActions
            actionsExtra={
              next ? (
                <>
                  <span className="text-xs text-muted-foreground">{t('tasks.reports.unreadLeft', { count: queue.length })}</span>
                  <Button size="sm" variant="outline" onClick={() => openReport(next.source_id, String(next.context.digital_human_id))}>
                    {t('tasks.reports.nextUnread')}
                    <ArrowRight className="size-4" />
                  </Button>
                </>
              ) : null
            }
          />
        ) : isError ? (
          <p className="text-sm text-muted-foreground">{t('tasks.reports.loadError')}</p>
        ) : null}
      </div>
    </>
  );
}
