'use client';

import { useEffect, useState } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { ArrowLeft, FileText } from 'lucide-react';
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

  const open = (reportId: string) => {
    if (!wide) {
      openInDrawer(reportId);
      return;
    }
    const params = new URLSearchParams(searchParams.toString());
    params.set('selected', reportId);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  };

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
        <TaskReportFeed digitalHumanId={id} selectedId={wide ? selected : null} onOpen={open} />
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
                onOpenRun={canViewLogs ? setRunView : undefined}
              />
            )}
          </section>
        ) : null}
      </div>
    </main>
  );
}
