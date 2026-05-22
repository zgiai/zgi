'use client';

import * as React from 'react';
import { useLocale } from 'next-intl';
import { useQuery } from '@tanstack/react-query';
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { ChevronRight, Plus, X } from 'lucide-react';
import { useT } from '@/i18n';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Textarea } from '@/components/ui/textarea';
import type {
  AutomationBodyType,
  AutomationWorkflowVersionStrategy,
  AutomationTaskDetailData,
  CreateAutomationTaskRequest,
  UpdateAutomationTaskRequest,
} from '@/services/types/automation';
import { useAgents } from '@/hooks/agent/use-agents';
import { useGenerateAutomationTaskDraft } from '@/hooks/automation/use-automation';
import { AgentType } from '@/services/types/agent';
import { workflowService } from '@/services/workflow.service';
import { WebAppService } from '@/services/webapp.service';
import { useAuthStore } from '@/store/auth-store';
import { isNotificationSMSAutomationChannelEnabled } from '@/lib/features/notification-sms';
import type { WebAppVariable } from '@/services/types/webapp';
import type { InputVar } from '@/components/workflow/types/input-var';
import {
  actionTypeOptions,
  actionTypeRegistry,
  channelTypeOptions,
  channelTypeRegistry,
} from './registry';
import { TaskActionListItem, TaskEditorActionDialog } from './task-editor-panel-action-ui';
import { TaskEditorScheduleSection } from './task-editor-panel-schedule-section';
import {
  createDefaultTaskActionDraft,
  createDefaultTaskDraft,
  draftToCreateRequest,
  draftToUpdateRequest,
  generatedTaskDraftToDraft,
  getBrowserTimezone,
  getMissingRequiredWorkflowInputVariables,
  parseWorkflowInputsJsonToFormValues,
  safeJson,
  taskDetailToDraft,
  validateTaskDraft,
} from './utils';
import { TaskEditorAIDraftCard } from './task-editor-ai-draft-card';
import type {
  TaskDraft,
  TaskDraftAction,
  TaskDraftActionErrors,
  TaskValidationErrors,
  TaskWeekdayKey,
  TaskWorkflowOption,
  TaskWorkflowVersionOption,
} from './types';

interface TaskEditorPanelProps {
  mode: 'create' | 'edit';
  workspaceId: string;
  canManage: boolean;
  isSubmitting: boolean;
  taskDetail?: AutomationTaskDetailData;
  onCancel: () => void;
  onSubmitted: (taskDetail: AutomationTaskDetailData) => void;
  onCreate: (data: CreateAutomationTaskRequest) => Promise<AutomationTaskDetailData | undefined>;
  onUpdate: (
    id: string,
    data: UpdateAutomationTaskRequest
  ) => Promise<AutomationTaskDetailData | undefined>;
}

interface ActionDialogState {
  open: boolean;
  mode: 'create' | 'edit';
  step: 'selectType' | 'configure';
  draft: TaskDraftAction | null;
}

function getDraftFromProps(
  mode: 'create' | 'edit',
  taskDetail: AutomationTaskDetailData | undefined
): TaskDraft {
  if (mode === 'edit' && taskDetail) {
    return taskDetailToDraft(taskDetail);
  }

  return createDefaultTaskDraft();
}

function getInitialSelectedActionId(draft: TaskDraft): string | null {
  return draft.actions[0]?.clientId ?? null;
}

function getFallbackChannelType(actionType: string, smsEnabled: boolean): string {
  if (actionType === 'run_workflow') {
    return '';
  }

  return (
    actionTypeRegistry[actionType]?.channelTypes.find(
      channelType => channelType !== 'sms' || smsEnabled
    ) ??
    channelTypeOptions.find(option => option.value !== 'sms' || smsEnabled)?.value ??
    'email'
  );
}

function toWorkflowInputVars(variables: WebAppVariable[] | undefined): InputVar[] {
  return (variables ?? []).map(variable => ({
    type: variable.type as InputVar['type'],
    variable: variable.variable,
    label: variable.label || variable.variable,
    description: variable.description,
    required: Boolean(variable.required),
    max_length: variable.max_length,
    default: variable.default,
    options: variable.options,
    allowed_file_upload_methods: variable.allowed_file_upload_methods,
    allowed_file_types: variable.allowed_file_types,
    allowed_file_extensions: variable.allowed_file_extensions,
  }));
}

