'use client';

import * as React from 'react';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { OptionEditor } from '@/components/ui/option-editor';
import { WorkflowValueEditor } from '@/components/workflow/ui';
import { WorkflowValueListEditor } from '@/components/workflow/common/workflow-value-list-editor';
import { useT } from '@/i18n';
import { getNotificationSMSPreviewTemplate } from '@/lib/features/notification-sms';
import { cn } from '@/lib/utils';
import { useAuthStore } from '@/store/auth-store';
import type { NotificationSMSDraft, NotificationSMSErrors } from './types';
import { NotificationSMSPreview } from './notification-sms-preview';
import { isNotificationSMSLinkCodeValid } from './validation';

interface NotificationSMSEditorProps {
  nodeId?: string;
  portalRoot?: React.ComponentProps<typeof WorkflowValueEditor>['portalRoot'];
  value: NotificationSMSDraft;
  errors?: NotificationSMSErrors;
  readOnly?: boolean;
  recipientMode?: 'single' | 'list';
  className?: string;
  onChange: (next: NotificationSMSDraft) => void;
}

export function NotificationSMSEditor({
  nodeId,
  portalRoot,
  value,
  errors,
  readOnly = false,
  recipientMode = 'single',
  className,
  onChange,
}: NotificationSMSEditorProps) {
  const t = useT('common');
  const systemFeatures = useAuthStore.use.systemFeatures();
  const previewTemplate = getNotificationSMSPreviewTemplate(systemFeatures);
  const workflowNodeId = nodeId;
  const canUseWorkflowValues = typeof workflowNodeId === 'string' && workflowNodeId.length > 0;
  const recipientPlaceholder =
    recipientMode === 'single'
      ? t('notificationSms.placeholders.recipientSingle' as never)
      : undefined;
  const linkCodeInvalidMessage = getCommonMessage(
    t,
    'notificationSms.validation.linkCodeInvalid',
    '链接后缀格式不正确，例如 /a/abc123。不要输入完整链接、中文或空格。'
  );
  const localLinkCodeError =
    value.linkCode.trim() &&
    !isNotificationSMSLinkCodeValid(value.linkCode, {
      allowWorkflowToken: canUseWorkflowValues,
    })
      ? linkCodeInvalidMessage
      : undefined;
  const linkCodeError = errors?.linkCode ?? localLinkCodeError;

  const setRecipients = React.useCallback(
    (recipients: string[]) => {
      onChange({
        ...value,
        recipients: recipientMode === 'single' ? [recipients[0] ?? ''] : recipients,
      });
    },
    [onChange, recipientMode, value]
  );

  return (
    <div className={cn('space-y-3', className)}>
      <div className="space-y-1.5">
        {canUseWorkflowValues && recipientMode === 'single' ? (
          <div className="space-y-2.5">
            <p className="text-[13px] font-medium text-foreground">
              {t('notificationSms.fields.recipients' as never)}
            </p>
            <WorkflowValueEditor
              nodeId={workflowNodeId}
              portalRoot={portalRoot}
              value={value.recipients[0] ?? ''}
              onChange={recipient => setRecipients([recipient])}
              readOnly={readOnly}
              placeholder={recipientPlaceholder}
              className="w-full"
              editorClassName="min-h-[40px] rounded-xl border-border bg-background px-3 py-2 shadow-none hover:border-border focus-within:border-primary/70"
            />
          </div>
        ) : canUseWorkflowValues ? (
          <WorkflowValueListEditor
            nodeId={workflowNodeId}
            portalRoot={portalRoot}
            value={value.recipients}
            onChange={setRecipients}
            readOnly={readOnly}
            addButtonPlacement="header"
            labels={{
              title: t('notificationSms.fields.recipients' as never),
              add: t('notificationSms.actions.addRecipient' as never),
              placeholder: index =>
                t('notificationSms.placeholders.recipient' as never, { index: index + 1 } as never),
              remove: index =>
                t(
                  'notificationSms.actions.removeRecipient' as never,
                  { index: index + 1 } as never
                ),
            }}
          />
        ) : (
          <div
            className={cn(!readOnly && 'space-y-1.5', readOnly && 'pointer-events-none opacity-70')}
          >
            <OptionEditor
              addButtonPlacement="header"
              options={recipientMode === 'single' ? [value.recipients[0] ?? ''] : value.recipients}
              onChange={setRecipients}
              labels={{
                title: t('notificationSms.fields.recipients' as never),
                add: t('notificationSms.actions.addRecipient' as never),
                placeholder: index =>
                  recipientMode === 'single' && index === 0
                    ? (recipientPlaceholder ?? '')
                    : t('notificationSms.placeholders.recipient' as never, { index } as never),
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
        )}
        {errors?.recipients ? (
          <p className="text-xs font-medium text-destructive">{errors.recipients}</p>
        ) : null}
        <p className="text-[10px] leading-4.5 text-muted-foreground">
          {t('notificationSms.help.recipients' as never)}
        </p>
      </div>

      <div className="space-y-1.5 border-t border-border/50 pt-2.5">
        <Label className="text-[13px] font-medium">
          {t('notificationSms.fields.notificationTitle' as never)}
        </Label>
        {canUseWorkflowValues ? (
          <WorkflowValueEditor
            nodeId={workflowNodeId}
            portalRoot={portalRoot}
            value={value.notificationTitle}
            onChange={notificationTitle => onChange({ ...value, notificationTitle })}
            readOnly={readOnly}
            placeholder={t('notificationSms.placeholders.notificationTitle' as never)}
            className="w-full"
            editorClassName="min-h-[36px] rounded-xl border-border bg-background px-3 py-2 shadow-none hover:border-border focus-within:border-primary/70"
          />
        ) : (
          <Input
            value={value.notificationTitle}
            onChange={event => onChange({ ...value, notificationTitle: event.target.value })}
            placeholder={t('notificationSms.placeholders.notificationTitle' as never)}
            errorText={errors?.notificationTitle}
            disabled={readOnly}
          />
        )}
        {canUseWorkflowValues && errors?.notificationTitle ? (
          <p className="text-xs font-medium text-destructive">{errors.notificationTitle}</p>
        ) : null}
      </div>

      <div className="space-y-1.5 border-t border-border/50 pt-2.5">
        <Label className="text-[13px] font-medium">
          {t('notificationSms.fields.linkCode' as never)}
        </Label>
        {canUseWorkflowValues ? (
          <WorkflowValueEditor
            nodeId={workflowNodeId}
            portalRoot={portalRoot}
            value={value.linkCode}
            onChange={linkCode => onChange({ ...value, linkCode })}
            readOnly={readOnly}
            placeholder={t('notificationSms.placeholders.linkCode' as never)}
            className="w-full"
            editorClassName="min-h-[36px] rounded-xl border-border bg-background px-3 py-2 shadow-none hover:border-border focus-within:border-primary/70"
          />
        ) : (
          <Input
            value={value.linkCode}
            onChange={event => onChange({ ...value, linkCode: event.target.value })}
            placeholder={t('notificationSms.placeholders.linkCode' as never)}
            errorText={linkCodeError}
            disabled={readOnly}
          />
        )}
        {canUseWorkflowValues && linkCodeError ? (
          <p className="text-xs font-medium text-destructive">{linkCodeError}</p>
        ) : null}
        <p className="text-[10px] leading-4.5 text-muted-foreground">
          {t('notificationSms.help.linkCode' as never)}
        </p>
      </div>

      <NotificationSMSPreview
        notificationTitle={value.notificationTitle}
        linkSuffix={value.linkCode}
        previewTemplate={previewTemplate}
      />
    </div>
  );
}

function getCommonMessage(
  t: ReturnType<typeof useT<'common'>>,
  key: string,
  fallback: string
): string {
  const message = t(key as never);
  return message === key || message === `common.${key}` ? fallback : message;
}
