'use client';

import * as React from 'react';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { ChevronRight, GripVertical, Trash2 } from 'lucide-react';
import { useT } from '@/i18n';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
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
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { OptionEditor } from '@/components/ui/option-editor';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import type {
  AutomationBodyType,
  AutomationWorkflowVersionStrategy,
} from '@/services/types/automation';
import WorkflowInputForm from '@/components/workflow/common/workflow-input-form';
import { NotificationSMSEditor } from '@/components/notification-sms/notification-sms-editor';
import type { FormInputs } from '@/components/workflow/common/workflow-input-form';
import type { InputVar } from '@/components/workflow/types/input-var';
import type { TaskActionTypeOption, TaskChannelTypeOption } from './registry';
import { actionTypeRegistry } from './registry';
import type {
  TaskDraftAction,
  TaskDraftActionErrors,
  TaskWorkflowOption,
  TaskWorkflowVersionOption,
} from './types';
import {
  getUnsupportedWorkflowInputVariables,
  parseWorkflowInputsJsonToFormValues,
  workflowFormValuesToJson,
} from './utils';

const EMAIL_BODY_TYPE_OPTIONS: Array<{
  value: AutomationBodyType;
  labelKey: 'actions.bodyTypeHtml' | 'actions.bodyTypePlainText';
}> = [
  { value: 'text/html', labelKey: 'actions.bodyTypeHtml' },
  { value: 'text/plain', labelKey: 'actions.bodyTypePlainText' },
];

interface TaskActionListItemProps {
  action: TaskDraftAction;
  selected: boolean;
  hasError: boolean;
  disabled: boolean;
  onOpen: (action: TaskDraftAction) => void;
  onDelete: (clientId: string) => void;
  onToggleEnabled: (clientId: string, enabled: boolean) => void;
}

interface TaskEditorActionDialogProps {
  open: boolean;
  mode: 'create' | 'edit';
  step: 'selectType' | 'configure';
  draft: TaskDraftAction | null;
  editable: boolean;
  actionTypeOptions: TaskActionTypeOption[];
  channelOptions: TaskChannelTypeOption[];
  workflowOptions: TaskWorkflowOption[];
  workflowOptionsLoading: boolean;
  workflowVersionOptions: TaskWorkflowVersionOption[];
  workflowVersionOptionsLoading: boolean;
  workflowVersionOptionsError: string | null;
  workflowInputVariables: InputVar[];
  workflowInputVariablesLoading: boolean;
  workflowInputVariablesError: string | null;
  errors: TaskDraftActionErrors;
  onClose: () => void;
  onSelectType: (value: string) => void;
  onBack: () => void;
  onActionTypeChange: (value: string) => void;
  onChannelTypeChange: (value: string) => void;
  onEnabledChange: (checked: boolean) => void;
  onRecipientsChange: (recipients: string[]) => void;
  onSubjectChange: (value: string) => void;
  onBodyTypeChange: (value: AutomationBodyType) => void;
  onBodyChange: (value: string) => void;
  onSmsTemplateChange: (value: string) => void;
  onSmsTemplateParamsChange: (value: Record<string, string>) => void;
  onWorkflowAgentChange: (value: string) => void;
  onWorkflowVersionStrategyChange: (value: AutomationWorkflowVersionStrategy) => void;
  onWorkflowVersionUuidChange: (value: string) => void;
  onWorkflowInputsJsonChange: (value: string) => void;
  onWorkflowTimeoutSecondsChange: (value: string) => void;
  onSave: () => void;
}

interface ActionTypeSelectionCardProps {
  value: string;
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  description: string;
  disabled: boolean;
  onSelect: (value: string) => void;
}

function ActionTypeSelectionCard({
  value,
  icon: Icon,
  title,
  description,
  disabled,
  onSelect,
}: ActionTypeSelectionCardProps) {
  return (
    <button
      type="button"
      onClick={() => onSelect(value)}
      disabled={disabled}
      className={cn(
        'rounded-2xl border border-border bg-background p-3 text-left transition-all',
        'hover:-translate-y-0.5 hover:border-primary/30 hover:bg-muted hover:shadow-sm',
        'disabled:cursor-not-allowed disabled:opacity-70'
      )}
    >
      <div className="flex items-start gap-3">
        <div className="flex size-8 shrink-0 items-center justify-center rounded-xl border border-border bg-muted text-primary">
          <Icon className="size-[15px]" />
        </div>
        <div className="min-w-0">
          <p className="text-[13px] font-medium text-foreground">{title}</p>
          <p className="mt-0.5 text-[10px] leading-4.5 text-muted-foreground">{description}</p>
        </div>
      </div>
    </button>
  );
}

