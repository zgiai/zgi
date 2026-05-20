'use client';

import React from 'react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { WorkflowValueEditor } from '@/components/workflow/ui';
import { WorkflowValueListEditor } from '@/components/workflow/common/workflow-value-list-editor';
import { useT } from '@/i18n';
import type {
  CreateScheduledTaskChannelType,
  CreateScheduledTaskActionValidationErrors,
  ScheduledTaskNotificationDraft,
} from '../config';

interface NotificationEmailEditorProps {
  nodeId: string;
  portalRoot?: React.ComponentProps<typeof WorkflowValueEditor>['portalRoot'];
  channelType?: CreateScheduledTaskChannelType;
  channelOptions: Array<{
    value: string;
    labelKey: string;
  }>;
  channelDescription: string;
  showEmailFields?: boolean;
  notification: ScheduledTaskNotificationDraft;
  errors?: CreateScheduledTaskActionValidationErrors;
  readOnly?: boolean;
  children?: React.ReactNode;
  onChannelTypeChange: (next: CreateScheduledTaskChannelType) => void;
  onChange: (next: ScheduledTaskNotificationDraft) => void;
}

const BODY_TYPE_OPTIONS: Array<{
  value: ScheduledTaskNotificationDraft['body_type'];
  labelKey: string;
}> = [
  { value: 'text/html', labelKey: 'createScheduledTask.bodyTypeHtml' },
  { value: 'text/plain', labelKey: 'createScheduledTask.bodyTypePlainText' },
];

/**
 * @component NotificationEmailEditor
 * @category Feature
 * @status Beta
 * @description Variable-friendly email notification editor for scheduled task actions.
 * @usage Render inside the action detail panel when the selected channel is email.
 * @example
 * <NotificationEmailEditor nodeId={id} notification={notification} onChange={setNotification} />
 */
