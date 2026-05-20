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
import { Switch } from '@/components/ui/switch';
import { useT } from '@/i18n';
import { useAuthStore } from '@/store/auth-store';
import { isNotificationSMSAutomationChannelEnabled } from '@/lib/features/notification-sms';
import { NotificationSMSEditor } from '@/components/notification-sms/notification-sms-editor';
import type {
  CreateScheduledTaskActionData,
  CreateScheduledTaskActionValidationErrors,
  ScheduledTaskNotificationDraft,
} from '../config';
import {
  scheduledTaskActionOptions,
  scheduledTaskActionRegistry,
  scheduledTaskChannelOptions,
  scheduledTaskChannelRegistry,
} from '../registry';
import { NotificationEmailEditor } from './notification-email-editor';

interface ActionEditorProps {
  nodeId: string;
  action: CreateScheduledTaskActionData | null;
  errors?: CreateScheduledTaskActionValidationErrors;
  readOnly?: boolean;
  portalRoot?: React.ComponentProps<typeof NotificationEmailEditor>['portalRoot'];
  onChange: (next: CreateScheduledTaskActionData) => void;
}

/**
 * @component ActionEditor
 * @category Feature
 * @status Beta
 * @description Detail editor for a selected create-scheduled-task action draft.
 * @usage Render beside the action list and pass the selected action plus update handler.
 * @example
 * <ActionEditor nodeId={id} action={selectedAction} onChange={setSelectedAction} />
 */