function cloneActionDraft(action: TaskDraftAction): TaskDraftAction {
  return {
    ...action,
    recipients: [...action.recipients],
    rawConfig: action.rawConfig ? { ...action.rawConfig } : null,
  };
}

function removeActionError(current: TaskValidationErrors, clientId: string): TaskValidationErrors {
  if (!current.actionErrors?.[clientId]) {
    return current;
  }

  const nextActionErrors = { ...current.actionErrors };
  delete nextActionErrors[clientId];

  return {
    ...current,
    actionErrors: Object.keys(nextActionErrors).length > 0 ? nextActionErrors : undefined,
  };
}

function formatWorkflowVersionLabel(version: string, createdAt?: string): string {
  if (!createdAt) {
    return version;
  }

  const timestamp = Date.parse(createdAt);
  if (Number.isNaN(timestamp)) {
    return version;
  }

  return `${version} - ${new Date(timestamp).toLocaleString()}`;
}

/**
 * @component TaskEditorPanel
 * @category Feature
 * @status Stable
 * @description Create and edit form for scheduled tasks, built on an extensible draft model.
 * @usage Render inside the task workbench panel for create and edit flows.
 */
export function TaskEditorPanel({
  mode,
  workspaceId,
  canManage,
  isSubmitting,
  taskDetail,
  onCancel,
  onSubmitted,
  onCreate,
  onUpdate,
}: TaskEditorPanelProps) {
  const t = useT('automation');
  const tCommon = useT('common');
  const locale = useLocale();
  const systemFeatures = useAuthStore.use.systemFeatures();
  const smsEnabled = isNotificationSMSAutomationChannelEnabled(systemFeatures);
  const initialDraft = React.useMemo(() => getDraftFromProps(mode, taskDetail), [mode, taskDetail]);
  const [draft, setDraft] = React.useState<TaskDraft>(initialDraft);
  const [errors, setErrors] = React.useState<TaskValidationErrors>({});
  const [selectedActionId, setSelectedActionId] = React.useState<string | null>(
    getInitialSelectedActionId(initialDraft)
  );
  const [actionDialog, setActionDialog] = React.useState<ActionDialogState>({
    open: false,
    mode: 'create',
    step: 'selectType',
    draft: null,
  });
  const [actionDialogErrors, setActionDialogErrors] = React.useState<TaskDraftActionErrors>({});
  const [deleteTargetId, setDeleteTargetId] = React.useState<string | null>(null);
  const [discardConfirmOpen, setDiscardConfirmOpen] = React.useState(false);
  const [aiDraftPrompt, setAIDraftPrompt] = React.useState('');
  const [aiDraftSummary, setAIDraftSummary] = React.useState('');
  const [aiDraftWarnings, setAIDraftWarnings] = React.useState<string[]>([]);
  const [aiDraftMissingFields, setAIDraftMissingFields] = React.useState<string[]>([]);
  const { generateAutomationTaskDraft, isGenerating: isGeneratingAIDraft } =
    useGenerateAutomationTaskDraft();

  React.useEffect(() => {
    setDraft(initialDraft);
    setErrors({});
    setSelectedActionId(getInitialSelectedActionId(initialDraft));
    setActionDialog({
      open: false,
      mode: 'create',
      step: 'selectType',
      draft: null,
    });
    setActionDialogErrors({});
    setDeleteTargetId(null);
    setDiscardConfirmOpen(false);
    setAIDraftSummary('');
    setAIDraftWarnings([]);
    setAIDraftMissingFields([]);
  }, [initialDraft]);

  React.useEffect(() => {
    if (draft.actions.length === 0) {
      if (selectedActionId !== null) {
        setSelectedActionId(null);
      }
      return;
    }

    if (!selectedActionId || !draft.actions.some(action => action.clientId === selectedActionId)) {
      setSelectedActionId(draft.actions[0]?.clientId ?? null);
    }
  }, [draft.actions, selectedActionId]);

  const editable = canManage && draft.support.isEditable;
  const translate = React.useCallback(
    (key: string, values?: Record<string, string | number>) => t(key as never, values as never),
    [t]
  );
  const dialogDraft = actionDialog.draft;
  const dialogWorkflowAgentId =
    dialogDraft?.actionType === 'run_workflow' ? dialogDraft.workflowAgentId : '';
  const workflowAgents = useAgents(
    {
      workspace_id: workspaceId,
      limit: 100,
    },
    {
      enabled: actionDialog.open && Boolean(workspaceId),
      staleTime: 60 * 1000,
    }
  );
  const workflowOptions = React.useMemo<TaskWorkflowOption[]>(() => {
    return workflowAgents.pages
      .flat()
      .filter(agent => agent.agent_type === AgentType.WORKFLOW && agent.is_published)
      .map(agent => ({
        id: agent.id,
        name: agent.name,
        description: agent.description,
        isPublished: agent.is_published,
      }));
  }, [workflowAgents.pages]);
  const latestWorkflowVersion = useQuery({
    queryKey: ['automation', 'run-workflow', 'latest-version', dialogWorkflowAgentId],
    queryFn: () => workflowService.getLatestWorkflowVersion(dialogWorkflowAgentId),
    enabled:
      actionDialog.open &&
      dialogDraft?.actionType === 'run_workflow' &&
      Boolean(dialogWorkflowAgentId),
    staleTime: 60 * 1000,
  });
  const publishedWorkflowVersions = useQuery({
    queryKey: ['automation', 'run-workflow', 'published-versions', dialogWorkflowAgentId],
    queryFn: () => workflowService.getPublishedWorkflowVersions(dialogWorkflowAgentId),
    enabled:
      actionDialog.open &&
      dialogDraft?.actionType === 'run_workflow' &&
      Boolean(dialogWorkflowAgentId),
    staleTime: 60 * 1000,
  });
  const workflowVersionOptions = React.useMemo<TaskWorkflowVersionOption[]>(() => {
    return (publishedWorkflowVersions.data?.data?.data ?? []).map(version => ({
      id: version.version_uuid,
      workflowId: version.workflow_id,
      label: formatWorkflowVersionLabel(version.version, version.created_at),
      version: version.version,
      createdAt: version.created_at,
    }));
  }, [publishedWorkflowVersions.data?.data?.data]);
  const workflowVersionOptionsError = publishedWorkflowVersions.isError
    ? t('actions.workflowVersionsLoadFailed')
    : null;
  const selectedWorkflowWebAppId = latestWorkflowVersion.data?.data?.web_app_id ?? '';
  const selectedWorkflowConfig = useQuery({
    queryKey: ['automation', 'run-workflow', 'webapp-config', selectedWorkflowWebAppId],
    queryFn: () => WebAppService.getConfig(selectedWorkflowWebAppId),
    enabled:
      actionDialog.open &&
      dialogDraft?.actionType === 'run_workflow' &&
      Boolean(selectedWorkflowWebAppId),
    staleTime: 60 * 1000,
  });
  const workflowInputVariables = React.useMemo(
    () => toWorkflowInputVars(selectedWorkflowConfig.data?.data?.variables),
    [selectedWorkflowConfig.data?.data?.variables]
  );
  const workflowInputVariablesLoading =
    latestWorkflowVersion.isFetching || selectedWorkflowConfig.isFetching;
  const workflowInputVariablesError =
    latestWorkflowVersion.isError || selectedWorkflowConfig.isError
      ? t('actions.workflowInputsLoadFailed')
      : !workflowInputVariablesLoading &&
          Boolean(dialogWorkflowAgentId) &&
          latestWorkflowVersion.isSuccess &&
          !selectedWorkflowWebAppId
        ? t('actions.workflowInputsLoadFailed')
        : null;

  const dialogActionTypeOptions = React.useMemo(() => {
    if (!dialogDraft?.actionType || actionTypeRegistry[dialogDraft.actionType]) {
      return actionTypeOptions;
    }

    return [
      ...actionTypeOptions,
      {
        value: dialogDraft.actionType,
        icon: ChevronRight,
        labelKey: 'fallback.unknownAction',
        descriptionKey: 'detail.unsupportedReadonly',
        channelTypes: [],
      },
    ];
  }, [dialogDraft]);

  const dialogChannelOptions = React.useMemo(() => {
    const visibleChannelTypeOptions = channelTypeOptions.filter(
      option => option.value !== 'sms' || smsEnabled
    );

    if (!dialogDraft) {
      return visibleChannelTypeOptions;
    }

    const allowedChannelTypes = (
      actionTypeRegistry[dialogDraft.actionType]?.channelTypes ?? []
    ).filter(channelType => channelType !== 'sms' || smsEnabled);
    const mappedOptions = allowedChannelTypes
      .map(channelType => channelTypeRegistry[channelType])
      .filter(Boolean);

    if (mappedOptions.length > 0 && channelTypeRegistry[dialogDraft.channelType]) {
      return mappedOptions;
    }

    if (channelTypeRegistry[dialogDraft.channelType]) {
      return [channelTypeRegistry[dialogDraft.channelType]];
    }

    if (dialogDraft.channelType) {
      return [
        ...mappedOptions,
        {
          value: dialogDraft.channelType,
          icon: ChevronRight,
          labelKey: 'fallback.unknownChannel',
          descriptionKey: 'detail.unsupportedReadonly',
        },
      ];
    }

    return mappedOptions.length > 0 ? mappedOptions : visibleChannelTypeOptions;
  }, [dialogDraft, smsEnabled]);

  const actionSensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 6,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  );
  const isDirty = React.useMemo(
    () => JSON.stringify(draft) !== JSON.stringify(initialDraft),
    [draft, initialDraft]
  );

  const updateDraft = React.useCallback((updater: (current: TaskDraft) => TaskDraft) => {
    setDraft(current => updater(current));
  }, []);

  const handleCancel = React.useCallback(() => {
    if (editable && isDirty) {
      setDiscardConfirmOpen(true);
      return;
    }

    onCancel();
  }, [editable, isDirty, onCancel]);

  const closeActionDialog = React.useCallback(() => {
    setActionDialog({
      open: false,
      mode: 'create',
      step: 'selectType',
      draft: null,
    });
    setActionDialogErrors({});
  }, []);

  const updateActionDialogDraft = React.useCallback(
    (updater: (current: TaskDraftAction) => TaskDraftAction) => {
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

  const toggleWeekday = React.useCallback(
    (weekday: TaskWeekdayKey) => {
      updateDraft(current => {
        const currentDays = current.schedule.recurringDays;
        const nextDays = currentDays.includes(weekday)
          ? currentDays.filter(item => item !== weekday)
          : [...currentDays, weekday];

        return {
          ...current,
          schedule: {
            ...current.schedule,
            recurringDays: nextDays,
          },
        };
      });
    },
    [updateDraft]
  );

  const handleDeleteAction = React.useCallback(
    (clientId: string) => {
      const currentIndex = draft.actions.findIndex(action => action.clientId === clientId);
      const remainingActions = draft.actions.filter(action => action.clientId !== clientId);
      const nextSelection =
        currentIndex >= 0
          ? (remainingActions[Math.min(currentIndex, remainingActions.length - 1)]?.clientId ??
            null)
          : selectedActionId;

      updateDraft(current => ({
        ...current,
        actions: current.actions.filter(action => action.clientId !== clientId),
      }));
      setSelectedActionId(nextSelection);
      setErrors(current => removeActionError(current, clientId));
    },
    [draft.actions, selectedActionId, updateDraft]
  );

  const handleToggleActionEnabled = React.useCallback(
    (clientId: string, enabled: boolean) => {
      updateDraft(current => ({
        ...current,
        actions: current.actions.map(action =>
          action.clientId === clientId ? { ...action, enabled } : action
        ),
      }));
    },
    [updateDraft]
  );

  const handleActionDragEnd = React.useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;

      if (!over || active.id === over.id) {
        return;
      }

      updateDraft(current => {
        const oldIndex = current.actions.findIndex(action => action.clientId === String(active.id));
        const newIndex = current.actions.findIndex(action => action.clientId === String(over.id));

        if (oldIndex < 0 || newIndex < 0) {
          return current;
        }

        return {
          ...current,
          actions: arrayMove(current.actions, oldIndex, newIndex),
        };
      });
    },
    [updateDraft]
  );

  const openCreateActionDialog = React.useCallback(() => {
    setActionDialog({
      open: true,
      mode: 'create',
      step: 'selectType',
      draft: null,
    });
    setActionDialogErrors({});
  }, []);

  const openEditActionDialog = React.useCallback(
    (action: TaskDraftAction) => {
      setSelectedActionId(action.clientId);
      setActionDialog({
        open: true,
        mode: 'edit',
        step: 'configure',
        draft: cloneActionDraft(action),
      });
      setActionDialogErrors(errors.actionErrors?.[action.clientId] ?? {});
    },
    [errors.actionErrors]
  );

  const handleSelectActionType = React.useCallback((value: string) => {
    setActionDialog({
      open: true,
      mode: 'create',
      step: 'configure',
      draft: createDefaultTaskActionDraft(value),
    });
    setActionDialogErrors({});
  }, []);

  const handleBackToTypeSelection = React.useCallback(() => {
    setActionDialog(current => ({
      ...current,
      step: 'selectType',
      draft: null,
    }));
    setActionDialogErrors({});
  }, []);

  const handleDialogActionTypeChange = React.useCallback(
    (value: string) => {
      updateActionDialogDraft(current => {
        const allowedChannelTypes = (actionTypeRegistry[value]?.channelTypes ?? []).filter(
          channelType => channelType !== 'sms' || smsEnabled
        );
        const nextChannelType = allowedChannelTypes.includes(current.channelType)
          ? current.channelType
          : (allowedChannelTypes[0] ?? getFallbackChannelType(value, smsEnabled));

        return {
          ...current,
          actionType: value,
          channelType: nextChannelType,
        };
      });
    },
    [smsEnabled, updateActionDialogDraft]
  );

  const handleDialogChannelTypeChange = React.useCallback(
    (value: string) => {
      updateActionDialogDraft(current => ({
        ...current,
        channelType: value,
      }));
    },
    [updateActionDialogDraft]
  );

  const handleDialogActionEnabledChange = React.useCallback(
    (checked: boolean) => {
      updateActionDialogDraft(current => ({
        ...current,
        enabled: checked,
      }));
    },
    [updateActionDialogDraft]
  );

  const handleDialogRecipientsChange = React.useCallback(
    (recipients: string[]) => {
      updateActionDialogDraft(current => ({
        ...current,
        recipients,
      }));
    },
    [updateActionDialogDraft]
  );

  const handleDialogSubjectChange = React.useCallback(
    (value: string) => {
      updateActionDialogDraft(current => ({
        ...current,
        subject: value,
      }));
    },
    [updateActionDialogDraft]
  );

  const handleDialogBodyTypeChange = React.useCallback(
    (value: AutomationBodyType) => {
      updateActionDialogDraft(current => ({
        ...current,
        bodyType: value,
      }));
    },
    [updateActionDialogDraft]
  );

  const handleDialogBodyChange = React.useCallback(
    (value: string) => {
      updateActionDialogDraft(current => ({
        ...current,
        body: value,
      }));
    },
    [updateActionDialogDraft]
  );

  const handleDialogSmsNotificationTitleChange = React.useCallback(
    (value: string) => {
      updateActionDialogDraft(current => ({
        ...current,
        smsNotificationTitle: value,
      }));
    },
    [updateActionDialogDraft]
  );

  const handleDialogSmsLinkCodeChange = React.useCallback(
    (value: string) => {
      updateActionDialogDraft(current => ({
        ...current,
        smsLinkCode: value,
      }));
    },
    [updateActionDialogDraft]
  );

  const handleDialogWorkflowAgentChange = React.useCallback(
    (value: string) => {
      updateActionDialogDraft(current => ({
        ...current,
        workflowAgentId: value,
        workflowVersionUuid: '',
        workflowInputsJson: '{}',
      }));
    },
    [updateActionDialogDraft]
  );

  const handleDialogWorkflowVersionStrategyChange = React.useCallback(
    (value: AutomationWorkflowVersionStrategy) => {
      updateActionDialogDraft(current => ({
        ...current,
        workflowVersionStrategy: value,
        workflowVersionUuid: value === 'pinned' ? current.workflowVersionUuid : '',
      }));
    },
    [updateActionDialogDraft]
  );

  const handleDialogWorkflowVersionUuidChange = React.useCallback(
    (value: string) => {
      updateActionDialogDraft(current => ({
        ...current,
        workflowVersionUuid: value,
      }));
    },
    [updateActionDialogDraft]
  );

  const handleDialogWorkflowInputsJsonChange = React.useCallback(
    (value: string) => {
      updateActionDialogDraft(current => ({
        ...current,
        workflowInputsJson: value,
      }));
    },
    [updateActionDialogDraft]
  );

  const handleDialogWorkflowTimeoutSecondsChange = React.useCallback(
    (value: string) => {
      updateActionDialogDraft(current => ({
        ...current,
        workflowTimeoutSeconds: value,
      }));
    },
    [updateActionDialogDraft]
  );

  const handleSaveAction = React.useCallback(() => {
    if (!dialogDraft) {
      return;
    }

    const nextDraft: TaskDraft =
      actionDialog.mode === 'create'
        ? {
            ...draft,
            actions: [...draft.actions, dialogDraft],
          }
        : {
            ...draft,
            actions: draft.actions.map(action =>
              action.clientId === dialogDraft.clientId ? dialogDraft : action
            ),
          };

    const nextErrors = validateTaskDraft(nextDraft, translate);
    const currentActionErrors: TaskDraftActionErrors = {
      ...(nextErrors.actionErrors?.[dialogDraft.clientId] ?? {}),
    };

    if (
      dialogDraft.actionType === 'run_workflow' &&
      workflowInputVariables.length > 0 &&
      !workflowInputVariablesError
    ) {
      const missingRequiredInputs = getMissingRequiredWorkflowInputVariables(
        workflowInputVariables,
        parseWorkflowInputsJsonToFormValues(dialogDraft.workflowInputsJson)
      );

      if (missingRequiredInputs.length > 0) {
        currentActionErrors.workflowInputsJson = translate(
          'editor.validation.workflowRequiredInputsMissing',
          {
            names: missingRequiredInputs
              .map(variable => variable.label || variable.variable)
              .join(' / '),
          }
        );
      }
    }

    if (Object.keys(currentActionErrors).length > 0) {
      setActionDialogErrors(currentActionErrors);
      setErrors(current => ({
        ...current,
        actionsRequired: nextErrors.actionsRequired,
        actionErrors: {
          ...(current.actionErrors ?? {}),
          [dialogDraft.clientId]: currentActionErrors,
        },
      }));
      return;
    }

    setDraft(nextDraft);
    setSelectedActionId(dialogDraft.clientId);
    setErrors(current =>
      removeActionError(
        {
          ...current,
          actionsRequired: undefined,
        },
        dialogDraft.clientId
      )
    );
    closeActionDialog();
  }, [
    actionDialog.mode,
    closeActionDialog,
    dialogDraft,
    draft,
    translate,
    workflowInputVariables,
    workflowInputVariablesError,
  ]);

  const handleSubmit = React.useCallback(async () => {
    const nextErrors = validateTaskDraft(draft, translate);
    setErrors(nextErrors);

    if (Object.keys(nextErrors).length > 0 || !editable) {
      return;
    }

    if (mode === 'create') {
      const created = await onCreate(draftToCreateRequest(draft, workspaceId));
      if (created) {
        onSubmitted(created);
      }
      return;
    }

    if (!taskDetail?.task.id) {
      return;
    }

    const updated = await onUpdate(taskDetail.task.id, draftToUpdateRequest(draft, workspaceId));
    if (updated) {
      onSubmitted(updated);
    }
  }, [
    draft,
    editable,
    mode,
    onCreate,
    onSubmitted,
    onUpdate,
    taskDetail?.task.id,
    translate,
    workspaceId,
  ]);

  const handleGenerateAIDraft = React.useCallback(async () => {
    const prompt = aiDraftPrompt.trim();
    if (!prompt || !editable || mode !== 'create') {
      return;
    }

    let result: Awaited<ReturnType<typeof generateAutomationTaskDraft>>;
    try {
      result = await generateAutomationTaskDraft({
        workspace_id: workspaceId,
        prompt,
        locale,
        timezone: getBrowserTimezone(),
      });
    } catch {
      return;
    }
    if (!result?.draft) {
      return;
    }

    const nextDraft = generatedTaskDraftToDraft(result.draft);
    setDraft(nextDraft);
    setSelectedActionId(getInitialSelectedActionId(nextDraft));
    setErrors({});
    setAIDraftSummary(result.summary ?? '');
    setAIDraftWarnings(result.warnings ?? []);
    setAIDraftMissingFields(result.missing_fields ?? []);
  }, [aiDraftPrompt, editable, generateAutomationTaskDraft, locale, mode, workspaceId]);

  return (
    <div className="flex h-full min-h-0 w-full flex-col">
      <div className="border-b border-border/70 px-5 py-4">
        <div className="flex items-start justify-between gap-3">
          <div className="space-y-2">
            <h2 className="text-lg font-semibold text-foreground">
              {mode === 'create' ? t('editor.createTitle') : t('editor.editTitle')}
            </h2>
            <p className="text-sm text-muted-foreground">
              {mode === 'create' ? t('editor.createDescription') : t('editor.editDescription')}
            </p>
          </div>
          <Button variant="ghost" size="sm" isIcon onClick={handleCancel}>
            <X className="size-4" />
          </Button>
        </div>
      </div>

      <ScrollArea className="h-0 grow">
        <div className="space-y-5 p-5">
          {!canManage ? (
            <Alert>
              <AlertTitle>{t('operations.readOnly')}</AlertTitle>
              <AlertDescription>{t('noManagePermission')}</AlertDescription>
            </Alert>
          ) : null}

          {!draft.support.isEditable ? (
            <Alert>
              <AlertTitle>{t('operations.readOnly')}</AlertTitle>
              <AlertDescription>{t('editor.unsupportedEdit')}</AlertDescription>
            </Alert>
          ) : null}

          {mode === 'create' ? (
            <TaskEditorAIDraftCard
              value={aiDraftPrompt}
              disabled={!editable}
              loading={isGeneratingAIDraft}
              summary={aiDraftSummary}
              warnings={aiDraftWarnings}
              missingFields={aiDraftMissingFields}
              onChange={setAIDraftPrompt}
              onGenerate={handleGenerateAIDraft}
            />
          ) : null}

          <Card className="border-border/70" padding="none">
            <CardHeader className="pb-3">
              <CardTitle className="text-base">{tCommon('name')}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="automation-task-name">{t('editor.name')}</Label>
                <Input
                  id="automation-task-name"
                  value={draft.name}
                  onChange={event =>
                    updateDraft(current => ({
                      ...current,
                      name: event.target.value,
                    }))
                  }
                  placeholder={t('editor.namePlaceholder')}
                  errorText={errors.name}
                  disabled={!editable}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="automation-task-description">{tCommon('description')}</Label>
                <Textarea
                  id="automation-task-description"
                  value={draft.description}
                  onChange={event =>
                    updateDraft(current => ({
                      ...current,
                      description: event.target.value,
                    }))
                  }
                  placeholder={t('editor.descriptionPlaceholder')}
                  disabled={!editable}
                  className="min-h-[104px]"
                />
              </div>
            </CardContent>
          </Card>

          <TaskEditorScheduleSection
            schedule={draft.schedule}
            editable={editable}
            errors={errors}
            onScheduleChange={updater =>
              updateDraft(current => ({
                ...current,
                schedule: updater(current.schedule),
              }))
            }
            onToggleWeekday={toggleWeekday}
          />

          <Card className="border-border/70" padding="none">
            <CardHeader className="pb-3">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <CardTitle className="text-base">{t('actions.title')}</CardTitle>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={openCreateActionDialog}
                  disabled={!editable}
                >
                  <Plus className="size-4" />
                  {t('actions.addAction')}
                </Button>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {errors.actionsRequired ? (
                <p className="text-xs font-medium text-destructive">{errors.actionsRequired}</p>
              ) : null}

              {draft.actions.length > 0 ? (
                <DndContext
                  sensors={actionSensors}
                  collisionDetection={closestCenter}
                  onDragEnd={handleActionDragEnd}
                >
                  <SortableContext
                    items={draft.actions.map(action => action.clientId)}
                    strategy={verticalListSortingStrategy}
                  >
                    <div className="space-y-3">
                      {draft.actions.map(action => (
                        <TaskActionListItem
                          key={action.clientId}
                          action={action}
                          selected={action.clientId === selectedActionId}
                          hasError={Boolean(errors.actionErrors?.[action.clientId])}
                          disabled={!editable}
                          onOpen={openEditActionDialog}
                          onDelete={setDeleteTargetId}
                          onToggleEnabled={handleToggleActionEnabled}
                        />
                      ))}
                    </div>
                  </SortableContext>
                </DndContext>
              ) : (
                <div className="flex min-h-[180px] flex-col items-center justify-center rounded-2xl border border-dashed border-border/70 bg-muted/10 px-6 py-10 text-center">
                  <div className="rounded-2xl border border-border/70 bg-background p-3 text-primary">
                    <ChevronRight className="size-5" />
                  </div>
                  <p className="mt-4 text-sm font-medium text-foreground">
                    {t('actions.emptyTitle')}
                  </p>
                  <p className="mt-2 max-w-sm text-xs leading-6 text-muted-foreground">
                    {t('actions.emptyDescription')}
                  </p>
                  <Button
                    type="button"
                    variant="outline"
                    className="mt-4"
                    onClick={openCreateActionDialog}
                    disabled={!editable}
                  >
                    <Plus className="size-4" />
                    {t('actions.addAction')}
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          {!draft.support.isEditable ? (
            <Card className="border-border/70" padding="none">
              <CardHeader className="pb-3">
                <CardTitle className="text-base">{t('detail.rawConfig')}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <pre className="overflow-x-auto rounded-xl border border-border/70 bg-muted/30 p-4 text-xs leading-6 text-muted-foreground">
                  {safeJson(draft.schedule.rawConfig)}
                </pre>
                {draft.actions.map(action => (
                  <pre
                    key={action.clientId}
                    className="overflow-x-auto rounded-xl border border-border/70 bg-muted/30 p-4 text-xs leading-6 text-muted-foreground"
                  >
                    {safeJson(action.rawConfig)}
                  </pre>
                ))}
              </CardContent>
            </Card>
          ) : null}
        </div>
      </ScrollArea>

      <div className="border-t border-border bg-background px-5 py-4">
        <div className="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
          <Button variant="outline" onClick={handleCancel}>
            {tCommon('cancel')}
          </Button>
          <Button onClick={handleSubmit} loading={isSubmitting} disabled={!editable}>
            {mode === 'create' ? t('editor.saveCreate') : t('editor.saveUpdate')}
          </Button>
        </div>
      </div>

      <TaskEditorActionDialog
        open={actionDialog.open}
        mode={actionDialog.mode}
        step={actionDialog.step}
        draft={dialogDraft}
        editable={editable}
        actionTypeOptions={dialogActionTypeOptions}
        channelOptions={dialogChannelOptions}
        workflowOptions={workflowOptions}
        workflowOptionsLoading={workflowAgents.isLoading || workflowAgents.isFetching}
        workflowVersionOptions={workflowVersionOptions}
        workflowVersionOptionsLoading={
          publishedWorkflowVersions.isLoading || publishedWorkflowVersions.isFetching
        }
        workflowVersionOptionsError={workflowVersionOptionsError}
        workflowInputVariables={workflowInputVariables}
        workflowInputVariablesLoading={workflowInputVariablesLoading}
        workflowInputVariablesError={workflowInputVariablesError}
        errors={actionDialogErrors}
        onClose={closeActionDialog}
        onSelectType={handleSelectActionType}
        onBack={handleBackToTypeSelection}
        onActionTypeChange={handleDialogActionTypeChange}
        onChannelTypeChange={handleDialogChannelTypeChange}
        onEnabledChange={handleDialogActionEnabledChange}
        onRecipientsChange={handleDialogRecipientsChange}
        onSubjectChange={handleDialogSubjectChange}
        onBodyTypeChange={handleDialogBodyTypeChange}
        onBodyChange={handleDialogBodyChange}
        onSmsNotificationTitleChange={handleDialogSmsNotificationTitleChange}
        onSmsLinkCodeChange={handleDialogSmsLinkCodeChange}
        onWorkflowAgentChange={handleDialogWorkflowAgentChange}
        onWorkflowVersionStrategyChange={handleDialogWorkflowVersionStrategyChange}
        onWorkflowVersionUuidChange={handleDialogWorkflowVersionUuidChange}
        onWorkflowInputsJsonChange={handleDialogWorkflowInputsJsonChange}
        onWorkflowTimeoutSecondsChange={handleDialogWorkflowTimeoutSecondsChange}
        onSave={handleSaveAction}
      />

      <ConfirmDialog
        variant="warning"
        open={discardConfirmOpen}
        onOpenChange={setDiscardConfirmOpen}
        title={t('editor.discardTitle')}
        description={t('editor.discardDescription')}
        confirmText={t('editor.discardConfirm')}
        cancelText={tCommon('cancel')}
        onConfirm={onCancel}
      />

      <ConfirmDialog
        variant="warning"
        open={Boolean(deleteTargetId)}
        onOpenChange={open => {
          if (!open) {
            setDeleteTargetId(null);
          }
        }}
        title={t('actions.deleteConfirmTitle')}
        description={t('actions.deleteConfirmDescription')}
        confirmText={tCommon('delete')}
        cancelText={tCommon('cancel')}
        onConfirm={() => {
          if (deleteTargetId) {
            handleDeleteAction(deleteTargetId);
          }
        }}
      />
    </div>
  );
}