export function NotificationEmailEditor({
  nodeId,
  portalRoot,
  channelType,
  channelOptions,
  channelDescription,
  showEmailFields = true,
  notification,
  errors,
  readOnly = false,
  children,
  onChannelTypeChange,
  onChange,
}: NotificationEmailEditorProps) {
  const t = useT('nodes');

  return (
    <div className="rounded-2xl border border-border/70 bg-muted/15 p-3">
      <div className="space-y-3">
        <p className="text-[10px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
          {t('createScheduledTask.section.actionContent')}
        </p>

        <div className="space-y-1.5">
          <Label className="text-[13px] font-medium">
            {t('createScheduledTask.fields.channelType')}
          </Label>
          <Select
            value={channelType ?? 'email'}
            onValueChange={value => onChannelTypeChange(value as CreateScheduledTaskChannelType)}
            disabled={readOnly}
          >
            <SelectTrigger className="h-9 rounded-xl border-border bg-background shadow-none hover:border-border">
              <SelectValue placeholder={t('createScheduledTask.fields.channelType')} />
            </SelectTrigger>
            <SelectContent>
              {channelOptions.map(option => (
                <SelectItem key={option.value} value={option.value}>
                  {t(option.labelKey as never)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {errors?.channelType ? (
            <p className="text-xs font-medium text-destructive">
              {t(errors.channelType.code as never, errors.channelType.params as never)}
            </p>
          ) : null}
          <p className="text-[10px] leading-4.5 text-muted-foreground">{channelDescription}</p>
        </div>

        {showEmailFields ? (
          <>
            <div className="space-y-1.5">
              <WorkflowValueListEditor
                nodeId={nodeId}
                portalRoot={portalRoot}
                value={notification.recipients}
                onChange={recipients =>
                  onChange({
                    ...notification,
                    recipients,
                  })
                }
                readOnly={readOnly}
                addButtonPlacement="header"
                labels={{
                  title: t('createScheduledTask.fields.recipients'),
                  add: t('createScheduledTask.actions.addRecipient'),
                  placeholder: index =>
                    t('createScheduledTask.placeholders.recipient', { index: index + 1 }),
                  remove: index =>
                    t('createScheduledTask.actions.removeRecipient', { index: index + 1 }),
                }}
              />
              {errors?.recipients ? (
                <p className="text-xs font-medium text-destructive">
                  {t(errors.recipients.code as never, errors.recipients.params as never)}
                </p>
              ) : null}
              <p className="text-[10px] leading-4.5 text-muted-foreground">
                {t('createScheduledTask.help.emailRecipients')}
              </p>
            </div>

            <div className="space-y-1.5 border-t border-border/50 pt-2.5">
              <Label htmlFor="scheduled-task-subject" className="text-[13px] font-medium">
                {t('createScheduledTask.fields.subject')}
              </Label>
              <WorkflowValueEditor
                nodeId={nodeId}
                portalRoot={portalRoot}
                value={notification.subject}
                onChange={subject =>
                  onChange({
                    ...notification,
                    subject,
                  })
                }
                readOnly={readOnly}
                placeholder={t('createScheduledTask.placeholders.subject')}
                className="w-full"
                editorClassName="min-h-[36px] rounded-xl border-border bg-background px-3 py-2 shadow-none hover:border-border focus-within:border-primary/70"
              />
              {errors?.subject ? (
                <p className="text-xs font-medium text-destructive">
                  {t(errors.subject.code as never, errors.subject.params as never)}
                </p>
              ) : null}
            </div>

            <div className="space-y-2 border-t border-border/50 pt-2.5">
              <div className="space-y-1.5">
                <Label htmlFor="scheduled-task-body" className="text-[13px] font-medium">
                  {t('createScheduledTask.fields.body')}
                </Label>
                <Label className="text-[11px] font-medium text-muted-foreground">
                  {t('createScheduledTask.fields.bodyType')}
                </Label>
                <Select
                  value={notification.body_type}
                  onValueChange={value =>
                    onChange({
                      ...notification,
                      body_type: value as ScheduledTaskNotificationDraft['body_type'],
                    })
                  }
                  disabled={readOnly}
                >
                  <SelectTrigger className="h-9 rounded-xl border-border bg-background shadow-none hover:border-border sm:max-w-[220px]">
                    <SelectValue placeholder={t('createScheduledTask.fields.bodyType')} />
                  </SelectTrigger>
                  <SelectContent>
                    {BODY_TYPE_OPTIONS.map(option => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.labelKey as never)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-[10px] leading-4.5 text-muted-foreground">
                  {notification.body_type === 'text/plain'
                    ? t('createScheduledTask.help.bodyTypePlainText')
                    : t('createScheduledTask.help.bodyTypeHtml')}
                </p>
                {errors?.bodyType ? (
                  <p className="text-xs font-medium text-destructive">
                    {t(errors.bodyType.code as never, errors.bodyType.params as never)}
                  </p>
                ) : null}
              </div>

              <div className="space-y-1.5">
                <WorkflowValueEditor
                  nodeId={nodeId}
                  portalRoot={portalRoot}
                  value={notification.body}
                  onChange={body =>
                    onChange({
                      ...notification,
                      body,
                    })
                  }
                  readOnly={readOnly}
                  placeholder={t('createScheduledTask.placeholders.body')}
                  className="w-full"
                  editorClassName="min-h-[132px] rounded-xl border-border bg-background px-3 py-2.5 shadow-none hover:border-border focus-within:border-primary/70"
                />
                {errors?.body ? (
                  <p className="text-xs font-medium text-destructive">
                    {t(errors.body.code as never, errors.body.params as never)}
                  </p>
                ) : null}
              </div>
            </div>
          </>
        ) : children ? (
          children
        ) : (
          <Alert>
            <AlertTitle>{t('createScheduledTask.empty.unsupportedActionTitle')}</AlertTitle>
            <AlertDescription>{t('createScheduledTask.help.unsupportedChannel')}</AlertDescription>
          </Alert>
        )}
      </div>
    </div>
  );
}

export default NotificationEmailEditor;
