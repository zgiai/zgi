'use client';

import * as React from 'react';
import { AlertCircle } from 'lucide-react';
import { useT } from '@/i18n';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  useArchiveAutomationTask,
  useAutomationTaskCounts,
  useAutomationTask,
  useAutomationTasks,
  useCreateAutomationTask,
  usePauseAutomationTask,
  useResumeAutomationTask,
  useRunAutomationTask,
  useUpdateAutomationTask,
} from '@/hooks/automation/use-automation';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useIsMobile } from '@/hooks/use-mobile';
import { useCurrentWorkspace } from '@/store/workspace-store';
import type { AutomationTaskDetailData } from '@/services/types/automation';
import { TaskDetailPanel } from './task-detail-panel';
import { TaskEditorPanel } from './task-editor-panel';
import { TaskListPane } from './task-list-pane';
import { TaskPanelHost } from './task-panel-host';
import { TaskWorkspaceRequiredState } from './task-workspace-required-state';
import { useTaskRouteState } from './use-task-route-state';

function TaskPanelErrorState({ onClose }: { onClose: () => void }) {
  const t = useT('automation');
  const tCommon = useT('common');

  return (
    <div className="flex h-full min-h-0 w-full flex-col">
      <div className="border-b border-border/70 px-5 py-4">
        <h2 className="text-lg font-semibold text-foreground">{t('detail.details')}</h2>
      </div>
      <div className="p-5">
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>{tCommon('error')}</AlertTitle>
          <AlertDescription>{tCommon('errorBoundary.unexpectedError')}</AlertDescription>
        </Alert>
      </div>
      <div className="mt-auto border-t border-border/70 px-5 py-4">
        <Button variant="ghost" size="sm" onClick={onClose}>
          {t('closePanel')}
        </Button>
      </div>
    </div>
  );
}

interface TaskWorkbenchContentProps {
  workspaceId: string;
  canManageTasks: boolean;
  isMobile: boolean;
}

/**
 * @component TaskWorkbench
 * @category Feature
 * @status Stable
 * @description Workspace automation workbench with list, detail, create, and edit flows.
 * @usage Render at `/console/work/task` as the main scheduled-task management center.
 */
export function TaskWorkbench() {
  const currentWorkspace = useCurrentWorkspace();
  const { isWorkspaceManager } = useAccountPermissions();
  const isMobile = useIsMobile();
  const workspaceId = currentWorkspace?.id;
  const canManageTasks = Boolean(workspaceId) && isWorkspaceManager();

  if (!workspaceId) {
    return <TaskWorkspaceRequiredState />;
  }

  return (
    <TaskWorkbenchContent
      workspaceId={workspaceId}
      canManageTasks={canManageTasks}
      isMobile={isMobile}
    />
  );
}

