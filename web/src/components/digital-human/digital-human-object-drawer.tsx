'use client';

import { useCallback, useState } from 'react';
import Link from 'next/link';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { ExternalLink } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { ArtifactDetail } from '@/components/digital-human/digital-human-artifacts';
import { DigitalHumanRunDetailDrawer } from '@/components/digital-human/digital-human-run-detail-drawer';
import { TaskDetailSheet, TaskEditorDialog } from '@/components/digital-human/digital-human-tasks';
import { useDigitalHumanMutations } from '@/hooks/digital-human/use-digital-humans';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import type { DigitalHumanTask } from '@/services/types/digital-human';
import {
  DIGITAL_HUMAN_VIEW_DH_PARAM,
  DIGITAL_HUMAN_VIEW_PARAM,
  parseDigitalHumanView,
  pathDigitalHuman,
} from '@/components/digital-human/digital-human-object-link';

/**
 * The console-wide drawer for runs, tasks, and artifacts, mounted once in the
 * console layout beside the report drawer. Lists and the workspace link here
 * instead of to another page, so looking at or handling an item keeps the
 * reader where they were.
 */
export function DigitalHumanObjectDrawer() {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const router = useRouter();
  const view = parseDigitalHumanView(searchParams?.get(DIGITAL_HUMAN_VIEW_PARAM) ?? null);
  const digitalHumanId = searchParams?.get(DIGITAL_HUMAN_VIEW_DH_PARAM) ?? pathDigitalHuman(pathname);

  const close = useCallback(() => {
    const params = new URLSearchParams(searchParams?.toString());
    params.delete(DIGITAL_HUMAN_VIEW_PARAM);
    params.delete(DIGITAL_HUMAN_VIEW_DH_PARAM);
    const next = params.toString();
    router.replace(next ? `${pathname}?${next}` : (pathname ?? '/console'), { scroll: false });
  }, [pathname, router, searchParams]);

  if (!view || !digitalHumanId) return null;
  if (view.kind === 'run') return <RunView digitalHumanId={digitalHumanId} runId={view.id} onClose={close} />;
  if (view.kind === 'task') return <TaskView digitalHumanId={digitalHumanId} taskId={view.id} onClose={close} />;
  return <ArtifactView digitalHumanId={digitalHumanId} artifactId={view.id} onClose={close} />;
}

function RunView({ digitalHumanId, runId, onClose }: { digitalHumanId: string; runId: string; onClose: () => void }) {
  const t = useT('digitalHumans');
  const mutations = useDigitalHumanMutations(digitalHumanId);
  return (
    <DigitalHumanRunDetailDrawer
      digitalHumanId={digitalHumanId}
      runId={runId}
      open
      onOpenChange={open => !open && onClose()}
      onCancel={async id => {
        await mutations.cancelRun.mutateAsync({ digitalHumanId, runId: id });
        toast.success(t('runs.cancelRequested'));
      }}
      cancelPending={mutations.cancelRun.isPending}
    />
  );
}

function TaskView({ digitalHumanId, taskId, onClose }: { digitalHumanId: string; taskId: string; onClose: () => void }) {
  const permissions = useAccountPermissions();
  const [editorTask, setEditorTask] = useState<DigitalHumanTask | null | undefined>(undefined);
  return (
    <>
      <TaskDetailSheet
        id={digitalHumanId}
        taskId={taskId}
        open
        canUpdate={permissions.hasPermission('digital_human.update')}
        canRun={permissions.hasPermission('digital_human.run')}
        onEdit={task => setEditorTask(task)}
        onOpenChange={open => !open && onClose()}
      />
      <TaskEditorDialog
        id={digitalHumanId}
        task={editorTask}
        open={editorTask !== undefined}
        onOpenChange={open => {
          if (!open) setEditorTask(undefined);
        }}
      />
    </>
  );
}

function ArtifactView({
  digitalHumanId,
  artifactId,
  onClose,
}: {
  digitalHumanId: string;
  artifactId: string;
  onClose: () => void;
}) {
  const t = useT('digitalHumans');
  return (
    <Sheet open onOpenChange={open => !open && onClose()}>
      <SheetContent className="w-full overflow-y-auto p-4 sm:max-w-xl">
        <SheetHeader className="p-0 pr-8">
          <SheetTitle className="sr-only">{t('artifacts.title')}</SheetTitle>
          <Button variant="link" size="sm" className="h-auto w-fit p-0 text-xs" asChild>
            <Link href={`/console/digital-humans/${digitalHumanId}/artifacts?selected=${artifactId}`}>
              <ExternalLink className="mr-1 size-3.5" />
              {t('artifacts.openInPage')}
            </Link>
          </Button>
        </SheetHeader>
        <ArtifactDetail key={artifactId} digitalHumanId={digitalHumanId} artifactId={artifactId} />
      </SheetContent>
    </Sheet>
  );
}