interface EmailChannelEditorProps {
  action: TaskDraftAction;
  editable: boolean;
  errors: TaskDraftActionErrors;
  onRecipientsChange: (recipients: string[]) => void;
  onSubjectChange: (value: string) => void;
  onBodyTypeChange: (value: AutomationBodyType) => void;
  onBodyChange: (value: string) => void;
}

function EmailChannelEditor({
  action,
  editable,
  errors,
  onRecipientsChange,
  onSubjectChange,
  onBodyTypeChange,
  onBodyChange,
}: EmailChannelEditorProps) {
  const t = useT('automation');

  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <div className={cn(!editable && 'pointer-events-none opacity-70')}>
          <OptionEditor
            addButtonPlacement="header"
            options={action.recipients}
            onChange={onRecipientsChange}
            labels={{
              title: t('actions.recipients'),
              add: t('editor.addRecipient'),
              placeholder: index => t('editor.recipientPlaceholder', { index }),
            }}
            classNames={{
              root: 'space-y-2',
              label: 'text-[13px] font-medium text-foreground',
              list: 'space-y-2',
              item: 'items-start gap-2',
              handle:
                'mt-1.5 flex size-7 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted',
              removeButton:
                'mt-1 size-8 rounded-lg text-muted-foreground hover:bg-destructive/10 hover:text-destructive',
              addButton: 'h-8 rounded-xl border-dashed px-3 text-sm font-medium hover:bg-muted',
            }}
          />
        </div>
        {errors.recipients ? (
          <p className="text-xs font-medium text-destructive">{errors.recipients}</p>
        ) : null}
        <p className="text-[10px] leading-4.5 text-muted-foreground">
          {t('actions.emailDescription')}
        </p>
      </div>

      <div className="space-y-1.5 border-t border-border/50 pt-2.5">
        <Label htmlFor="automation-subject" className="text-[13px] font-medium">
          {t('actions.subject')}
        </Label>
        <Input
          id="automation-subject"
          value={action.subject}
          onChange={event => onSubjectChange(event.target.value)}
          placeholder={t('editor.subjectPlaceholder')}
          errorText={errors.subject}
          disabled={!editable}
        />
      </div>

      <div className="space-y-2 border-t border-border/50 pt-2.5">
        <div className="space-y-1.5">
          <Label className="text-[13px] font-medium">{t('actions.content')}</Label>
          <div className="space-y-1.5">
            <Label className="text-[11px] font-medium text-muted-foreground">
              {t('actions.bodyType')}
            </Label>
            <Select
              value={action.bodyType}
              onValueChange={value => onBodyTypeChange(value as AutomationBodyType)}
              disabled={!editable}
            >
              <SelectTrigger className="h-9 sm:max-w-[220px]">
                <SelectValue placeholder={t('actions.bodyType')} />
              </SelectTrigger>
              <SelectContent>
                {EMAIL_BODY_TYPE_OPTIONS.map(option => (
                  <SelectItem key={option.value} value={option.value}>
                    {t(option.labelKey)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-[10px] leading-4.5 text-muted-foreground">
              {action.bodyType === 'text/plain'
                ? t('actions.bodyTypePlainTextDescription')
                : t('actions.bodyTypeHtmlDescription')}
            </p>
            {errors.bodyType ? (
              <p className="text-xs font-medium text-destructive">{errors.bodyType}</p>
            ) : null}
          </div>
        </div>

        <div className="space-y-1.5">
          <Textarea
            id="automation-body"
            value={action.body}
            onChange={event => onBodyChange(event.target.value)}
            className="min-h-[144px] max-h-none"
            disabled={!editable}
            placeholder={t('editor.contentPlaceholder')}
            aria-invalid={Boolean(errors.body)}
          />
          {errors.body ? (
            <p className="text-xs font-medium text-destructive">{errors.body}</p>
          ) : null}
        </div>
      </div>
    </div>
  );
}

interface SendNotificationActionEditorProps {
  action: TaskDraftAction;
  editable: boolean;
  channelOptions: TaskChannelTypeOption[];
  errors: TaskDraftActionErrors;
  onChannelTypeChange: (value: string) => void;
  onRecipientsChange: (recipients: string[]) => void;
  onSubjectChange: (value: string) => void;
  onBodyTypeChange: (value: AutomationBodyType) => void;
  onBodyChange: (value: string) => void;
  onSmsTemplateChange: (value: string) => void;
  onSmsTemplateParamsChange: (value: Record<string, string>) => void;
}

function SendNotificationActionEditor({
  action,
  editable,
  channelOptions,
  errors,
  onChannelTypeChange,
  onRecipientsChange,
  onSubjectChange,
  onBodyTypeChange,
  onBodyChange,
  onSmsTemplateChange,
  onSmsTemplateParamsChange,
}: SendNotificationActionEditorProps) {
  const t = useT('automation');
  const renderEmailEditor = action.channelType === 'email';
  const renderSMSEditor = action.channelType === 'sms';
  const currentChannelOption =
    channelOptions.find(option => option.value === action.channelType) ?? null;

  return (
    <div className="rounded-2xl border border-border/70 bg-muted/15 p-3">
      <div className="space-y-3">
        <div className="space-y-1">
          <p className="text-[10px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
            {t('actions.contentSettings')}
          </p>
        </div>

        <div className="space-y-1.5">
          <Label className="text-[13px] font-medium">{t('actions.channel')}</Label>
          <Select
            value={action.channelType}
            onValueChange={onChannelTypeChange}
            disabled={!editable}
          >
            <SelectTrigger className="h-9">
              <SelectValue placeholder={t('actions.channel')} />
            </SelectTrigger>
            <SelectContent>
              {channelOptions.map(option => (
                <SelectItem key={option.value} value={option.value}>
                  {t(option.labelKey as never)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {errors.channelType ? (
            <p className="text-xs font-medium text-destructive">{errors.channelType}</p>
          ) : null}
          <p className="text-[10px] leading-4.5 text-muted-foreground">
            {currentChannelOption
              ? t(currentChannelOption.descriptionKey as never)
              : t('detail.unsupportedReadonly')}
          </p>
        </div>

        {renderEmailEditor ? (
          <EmailChannelEditor
            action={action}
            editable={editable}
            errors={errors}
            onRecipientsChange={onRecipientsChange}
            onSubjectChange={onSubjectChange}
            onBodyTypeChange={onBodyTypeChange}
            onBodyChange={onBodyChange}
          />
        ) : renderSMSEditor ? (
          <NotificationSMSEditor
            recipientMode="list"
            value={{
              recipients: action.recipients,
              template: action.smsTemplate,
              templateParams: action.smsTemplateParams,
            }}
            errors={{
              recipients: errors.recipients,
              template: errors.smsTemplate,
              templateParams: errors.smsTemplateParams,
            }}
            readOnly={!editable}
            onChange={next => {
              onRecipientsChange(next.recipients);
              onSmsTemplateChange(next.template);
              onSmsTemplateParamsChange(next.templateParams);
            }}
          />
        ) : (
          <Alert>
            <AlertTitle>{t('fallback.unknownChannel')}</AlertTitle>
            <AlertDescription>{t('detail.unsupportedReadonly')}</AlertDescription>
          </Alert>
        )}
      </div>
    </div>
  );
}

interface RunWorkflowActionEditorProps {
  action: TaskDraftAction;
  editable: boolean;
  workflowOptions: TaskWorkflowOption[];
  workflowOptionsLoading: boolean;
  workflowVersionOptions: TaskWorkflowVersionOption[];
  workflowVersionOptionsLoading: boolean;
  workflowVersionOptionsError: string | null;
  workflowInputVariables: InputVar[];
  workflowInputVariablesLoading: boolean;
  workflowInputVariablesError: string | null;
  errors: TaskDraftActionErrors;
  onWorkflowAgentChange: (value: string) => void;
  onWorkflowVersionStrategyChange: (value: AutomationWorkflowVersionStrategy) => void;
  onWorkflowVersionUuidChange: (value: string) => void;
  onWorkflowInputsJsonChange: (value: string) => void;
  onWorkflowTimeoutSecondsChange: (value: string) => void;
}

function RunWorkflowActionEditor({
  action,
  editable,
  workflowOptions,
  workflowOptionsLoading,
  workflowVersionOptions,
  workflowVersionOptionsLoading,
  workflowVersionOptionsError,
  workflowInputVariables,
  workflowInputVariablesLoading,
  workflowInputVariablesError,
  errors,
  onWorkflowAgentChange,
  onWorkflowVersionStrategyChange,
  onWorkflowVersionUuidChange,
  onWorkflowInputsJsonChange,
  onWorkflowTimeoutSecondsChange,
}: RunWorkflowActionEditorProps) {
  const t = useT('automation');
  const hasWorkflowOptions = workflowOptions.length > 0;
  const hasWorkflowVersionOptions = workflowVersionOptions.length > 0;
  const [showAdvancedJson, setShowAdvancedJson] = React.useState(false);
  const unsupportedVariables = React.useMemo(
    () => getUnsupportedWorkflowInputVariables(workflowInputVariables),
    [workflowInputVariables]
  );
  const supportedVariables = React.useMemo(
    () =>
      workflowInputVariables.filter(
        variable => !unsupportedVariables.some(item => item.variable === variable.variable)
      ),
    [unsupportedVariables, workflowInputVariables]
  );
  const variablesSignature = React.useMemo(
    () =>
      JSON.stringify(
        supportedVariables.map(variable => ({
          variable: variable.variable,
          type: variable.type,
          required: Boolean(variable.required),
          options: variable.options ?? [],
          max_length: variable.max_length ?? undefined,
        }))
      ),
    [supportedVariables]
  );
  const initialWorkflowInputsKey = `${action.clientId}-${action.workflowAgentId}-${variablesSignature}`;
  const initialWorkflowInputsRef = React.useRef<{
    key: string;
    values: FormInputs;
  } | null>(null);
  if (initialWorkflowInputsRef.current?.key !== initialWorkflowInputsKey) {
    initialWorkflowInputsRef.current = {
      key: initialWorkflowInputsKey,
      values: parseWorkflowInputsJsonToFormValues(action.workflowInputsJson),
    };
  }
  const initialWorkflowInputs = initialWorkflowInputsRef.current.values;
  const canRenderWorkflowInputForm =
    !workflowInputVariablesLoading && !workflowInputVariablesError && supportedVariables.length > 0;
  const showJsonFallback =
    showAdvancedJson ||
    Boolean(workflowInputVariablesError) ||
    (!workflowInputVariablesLoading &&
      action.workflowAgentId &&
      workflowInputVariables.length === 0);
  const unsupportedVariableNames = unsupportedVariables
    .map(variable => variable.label || variable.variable)
    .join(' / ');
  const handleWorkflowFormChange = React.useCallback(
    (values: FormInputs) => {
      onWorkflowInputsJsonChange(workflowFormValuesToJson(values));
    },
    [onWorkflowInputsJsonChange]
  );

  return (
    <div className="rounded-2xl border border-border/70 bg-muted/15 p-3">
      <div className="space-y-3">
        <p className="text-[10px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
          {t('actions.workflowSettings')}
        </p>

        <div className="space-y-1.5">
          <Label className="text-[13px] font-medium">{t('actions.targetWorkflow')}</Label>
          <Select
            value={action.workflowAgentId}
            onValueChange={onWorkflowAgentChange}
            disabled={!editable || workflowOptionsLoading || !hasWorkflowOptions}
          >
            <SelectTrigger className="h-9">
              <SelectValue
                placeholder={
                  workflowOptionsLoading
                    ? t('actions.loadingWorkflows')
                    : t('actions.targetWorkflowPlaceholder')
                }
              />
            </SelectTrigger>
            <SelectContent>
              {hasWorkflowOptions ? (
                workflowOptions.map(option => (
                  <SelectItem key={option.id} value={option.id}>
                    {option.name}
                  </SelectItem>
                ))
              ) : (
                <SelectItem value="__empty" disabled>
                  {t('actions.noWorkflowOptions')}
                </SelectItem>
              )}
            </SelectContent>
          </Select>
          {errors.workflowAgentId ? (
            <p className="text-xs font-medium text-destructive">{errors.workflowAgentId}</p>
          ) : null}
        </div>

        <div className="grid gap-3 border-t border-border/50 pt-2.5 md:grid-cols-2">
          <div className="space-y-1.5">
            <Label className="text-[13px] font-medium">{t('actions.versionStrategy')}</Label>
            <Select
              value={action.workflowVersionStrategy}
              onValueChange={value =>
                onWorkflowVersionStrategyChange(value as AutomationWorkflowVersionStrategy)
              }
              disabled={!editable}
            >
              <SelectTrigger className="h-9">
                <SelectValue placeholder={t('actions.versionStrategy')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="latest_published">
                  {t('actions.versionLatestPublished')}
                </SelectItem>
                <SelectItem value="pinned">{t('actions.versionPinned')}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="automation-workflow-timeout" className="text-[13px] font-medium">
              {t('actions.timeoutSeconds')}
            </Label>
            <Input
              id="automation-workflow-timeout"
              type="number"
              min={30}
              max={1800}
              value={action.workflowTimeoutSeconds}
              onChange={event => onWorkflowTimeoutSecondsChange(event.target.value)}
              errorText={errors.workflowTimeoutSeconds}
              disabled={!editable}
            />
          </div>
        </div>

        {action.workflowVersionStrategy === 'pinned' ? (
          <div className="space-y-1.5">
            <Label className="text-[13px] font-medium">{t('actions.versionUuid')}</Label>
            <Select
              value={action.workflowVersionUuid}
              onValueChange={onWorkflowVersionUuidChange}
              disabled={
                !editable ||
                !action.workflowAgentId ||
                workflowVersionOptionsLoading ||
                !hasWorkflowVersionOptions
              }
            >
              <SelectTrigger className="h-9">
                <SelectValue
                  placeholder={
                    workflowVersionOptionsLoading
                      ? t('actions.loadingWorkflowVersions')
                      : t('actions.versionUuidPlaceholder')
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {hasWorkflowVersionOptions ? (
                  workflowVersionOptions.map(option => (
                    <SelectItem key={option.id} value={option.id}>
                      {option.label}
                    </SelectItem>
                  ))
                ) : (
                  <SelectItem value="__empty" disabled>
                    {t('actions.noWorkflowVersions')}
                  </SelectItem>
                )}
              </SelectContent>
            </Select>
            {workflowVersionOptionsError ? (
              <p className="text-xs font-medium text-destructive">{workflowVersionOptionsError}</p>
            ) : null}
            {errors.workflowVersionUuid ? (
              <p className="text-xs font-medium text-destructive">{errors.workflowVersionUuid}</p>
            ) : null}
          </div>
        ) : null}

        <div className="space-y-1.5 border-t border-border/50 pt-2.5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Label className="text-[13px] font-medium">{t('actions.workflowInputs')}</Label>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setShowAdvancedJson(current => !current)}
            >
              {showAdvancedJson ? t('actions.hideAdvancedJson') : t('actions.showAdvancedJson')}
            </Button>
          </div>

          {workflowInputVariablesLoading ? (
            <div className="rounded-xl border border-border/70 bg-muted/20 p-4 text-sm text-muted-foreground">
              {t('actions.loadingWorkflowInputs')}
            </div>
          ) : null}

          {unsupportedVariables.length > 0 ? (
            <Alert>
              <AlertTitle>{t('actions.unsupportedWorkflowInputs')}</AlertTitle>
              <AlertDescription>
                {t('actions.unsupportedWorkflowInputsDescription', {
                  names: unsupportedVariableNames,
                })}
              </AlertDescription>
            </Alert>
          ) : null}

          {canRenderWorkflowInputForm ? (
            <div className={cn(!editable && 'pointer-events-none opacity-70')}>
              <WorkflowInputForm
                key={`${action.clientId}-${action.workflowAgentId}-${variablesSignature}`}
                startVariables={supportedVariables}
                initialValues={initialWorkflowInputs}
                isStarting={false}
                onSubmit={handleWorkflowFormChange}
                onChange={handleWorkflowFormChange}
                hideSubmitButton
                showResetButton
              />
            </div>
          ) : null}

          {!workflowInputVariablesLoading &&
          !workflowInputVariablesError &&
          action.workflowAgentId &&
          workflowInputVariables.length === 0 ? (
            <div className="rounded-xl border border-border/70 bg-muted/20 p-4 text-sm text-muted-foreground">
              {t('actions.noWorkflowInputs')}
            </div>
          ) : null}

          {workflowInputVariablesError ? (
            <Alert>
              <AlertTitle>{t('actions.workflowInputsLoadFailedTitle')}</AlertTitle>
              <AlertDescription>{workflowInputVariablesError}</AlertDescription>
            </Alert>
          ) : null}

          {showJsonFallback ? (
            <div className="space-y-1.5">
              <Label htmlFor="automation-workflow-inputs" className="text-[13px] font-medium">
                {t('actions.workflowInputsJson')}
              </Label>
              <Textarea
                id="automation-workflow-inputs"
                value={action.workflowInputsJson}
                onChange={event => onWorkflowInputsJsonChange(event.target.value)}
                className="min-h-[144px] max-h-none font-mono text-xs"
                disabled={!editable}
                placeholder={t('actions.workflowInputsPlaceholder')}
                aria-invalid={Boolean(errors.workflowInputsJson)}
              />
            </div>
          ) : null}

          {errors.workflowInputsJson ? (
            <p className="text-xs font-medium text-destructive">{errors.workflowInputsJson}</p>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function ActionSettingsSection({
  draft,
  editable,
  errors,
  actionTypeOptions,
  currentActionTypeOption,
  onActionTypeChange,
  onEnabledChange,
}: {
  draft: TaskDraftAction;
  editable: boolean;
  errors: TaskDraftActionErrors;
  actionTypeOptions: TaskActionTypeOption[];
  currentActionTypeOption: TaskActionTypeOption | null;
  onActionTypeChange: (value: string) => void;
  onEnabledChange: (checked: boolean) => void;
}) {
  const t = useT('automation');
  const tCommon = useT('common');

  return (
    <div className="rounded-2xl border border-border/70 bg-muted/15 p-3">
      <div className="space-y-3">
        <p className="text-[10px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
          {t('actions.actionSettings')}
        </p>

        <div className="space-y-1.5">
          <Label className="text-[13px] font-medium">{t('actions.type')}</Label>
          <Select value={draft.actionType} onValueChange={onActionTypeChange} disabled={!editable}>
            <SelectTrigger className="h-9 rounded-xl border-border bg-background shadow-none hover:border-border">
              <SelectValue placeholder={t('actions.type')} />
            </SelectTrigger>
            <SelectContent>
              {actionTypeOptions.map(option => (
                <SelectItem key={option.value} value={option.value}>
                  {t(option.labelKey as never)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {errors.actionType ? (
            <p className="text-xs font-medium text-destructive">{errors.actionType}</p>
          ) : null}
          <p className="text-[10px] leading-4.5 text-muted-foreground">
            {currentActionTypeOption
              ? t(currentActionTypeOption.descriptionKey as never)
              : t('detail.unsupportedReadonly')}
          </p>
        </div>

        <div className="flex items-center justify-between gap-4 border-t border-border/50 pt-2.5">
          <div className="space-y-0.5">
            <p className="text-[13px] font-medium text-foreground">{t('actions.enabledLabel')}</p>
            <p className="text-[10px] leading-4.5 text-muted-foreground">
              {draft.enabled ? tCommon('enabled') : tCommon('disabled')}
            </p>
          </div>
          <Switch checked={draft.enabled} onCheckedChange={onEnabledChange} disabled={!editable} />
        </div>
      </div>
    </div>
  );
}

/**
 * @component TaskActionListItem
 * @category Feature
 * @status Stable
 * @description Sortable list item for a scheduled task operation inside the task editor.
 * @usage Render inside the task action list with DnD context enabled.
 */
export function TaskActionListItem({
  action,
  selected,
  hasError,
  disabled,
  onOpen,
  onDelete,
  onToggleEnabled,
}: TaskActionListItemProps) {
  const t = useT('automation');
  const tCommon = useT('common');
  const actionMeta = actionTypeRegistry[action.actionType];
  const actionIcon = actionMeta?.icon ?? ChevronRight;
  const ActionIcon = actionIcon;
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: action.clientId,
    disabled,
  });

  return (
    <div
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
      }}
      className={cn(isDragging && 'z-10')}
    >
      <div
        className={cn(
          'flex items-center gap-2 rounded-2xl border px-2 py-1 transition-all',
          selected
            ? 'border-primary/40 bg-primary/5 shadow-sm'
            : 'border-border/70 bg-background hover:border-primary/20',
          hasError && 'border-destructive/40',
          disabled && 'opacity-70',
          isDragging && 'shadow-lg'
        )}
      >
        <button
          type="button"
          {...attributes}
          {...listeners}
          disabled={disabled}
          className={cn(
            'inline-flex size-6 shrink-0 items-center justify-center self-center text-muted-foreground/65',
            !disabled && 'cursor-grab touch-none active:cursor-grabbing'
          )}
          aria-label={t('actions.listTitle')}
          title={t('actions.listTitle')}
        >
          <GripVertical className="size-3.5 shrink-0" strokeWidth={2.2} />
        </button>

        <button
          type="button"
          onClick={() => onOpen(action)}
          disabled={disabled}
          className={cn(
            'min-w-0 flex-1 rounded-xl px-1 py-1 text-left transition-all',
            'disabled:cursor-not-allowed'
          )}
        >
          <div className="flex min-w-0 items-center gap-2">
            <div
              className={cn(
                'flex size-8 shrink-0 items-center justify-center rounded-xl border',
                selected
                  ? 'border-primary/20 bg-primary/10 text-primary'
                  : 'border-border bg-muted text-muted-foreground'
              )}
            >
              <ActionIcon className="size-[15px]" />
            </div>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-foreground">
                {actionMeta ? t(actionMeta.labelKey as never) : t('fallback.unknownAction')}
              </p>
            </div>
          </div>
        </button>

        <div
          className="flex shrink-0 items-center gap-2"
          onClick={event => event.stopPropagation()}
          onPointerDown={event => event.stopPropagation()}
        >
          <span
            className={cn(
              'text-xs font-medium',
              action.enabled ? 'text-foreground' : 'text-muted-foreground',
              hasError && 'text-destructive'
            )}
          >
            {action.enabled ? tCommon('enabled') : tCommon('disabled')}
          </span>
          <Switch
            checked={action.enabled}
            onCheckedChange={checked => onToggleEnabled(action.clientId, checked)}
            disabled={disabled}
            aria-label={t('actions.enabledLabel')}
          />
          <Button
            type="button"
            variant="ghost"
            size="sm"
            isIcon
            className="size-8 rounded-full text-muted-foreground/80 hover:text-destructive"
            onClick={event => {
              event.stopPropagation();
              onDelete(action.clientId);
            }}
            disabled={disabled}
            aria-label={tCommon('delete')}
            title={tCommon('delete')}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}

/**
 * @component TaskEditorActionDialog
 * @category Feature
 * @status Stable
 * @description Dialog for creating and editing scheduled task operations in the automation task editor.
 * @usage Render from the main task editor panel and wire it to local operation draft state.
 */
export function TaskEditorActionDialog({
  open,
  mode,
  step,
  draft,
  editable,
  actionTypeOptions,
  channelOptions,
  workflowOptions,
  workflowOptionsLoading,
  workflowVersionOptions,
  workflowVersionOptionsLoading,
  workflowVersionOptionsError,
  workflowInputVariables,
  workflowInputVariablesLoading,
  workflowInputVariablesError,
  errors,
  onClose,
  onSelectType,
  onBack,
  onActionTypeChange,
  onChannelTypeChange,
  onEnabledChange,
  onRecipientsChange,
  onSubjectChange,
  onBodyTypeChange,
  onBodyChange,
  onSmsTemplateChange,
  onSmsTemplateParamsChange,
  onWorkflowAgentChange,
  onWorkflowVersionStrategyChange,
  onWorkflowVersionUuidChange,
  onWorkflowInputsJsonChange,
  onWorkflowTimeoutSecondsChange,
  onSave,
}: TaskEditorActionDialogProps) {
  const t = useT('automation');
  const tCommon = useT('common');
  const currentActionTypeOption = draft
    ? (actionTypeOptions.find(option => option.value === draft.actionType) ?? null)
    : null;

  return (
    <Dialog
      open={open}
      onOpenChange={nextOpen => {
        if (!nextOpen) {
          onClose();
        }
      }}
    >
      <DialogContent
        size="xl"
        className="max-w-2xl overflow-hidden rounded-2xl border-border bg-background p-0 shadow-premium"
      >
        <DialogHeader className="border-b border-border bg-background pb-2.5">
          <DialogTitle className="text-[15px]">
            {mode === 'create' && step === 'selectType'
              ? t('actions.selectTypeTitle')
              : mode === 'create'
                ? t('actions.addAction')
                : t('actions.detailTitle')}
          </DialogTitle>
          {mode === 'create' && step === 'selectType' ? (
            <DialogDescription className="max-w-2xl text-[10px] leading-4.5 text-muted-foreground">
              {t('actions.selectTypeDescription')}
            </DialogDescription>
          ) : null}
        </DialogHeader>

        <DialogBody className="space-y-2.5 bg-background py-3">
          {mode === 'create' && step === 'selectType' ? (
            <div className="space-y-2.5">
              {actionTypeOptions.map(option => (
                <ActionTypeSelectionCard
                  key={option.value}
                  value={option.value}
                  icon={option.icon}
                  title={t(option.labelKey as never)}
                  description={t(option.descriptionKey as never)}
                  disabled={!editable}
                  onSelect={onSelectType}
                />
              ))}
            </div>
          ) : draft ? (
            <div className="space-y-3">
              <ActionSettingsSection
                draft={draft}
                editable={editable}
                errors={errors}
                actionTypeOptions={actionTypeOptions}
                currentActionTypeOption={currentActionTypeOption}
                onActionTypeChange={onActionTypeChange}
                onEnabledChange={onEnabledChange}
              />
              {draft.actionType === 'send_notification' ? (
                <SendNotificationActionEditor
                  action={draft}
                  editable={editable}
                  channelOptions={channelOptions}
                  errors={errors}
                  onChannelTypeChange={onChannelTypeChange}
                  onRecipientsChange={onRecipientsChange}
                  onSubjectChange={onSubjectChange}
                  onBodyTypeChange={onBodyTypeChange}
                  onBodyChange={onBodyChange}
                  onSmsTemplateChange={onSmsTemplateChange}
                  onSmsTemplateParamsChange={onSmsTemplateParamsChange}
                />
              ) : draft.actionType === 'run_workflow' ? (
                <RunWorkflowActionEditor
                  action={draft}
                  editable={editable}
                  workflowOptions={workflowOptions}
                  workflowOptionsLoading={workflowOptionsLoading}
                  workflowVersionOptions={workflowVersionOptions}
                  workflowVersionOptionsLoading={workflowVersionOptionsLoading}
                  workflowVersionOptionsError={workflowVersionOptionsError}
                  workflowInputVariables={workflowInputVariables}
                  workflowInputVariablesLoading={workflowInputVariablesLoading}
                  workflowInputVariablesError={workflowInputVariablesError}
                  errors={errors}
                  onWorkflowAgentChange={onWorkflowAgentChange}
                  onWorkflowVersionStrategyChange={onWorkflowVersionStrategyChange}
                  onWorkflowVersionUuidChange={onWorkflowVersionUuidChange}
                  onWorkflowInputsJsonChange={onWorkflowInputsJsonChange}
                  onWorkflowTimeoutSecondsChange={onWorkflowTimeoutSecondsChange}
                />
              ) : (
                <Alert>
                  <AlertTitle>{t('fallback.unknownAction')}</AlertTitle>
                  <AlertDescription>{t('detail.unsupportedReadonly')}</AlertDescription>
                </Alert>
              )}
            </div>
          ) : null}
        </DialogBody>

        <DialogFooter className="border-t border-border bg-background px-5 py-2.5">
          {mode === 'create' && step === 'configure' ? (
            <Button variant="outline" onClick={onBack}>
              {tCommon('back')}
            </Button>
          ) : null}
          <Button variant="outline" onClick={onClose}>
            {tCommon('cancel')}
          </Button>
          {step === 'configure' ? (
            <Button onClick={onSave} disabled={!editable}>
              {tCommon('save')}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
