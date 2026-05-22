'use client';

import React from 'react';
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
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { getNotificationSMSTemplates } from '@/lib/features/notification-sms';
import { useAuthStore } from '@/store/auth-store';
import OutputVariablesView from '../../../common/output-variables-view';
import { useNodeData, useNodeDataUpdate, useNodeOutputVariables } from '../../../hooks';
import { WorkflowValueEditor } from '../../../ui';
import {
  cloneCreateScheduledTaskActionData,
  createDefaultScheduledTaskActionDraft,
  getCreateScheduledTaskActionValidationErrors,
  hasCreateScheduledTaskActionValidationErrors,
  normalizeCreateScheduledTaskNodeData,
  type CreateScheduledTaskActionData,
  type CreateScheduledTaskActionValidationErrors,
  type CreateScheduledTaskNodeData,
} from '../config';
import { scheduledTaskActionRegistry } from '../registry';
import { ActionEditor } from './action-editor';
import { ActionList } from './action-list';
import { ScheduleEditor } from './schedule-editor';

interface CreateScheduledTaskManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

type ActionDialogMode = 'create' | 'edit';

interface ActionDialogState {
  open: boolean;
  mode: ActionDialogMode;
  draft: CreateScheduledTaskActionData | null;
}

function createInitialActionDialogState(): ActionDialogState {
  return {
    open: false,
    mode: 'create',
    draft: null,
  };
}

function canEditActionDraft(action: CreateScheduledTaskActionData | null): boolean {
  return Boolean(
    action &&
      action.action_type === 'send_notification' &&
      (action.channel_type === 'email' || action.channel_type === 'sms')
  );
}

/**
 * @component CreateScheduledTaskManager
 * @category Feature
 * @status Beta
 * @description V2 manager for the create-scheduled-task workflow node using the structured task draft model.
 * @usage Render inside the workflow floating panel when the selected node type is create-scheduled-task.
 * @example
 * <CreateScheduledTaskManager id={nodeId} />
 */