function TaskWorkbenchContent({
  workspaceId,
  canManageTasks,
  isMobile,
}: TaskWorkbenchContentProps) {
  const t = useT('automation');
  const tCommon = useT('common');
  const route = useTaskRouteState();
  const [archiveTarget, setArchiveTarget] = React.useState<{ id: string; name: string } | null>(
    null
  );
  const list = useAutomationTasks({
    workspace_id: workspaceId,
    statuses: route.statusQuery || undefined,
    page: route.page,
    limit: 20,
  });
  const taskCounts = useAutomationTaskCounts(workspaceId);
  const detail = useAutomationTask(
    route.taskId ?? undefined,
    {
      workspace_id: workspaceId,
    },
    route.panelView === 'view' || route.panelView === 'edit'
  );
  const { createAutomationTask, isCreating } = useCreateAutomationTask();
  const { updateAutomationTask, isUpdating } = useUpdateAutomationTask();
  const { runAutomationTask, isRunning } = useRunAutomationTask();
  const { pauseAutomationTask, isPausing } = usePauseAutomationTask();
  const { resumeAutomationTask, isResuming } = useResumeAutomationTask();
  const { archiveAutomationTask, isArchiving } = useArchiveAutomationTask();
  const workbenchRef = React.useRef<HTMLDivElement | null>(null);

  const actionBusy = isRunning || isPausing || isResuming || isArchiving;
  const totalPages = Math.max(1, Math.ceil(list.total / list.limit));

  React.useEffect(() => {
    if (!list.isLoading && route.page > totalPages) {
      route.setPage(totalPages);
    }
  }, [list.isLoading, route, totalPages]);

  React.useEffect(() => {
    if (isMobile || !route.panelOpen) {
      return;
    }

    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target as HTMLElement | null;

      if (!target || !workbenchRef.current?.contains(target)) {
        return;
      }

      if (
        target.closest('[data-task-panel="true"]') ||
        target.closest('[data-task-card="true"]') ||
        target.closest('[data-radix-popper-content-wrapper]')
      ) {
        return;
      }

      route.closePanel();
    };

    document.addEventListener('pointerdown', handlePointerDown);

    return () => {
      document.removeEventListener('pointerdown', handlePointerDown);
    };
  }, [isMobile, route]);

  const handleCreateSubmitted = React.useCallback(
    (taskDetail: AutomationTaskDetailData) => {
      route.selectTask(taskDetail.task.id, 'overview');
    },
    [route]
  );

  const handleEditSubmitted = React.useCallback(
    (taskDetail: AutomationTaskDetailData) => {
      route.selectTask(taskDetail.task.id, 'overview');
    },
    [route]
  );

  const handleRunTask = React.useCallback(
    async (taskId: string) => {
      await runAutomationTask(taskId, { workspace_id: workspaceId });
      route.selectTask(taskId, 'runs');
    },
    [route, runAutomationTask, workspaceId]
  );

  const handlePauseTask = React.useCallback(
    async (taskId: string) => {
      await pauseAutomationTask(taskId, { workspace_id: workspaceId });
    },
    [pauseAutomationTask, workspaceId]
  );

  const handleResumeTask = React.useCallback(
    async (taskId: string) => {
      await resumeAutomationTask(taskId, { workspace_id: workspaceId });
    },
    [resumeAutomationTask, workspaceId]
  );

  const handleArchiveTask = React.useCallback(async () => {
    if (!archiveTarget) {
      return;
    }

    await archiveAutomationTask(archiveTarget.id, { workspace_id: workspaceId });
    setArchiveTarget(null);
  }, [archiveAutomationTask, archiveTarget, workspaceId]);

  const handleRefreshTask = React.useCallback(() => {
    void detail.refetch();
    void list.refetch();
  }, [detail, list]);

  const panelContent = React.useMemo(() => {
    if (route.panelView === 'create') {
      return (
        <TaskEditorPanel
          mode="create"
          workspaceId={workspaceId}
          canManage={canManageTasks}
          isSubmitting={isCreating}
          onCancel={route.closePanel}
          onSubmitted={handleCreateSubmitted}
          onCreate={createAutomationTask}
          onUpdate={updateAutomationTask}
        />
      );
    }

    if (route.panelView === 'edit') {
      return (
        <TaskEditorPanel
          mode="edit"
          workspaceId={workspaceId}
          canManage={canManageTasks}
          isSubmitting={isUpdating}
          taskDetail={detail.taskDetail}
          onCancel={() => (route.taskId ? route.selectTask(route.taskId) : route.closePanel())}
          onSubmitted={handleEditSubmitted}
          onCreate={createAutomationTask}
          onUpdate={updateAutomationTask}
        />
      );
    }

    if (route.panelView === 'view') {
      if (detail.error) {
        return <TaskPanelErrorState onClose={route.closePanel} />;
      }

      return (
        <TaskDetailPanel
          taskDetail={detail.taskDetail}
          workspaceId={workspaceId}
          tab={route.tab}
          selectedStatuses={route.selectedStatuses}
          isLoading={detail.isLoading}
          canManage={canManageTasks}
          actionBusy={actionBusy}
          onClose={route.closePanel}
          onEdit={() => {
            if (route.taskId) {
              route.openEdit(route.taskId);
            }
          }}
          onRunTask={() => {
            if (route.taskId) {
              void handleRunTask(route.taskId);
            }
          }}
          onPauseTask={() => {
            if (route.taskId) {
              void handlePauseTask(route.taskId);
            }
          }}
          onResumeTask={() => {
            if (route.taskId) {
              void handleResumeTask(route.taskId);
            }
          }}
          onArchiveTask={() => {
            if (detail.task) {
              setArchiveTarget({ id: detail.task.id, name: detail.task.name });
            }
          }}
          onTabChange={route.setTab}
          onRefreshTask={handleRefreshTask}
        />
      );
    }

    return null;
  }, [
    actionBusy,
    canManageTasks,
    createAutomationTask,
    detail.error,
    detail.isLoading,
    detail.task,
    detail.taskDetail,
    handleCreateSubmitted,
    handleEditSubmitted,
    handlePauseTask,
    handleResumeTask,
    handleRunTask,
    handleRefreshTask,
    isCreating,
    isUpdating,
    route,
    updateAutomationTask,
    workspaceId,
  ]);

  return (
    <>
      <div ref={workbenchRef} className="relative flex h-full min-h-0 bg-background">
        <section className="min-w-0 grow">
          <TaskListPane
            items={list.items}
            total={list.total}
            page={list.page}
            pageSize={list.limit}
            isLoading={list.isLoading}
            isFetching={list.isFetching || taskCounts.isFetching}
            counts={taskCounts.counts}
            selectedTaskId={route.taskId}
            filterKey={route.filterKey}
            panelOpen={route.panelOpen}
            canManage={canManageTasks}
            onOpenCreate={route.openCreate}
            onFilterChange={route.setStatusFilter}
            onPageChange={route.setPage}
            onSelectTask={route.selectTask}
          />
        </section>

        <TaskPanelHost
          open={route.panelOpen}
          isMobile={isMobile}
          onOpenChange={open => {
            if (!open) {
              route.closePanel();
            }
          }}
        >
          {panelContent}
        </TaskPanelHost>
      </div>

      <ConfirmDialog
        open={Boolean(archiveTarget)}
        onOpenChange={open => {
          if (!open) {
            setArchiveTarget(null);
          }
        }}
        title={t('operations.archiveTitle')}
        description={
          archiveTarget
            ? `${archiveTarget.name} - ${t('operations.archiveDescription')}`
            : t('operations.archiveDescription')
        }
        confirmText={t('operations.archive')}
        cancelText={tCommon('cancel')}
        onConfirm={() => {
          void handleArchiveTask();
        }}
        loading={isArchiving}
      />
    </>
  );
}