export function ActionEditor({
  nodeId,
  action,
  errors,
  readOnly = false,
  portalRoot,
  onChange,
}: ActionEditorProps) {
  const t = useT('nodes');
  const tCommon = useT('common');
  const systemFeatures = useAuthStore.use.systemFeatures();
  const smsEnabled = isNotificationSMSAutomationChannelEnabled(systemFeatures);

  if (!action) {
    return (
      <div className="flex min-h-[280px] items-center justify-center rounded-2xl border border-dashed border-border bg-muted/10 px-6 py-10 text-center">
        <div>
          <p className="text-sm font-medium text-foreground">
            {t('createScheduledTask.empty.noActionSelectedTitle')}
          </p>
          <p className="mt-2 text-xs leading-5 text-muted-foreground">
            {t('createScheduledTask.empty.noActionSelectedDescription')}
          </p>
        </div>
      </div>
    );
  }

  const actionMeta = scheduledTaskActionRegistry[action.action_type];
  const actionOptions = scheduledTaskActionOptions.some(option => option.value === action.action_type)
    ? scheduledTaskActionOptions
    : [...scheduledTaskActionOptions, actionMeta];
  const baseChannelOptions = smsEnabled
    ? [...scheduledTaskChannelOptions, scheduledTaskChannelRegistry.sms]
    : scheduledTaskChannelOptions;
  const supportedChannelOptions = baseChannelOptions.filter(option =>
    actionMeta.channelTypes.includes(option.value)
  );
  const channelMeta = action.channel_type
    ? scheduledTaskChannelRegistry[action.channel_type]
    : scheduledTaskChannelRegistry.email;
  const availableChannelOptions =
    channelMeta && !supportedChannelOptions.some(option => option.value === channelMeta.value)
      ? [...supportedChannelOptions, channelMeta]
      : supportedChannelOptions;
  const isSupportedActionType = action.action_type === 'send_notification';
  const editorReadOnly = readOnly || !isSupportedActionType;

  return (
    <div className="space-y-3">
      <div className="rounded-2xl border border-border/70 bg-muted/15 p-3">
        <div className="space-y-3">
          <p className="text-[10px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
            {t('createScheduledTask.section.actionSettings')}
          </p>

          <div className="space-y-1.5">
            <Label className="text-[13px] font-medium">
              {t('createScheduledTask.fields.actionType')}
            </Label>
            <Select
              value={action.action_type}
              onValueChange={value =>
                onChange({
                  ...action,
                  action_type: value as CreateScheduledTaskActionData['action_type'],
                  channel_type: scheduledTaskActionRegistry[
                    value as CreateScheduledTaskActionData['action_type']
                  ].channelTypes[0],
                })
              }
              disabled={editorReadOnly}
            >
              <SelectTrigger className="h-9 rounded-xl border-border bg-background shadow-none hover:border-border">
                <SelectValue placeholder={t('createScheduledTask.fields.actionType')} />
              </SelectTrigger>
              <SelectContent>
                {actionOptions.map(option => (
                  <SelectItem key={option.value} value={option.value}>
                    {t(option.labelKey as never)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {errors?.actionType ? (
              <p className="text-xs font-medium text-destructive">
                {t(errors.actionType.code as never, errors.actionType.params as never)}
              </p>
            ) : null}
          </div>

          <div className="flex items-center justify-between gap-4 border-t border-border/50 pt-2.5">
            <div className="space-y-0.5">
              <p className="text-[13px] font-medium text-foreground">
                {t('createScheduledTask.fields.enabled')}
              </p>
              <p className="text-[10px] leading-4.5 text-muted-foreground">
                {action.enabled ? tCommon('enabled') : tCommon('disabled')}
              </p>
            </div>
            <Switch
              checked={action.enabled}
              onCheckedChange={enabled =>
                onChange({
                  ...action,
                  enabled,
                })
              }
              disabled={editorReadOnly}
              aria-label={t('createScheduledTask.fields.enabled')}
            />
          </div>
        </div>
      </div>

      {action.action_type === 'send_notification' ? (
        <NotificationEmailEditor
          nodeId={nodeId}
          portalRoot={portalRoot}
          channelType={action.channel_type}
          channelOptions={availableChannelOptions}
          channelDescription={
            channelMeta
              ? t(channelMeta.descriptionKey as never)
              : t('createScheduledTask.help.unsupportedChannel')
          }
          showEmailFields={action.channel_type === 'email'}
          notification={action.notification ?? getFallbackNotificationDraft()}
          errors={errors}
          readOnly={readOnly || !isSupportedActionType}
          onChannelTypeChange={channel_type =>
            onChange({
              ...action,
              channel_type,
            })
          }
          onChange={(notification: ScheduledTaskNotificationDraft) =>
            onChange({
              ...action,
              notification,
            })
          }
        >
          {action.channel_type === 'sms' ? (
            <NotificationSMSEditor
              nodeId={nodeId}
              portalRoot={portalRoot}
              recipientMode="list"
              value={{
                recipients: action.notification?.recipients ?? [''],
                notificationTitle: action.notification?.notification_title ?? '',
                linkCode: action.notification?.link_code ?? '',
              }}
              errors={{
                recipients: errors?.recipients
                  ? t(errors.recipients.code as never, errors.recipients.params as never)
                  : undefined,
                notificationTitle: errors?.notificationTitle
                  ? t(
                      errors.notificationTitle.code as never,
                      errors.notificationTitle.params as never
                    )
                  : undefined,
                linkCode: errors?.linkCode
                  ? t(errors.linkCode.code as never, errors.linkCode.params as never)
                  : undefined,
              }}
              readOnly={readOnly || !isSupportedActionType}
              onChange={next =>
                onChange({
                  ...action,
                  notification: {
                    ...(action.notification ?? getFallbackNotificationDraft()),
                    recipients: next.recipients,
                    notification_title: next.notificationTitle,
                    link_code: next.linkCode,
                  },
                })
              }
            />
          ) : null}
        </NotificationEmailEditor>
      ) : (
        <Alert>
          <AlertTitle>{t('createScheduledTask.empty.unsupportedActionTitle')}</AlertTitle>
          <AlertDescription>{t('createScheduledTask.help.unsupportedAction')}</AlertDescription>
        </Alert>
      )}
    </div>
  );
}

function getFallbackNotificationDraft(): ScheduledTaskNotificationDraft {
  return {
    recipients: [''],
    subject: '',
    body: '',
    body_type: 'text/html',
    template: 'pending_action_notification',
    notification_title: '',
    link_code: '',
  };
}

export default ActionEditor;