export function CreateScheduledTaskManager({
  id: nodeId,
  className,
  readOnly = false,
}: CreateScheduledTaskManagerProps) {
  const t = useT('nodes');
  const tCommon = useT('common');
  const systemFeatures = useAuthStore.use.systemFeatures();
  const smsTemplates = React.useMemo(
    () => getNotificationSMSTemplates(systemFeatures),
    [systemFeatures]
  );
  const updateData = useNodeDataUpdate<CreateScheduledTaskNodeData>(nodeId);
  const selfNodeData = useNodeData<CreateScheduledTaskNodeData>(nodeId);

  const nodeData = React.useMemo(
    () => normalizeCreateScheduledTaskNodeData(selfNodeData),
    [selfNodeData]
  );
  const [selectedActionId, setSelectedActionId] = React.useState<string | null>(
    nodeData.task.actions[0]?.client_id ?? null
  );
  const [actionDialog, setActionDialog] = React.useState<ActionDialogState>(
    createInitialActionDialogState()
  );
  const [actionDialogErrors, setActionDialogErrors] =
    React.useState<CreateScheduledTaskActionValidationErrors>({});
  const dialogPortalHostRef = React.useRef<HTMLDivElement | null>(null);

  React.useEffect(() => {
    const currentIds = nodeData.task.actions.map(action => action.client_id);

    if (currentIds.length === 0) {
      setSelectedActionId(null);
      return;
    }

    if (!selectedActionId || !currentIds.includes(selectedActionId)) {
      setSelectedActionId(currentIds[0]);
    }
  }, [nodeData.task.actions, selectedActionId]);

  React.useEffect(() => {
    setActionDialog(createInitialActionDialogState());
    setActionDialogErrors({});
  }, [nodeId]);

  const outputs = useNodeOutputVariables(nodeId);

  const updateTask = React.useCallback(
    (
      updater: (
        currentTask: CreateScheduledTaskNodeData['task']
      ) => CreateScheduledTaskNodeData['task']
    ) => {
      updateData(prev => {
        const normalized = normalizeCreateScheduledTaskNodeData(prev);
        return {
          task: updater(normalized.task),
        };
      });
    },
    [updateData]
  );

  const updateSelectedAction = React.useCallback(
    (clientId: string, nextAction: CreateScheduledTaskActionData) => {
      updateTask(currentTask => ({
        ...currentTask,
        actions: currentTask.actions.map(action =>
          action.client_id === clientId ? nextAction : action
        ),
      }));
    },
    [updateTask]
  );

  const closeActionDialog = React.useCallback(() => {
    setActionDialog(createInitialActionDialogState());
    setActionDialogErrors({});
  }, []);

  const updateActionDialogDraft = React.useCallback(
    (updater: (current: CreateScheduledTaskActionData) => CreateScheduledTaskActionData) => {
      setActionDialog(current => {
        if (!current.draft) {
          return current;
        }

        return {
          ...current,
          draft: updater(current.draft),
        };
      });
    },
    []
  );

  const openCreateActionDialog = React.useCallback(() => {
    setActionDialog({
      open: true,
      mode: 'create',
      draft: createDefaultScheduledTaskActionDraft(),
    });
    setActionDialogErrors({});
  }, []);

  const openEditActionDialog = React.useCallback(
    (clientId: string) => {
      const action = nodeData.task.actions.find(item => item.client_id === clientId);

      if (!action) {
        return;
      }

      setSelectedActionId(clientId);
      setActionDialog({
        open: true,
        mode: 'edit',
        draft: cloneCreateScheduledTaskActionData(action),
      });
      setActionDialogErrors({});
    },
    [nodeData.task.actions]
  );

  const handleDeleteAction = React.useCallback(
    (clientId: string) => {
      const currentIndex = nodeData.task.actions.findIndex(action => action.client_id === clientId);
      const remainingActions = nodeData.task.actions.filter(
        action => action.client_id !== clientId
      );
      const nextSelection =
        currentIndex >= 0
          ? (remainingActions[Math.min(currentIndex, remainingActions.length - 1)]?.client_id ??
            null)
          : selectedActionId;

      updateTask(currentTask => ({
        ...currentTask,
        actions: currentTask.actions.filter(action => action.client_id !== clientId),
      }));
      setSelectedActionId(nextSelection);

      if (actionDialog.draft?.client_id === clientId) {
        closeActionDialog();
      }
    },
    [actionDialog.draft, closeActionDialog, nodeData.task.actions, selectedActionId, updateTask]
  );

  const handleToggleActionEnabled = React.useCallback(
    (clientId: string, enabled: boolean) => {
      updateTask(currentTask => ({
        ...currentTask,
        actions: currentTask.actions.map(action =>
          action.client_id === clientId ? { ...action, enabled } : action
        ),
      }));
    },
    [updateTask]
  );

  const handleSaveAction = React.useCallback(() => {
    const draftAction = actionDialog.draft;

    if (!draftAction) {
      return;
    }

    if (actionDialog.mode === 'edit') {
      const exists = nodeData.task.actions.some(
        action => action.client_id === draftAction.client_id
      );

      if (!exists) {
        closeActionDialog();
        return;
      }
    }

    const actionIndex =
      actionDialog.mode === 'create'
        ? nodeData.task.actions.length
        : nodeData.task.actions.findIndex(action => action.client_id === draftAction.client_id);
    const nextErrors = getCreateScheduledTaskActionValidationErrors(
      draftAction,
      actionIndex >= 0 ? actionIndex : nodeData.task.actions.length,
      smsTemplates
    );

    if (hasCreateScheduledTaskActionValidationErrors(nextErrors)) {
      setActionDialogErrors(nextErrors);
      return;
    }

    if (actionDialog.mode === 'create') {
      updateTask(currentTask => ({
        ...currentTask,
        actions: [...currentTask.actions, draftAction],
      }));
    } else {
      updateSelectedAction(draftAction.client_id, draftAction);
    }

    setSelectedActionId(draftAction.client_id);
    closeActionDialog();
  }, [
    actionDialog,
    closeActionDialog,
    nodeData.task.actions,
    smsTemplates,
    updateSelectedAction,
    updateTask,
  ]);

  const dialogAction = actionDialog.draft;
  const dialogActionMeta = dialogAction
    ? scheduledTaskActionRegistry[dialogAction.action_type]
    : null;
  const dialogCanSave = !readOnly && canEditActionDraft(dialogAction);

  return (
    <div className={cn('space-y-5', className)}>
      <section className="space-y-4">
        <h3 className="text-sm font-semibold text-foreground">
          {t('createScheduledTask.section.basic')}
        </h3>

        <div className="space-y-2">
          <Label htmlFor="workflow-task-name">{t('createScheduledTask.fields.taskName')}</Label>
          <WorkflowValueEditor
            nodeId={nodeId}
            value={nodeData.task.name}
            onChange={value =>
              updateTask(currentTask => ({
                ...currentTask,
                name: value,
              }))
            }
            readOnly={readOnly}
            placeholder={t('createScheduledTask.placeholders.taskName')}
            className="w-full"
            editorClassName="min-h-10 rounded-xl border-border bg-background px-3 py-2 shadow-none hover:border-border focus-within:border-primary/70"
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="workflow-task-description">
            {t('createScheduledTask.fields.taskDescription')}
          </Label>
          <WorkflowValueEditor
            nodeId={nodeId}
            value={nodeData.task.description}
            onChange={value =>
              updateTask(currentTask => ({
                ...currentTask,
                description: value,
              }))
            }
            readOnly={readOnly}
            placeholder={t('createScheduledTask.placeholders.taskDescription')}
            className="w-full"
            editorClassName="min-h-[96px] rounded-xl border-border bg-background px-3 py-2.5 shadow-none hover:border-border focus-within:border-primary/70"
          />
        </div>
      </section>

      <ScheduleEditor
        nodeId={nodeId}
        schedule={nodeData.task.schedule}
        readOnly={readOnly}
        onChange={schedule =>
          updateTask(currentTask => ({
            ...currentTask,
            schedule,
          }))
        }
      />

      <section className="space-y-4">
        <ActionList
          actions={nodeData.task.actions}
          selectedActionId={selectedActionId}
          readOnly={readOnly}
          onOpen={openEditActionDialog}
          onAdd={openCreateActionDialog}
          onDelete={handleDeleteAction}
          onToggleEnabled={handleToggleActionEnabled}
        />
      </section>

      <OutputVariablesView variables={outputs} />

      <Dialog
        open={actionDialog.open}
        onOpenChange={open => {
          if (!open) {
            closeActionDialog();
          }
        }}
      >
        <DialogContent
          size="lg"
          className="max-w-2xl overflow-hidden rounded-2xl border-border bg-background p-0 shadow-premium"
        >
          <div ref={dialogPortalHostRef} data-portal-host className="contents" />
          <DialogHeader className="border-b border-border bg-background pb-2.5">
            <DialogTitle className="text-[15px]">
              {actionDialog.mode === 'create'
                ? t('createScheduledTask.actions.addAction')
                : t('createScheduledTask.section.actionSettings')}
            </DialogTitle>
            <DialogDescription className="max-w-2xl text-[10px] leading-4.5 text-muted-foreground">
              {dialogActionMeta
                ? t(dialogActionMeta.descriptionKey as never)
                : t('createScheduledTask.help.actions')}
            </DialogDescription>
          </DialogHeader>

          <DialogBody className="space-y-2.5 bg-background py-3">
            <ActionEditor
              nodeId={nodeId}
              action={dialogAction}
              errors={actionDialogErrors}
              portalRoot={dialogPortalHostRef}
              readOnly={!dialogCanSave}
              onChange={nextAction => {
                setActionDialogErrors({});
                updateActionDialogDraft(() => nextAction);
              }}
            />
          </DialogBody>

          <DialogFooter className="border-t border-border bg-background px-5 py-2.5">
            <Button variant="outline" onClick={closeActionDialog}>
              {dialogCanSave ? tCommon('cancel') : tCommon('close')}
            </Button>
            {dialogCanSave ? <Button onClick={handleSaveAction}>{tCommon('save')}</Button> : null}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export default CreateScheduledTaskManager;
