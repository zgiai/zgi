'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { ArrowLeft, ChevronLeft, ChevronRight, FileText } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { TaskReportFeed, TaskReportView } from '@/components/digital-human/digital-human-task-reports';
import { DigitalHumanRunTimeline } from '@/components/digital-human/digital-human-run-timeline';
import { useOpenTaskReport } from '@/components/digital-human/task-report-drawer';
import { useMediaQuery } from '@/hooks/use-media-query';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';

/**
 * The Reports page: every report of the tasks the reader can see. On wide
 * screens the list and the open report sit side by side; on narrow ones a
 * report opens in the report drawer.
 */
export function DigitalHumanReports({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const selected = searchParams.get('selected');
  const wide = useMediaQuery('(min-width: 1024px)');
  const openInDrawer = useOpenTaskReport();
  const canViewLogs = useAccountPermissions().hasPermission('digital_human.logs.view');
  const [runView, setRunView] = useState<string | null>(null);
  useEffect(() => setRunView(null), [selected]);
  // The reports in the list, in order; kept from before a review changes it.
  const [ids, setIds] = useState<string[]>([]);
  const idsRef = useRef<string[]>([]);
  const onItemsChange = useCallback((next: string[]) => {
    idsRef.current = next;
    setIds(next);
  }, []);
  const index = selected ? ids.indexOf(selected) : -1;

  const open = (reportId: string) => {
    if (!wide) {
      openInDrawer(reportId);
      return;
    }
    const params = new URLSearchParams(searchParams.toString());
    params.set('selected', reportId);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  };
  // After a review, the next report in the list opens so reading goes on.
  const openNextAfter = (reportId: string) => {
    const list = idsRef.current;
    const at = list.indexOf(reportId);
    const next = at >= 0 ? list[at + 1] : undefined;
    if (next) open(next);
  };
  const navigation = (
    <div className="flex items-center gap-1">
      <span className="mr-1 text-xs tabular-nums text-muted-foreground">{index >= 0 ? `${index + 1} / ${ids.length}` : ''}</span>
      <Button
        size="sm"
        variant="outline"
        className="size-8 p-0"
        disabled={index <= 0}
        aria-label={t('tasks.reports.previousReport')}
        title={t('tasks.reports.previousReport')}
        onClick={() => open(ids[index - 1])}
      >
        <ChevronLeft className="size-4" />
      </Button>
      <Button
        size="sm"
        variant="outline"
        className="size-8 p-0"
        disabled={index < 0 || index >= ids.length - 1}
        aria-label={t('tasks.reports.nextReport')}
        title={t('tasks.reports.nextReport')}
        onClick={() => open(ids[index + 1])}
      >
        <ChevronRight className="size-4" />
      </Button>
    </div>
  );

  return (
    <main className="mx-auto flex min-h-full w-full max-w-[1600px] flex-col gap-5 px-4 py-6 md:px-8 md:py-8">
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <FileText className="size-5 text-primary" />
          <h1 className="text-2xl font-semibold tracking-tight">{t('tasks.reports.pageTitle')}</h1>
        </div>
        <p className="max-w-3xl text-sm text-muted-foreground">{t('tasks.reports.pageDescription')}</p>
      </div>
      <div className="grid items-start gap-4 lg:grid-cols-[minmax(320px,420px)_minmax(0,1fr)]">
        <TaskReportFeed digitalHumanId={id} selectedId={wide ? selected : null} onOpen={open} onItemsChange={onItemsChange} />
        {wide ? (
          <section className="sticky top-4 max-h-[calc(100vh-12rem)] overflow-y-auto rounded-xl border bg-background px-4 pt-4">
            {!selected ? (
              <div className="flex min-h-60 flex-col items-center justify-center gap-2 pb-4 text-center">
                <FileText className="size-7 text-muted-foreground/50" />
                <p className="text-sm text-muted-foreground">{t('tasks.reports.selectHint')}</p>
              </div>
            ) : runView ? (
              <div className="space-y-4 pb-4">
                <Button variant="ghost" size="sm" className="-ml-2" onClick={() => setRunView(null)}>
                  <ArrowLeft className="size-4" />
                  {t('tasks.runTrace.backToReport')}
                </Button>
                <DigitalHumanRunTimeline digitalHumanId={id} runId={runView} />
              </div>
            ) : (
              <TaskReportView
                key={selected}
                digitalHumanId={id}
                reportId={selected}
                mine={false}
                withHeading
                stickyActions
                navigation={navigation}
                onReviewed={() => openNextAfter(selected)}
                onOpenRun={canViewLogs ? setRunView : undefined}
              />
            )}
          </section>
        ) : null}
      </div>
    </main>
  );
}
