import React from 'react';
import { useT } from '@/i18n';
import type { WorkflowNodeData } from '../../store';
import { normalizeCreateScheduledTaskNodeData } from './config';
import { scheduledTaskActionRegistry } from './registry';

type CreateScheduledTaskData = Extract<WorkflowNodeData, { type: 'create-scheduled-task' }>;

interface CreateScheduledTaskContentProps {
  nodeId: string;
  data: CreateScheduledTaskData;
}

const CreateScheduledTaskContent: React.FC<CreateScheduledTaskContentProps> = ({ data }) => {
  const t = useT('nodes');
  const normalized = normalizeCreateScheduledTaskNodeData(data);
  const actions = normalized.task.actions;
  const enabledActions = actions.filter(action => action.enabled);
  const previewActions = enabledActions.length > 0 ? enabledActions : actions;
  const firstPreviewAction = previewActions[0];

  const scheduleLabel =
    normalized.task.schedule.type === 'cron'
      ? t('createScheduledTask.scheduleTypeCron')
      : t('createScheduledTask.scheduleTypeOnce');
  const scheduleValue =
    normalized.task.schedule.type === 'cron'
      ? normalized.task.schedule.cron.expr || t('createScheduledTask.preview.notConfigured')
      : normalized.task.schedule.once.run_at || t('createScheduledTask.preview.notConfigured');
  const actionSummary = t('createScheduledTask.preview.actionsSummary', {
    total: actions.length,
    enabled: enabledActions.length,
  });
  const firstActionMeta = firstPreviewAction
    ? scheduledTaskActionRegistry[firstPreviewAction.action_type]
    : null;
  const operationTypeLabel = firstActionMeta
    ? t(firstActionMeta.labelKey as never)
    : t('createScheduledTask.empty.unsupportedActionTitle');

  return (
    <div className="space-y-2 text-xs text-muted-foreground">
      <div className="flex items-center gap-2">
        <span className="rounded-full border border-border bg-muted px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.08em] text-foreground">
          {scheduleLabel}
        </span>
        <span className="truncate" title={scheduleValue}>
          {scheduleValue}
        </span>
      </div>

      <div className="flex items-center gap-2">
        <span className="rounded-full border border-border bg-muted px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.08em] text-foreground">
          {actionSummary}
        </span>
        <span className="truncate" title={operationTypeLabel}>
          {operationTypeLabel}
        </span>
      </div>
    </div>
  );
};

export default CreateScheduledTaskContent;
