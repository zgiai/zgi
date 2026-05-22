'use client';

import * as React from 'react';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { OptionEditor } from '@/components/ui/option-editor';
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
import {
  getDefaultNotificationSMSTemplateKey,
  getNotificationSMSTemplates,
  NOTIFICATION_SMS_TEMPLATE,
  type NotificationSMSTemplate,
  type NotificationSMSTemplateParam,
} from '@/lib/features/notification-sms';
import { cn } from '@/lib/utils';
import { useAuthStore } from '@/store/auth-store';
import type { NotificationSMSDraft, NotificationSMSErrors } from './types';
import { NotificationSMSPreview } from './notification-sms-preview';
import { isWorkflowValueToken } from './validation';

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
  const templates = React.useMemo(
    () => getNotificationSMSTemplates(systemFeatures),
    [systemFeatures]
  );
  const defaultTemplateKey = getDefaultNotificationSMSTemplateKey(systemFeatures);
  const selectedTemplate =
    templates.find(template => template.key === value.template) ??
    templates.find(template => template.key === defaultTemplateKey) ??
    templates[0];
  const selectedTemplateKey = selectedTemplate?.key ?? value.template;
  const selectedTemplateParams = selectedTemplate?.params ?? [];
  const workflowNodeId = nodeId;
  const canUseWorkflowValues = typeof workflowNodeId === 'string' && workflowNodeId.length > 0;
  const recipientPlaceholder =
    recipientMode === 'single'
      ? t('notificationSms.placeholders.recipientSingle' as never)
      : undefined;

  const setRecipients = React.useCallback(
    (recipients: string[]) => {
      onChange({
        ...value,
        recipients: recipientMode === 'single' ? [recipients[0] ?? ''] : recipients,
      });
    },
    [onChange, recipientMode, value]
  );
  const setTemplate = React.useCallback(
    (templateKey: string) => {
      const nextTemplate = templates.find(template => template.key === templateKey);
      onChange({
        ...value,
        template: templateKey,
        templateParams: buildTemplateParams(nextTemplate, value.templateParams),
      });
    },
    [onChange, templates, value]
  );
  const setTemplateParam = React.useCallback(
    (key: string, nextValue: string) => {
      onChange({
        ...value,
        template: selectedTemplateKey,
        templateParams: {
          ...value.templateParams,
          [key]: nextValue,
        },
      });
    },
    [onChange, selectedTemplateKey, value]
  );

  return (
    <div className={cn('space-y-3', className)}>
      <div className="space-y-1.5">
        <Label className="text-[13px] font-medium">
          {t('notificationSms.fields.template' as never)}
        </Label>
        <Select value={selectedTemplateKey} onValueChange={setTemplate} disabled={readOnly}>
          <SelectTrigger className="h-9 rounded-xl border-border bg-background shadow-none hover:border-border">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {templates.map(template => (
              <SelectItem key={template.key} value={template.key}>
                {getTemplateLabel(t, template)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {errors?.template ? (
          <p className="text-xs font-medium text-destructive">{errors.template}</p>
        ) : null}
      </div>

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

      {selectedTemplateParams.map(param => {
        const paramValue = value.templateParams[param.key] ?? '';
        const paramError =
          errors?.templateParams?.[param.key] ??
          getLocalParamError(t, param, paramValue, canUseWorkflowValues);
        return (
          <div key={param.key} className="space-y-1.5 border-t border-border/50 pt-2.5">
            <Label className="text-[13px] font-medium">{getParamLabel(t, param)}</Label>
            {canUseWorkflowValues ? (
              <WorkflowValueEditor
                nodeId={workflowNodeId}
                portalRoot={portalRoot}
                value={paramValue}
                onChange={nextValue => setTemplateParam(param.key, nextValue)}
                readOnly={readOnly}
                placeholder={getParamPlaceholder(t, param)}
                className="w-full"
                editorClassName="min-h-[36px] rounded-xl border-border bg-background px-3 py-2 shadow-none hover:border-border focus-within:border-primary/70"
              />
            ) : (
              <Input
                value={paramValue}
                onChange={event => setTemplateParam(param.key, event.target.value)}
                placeholder={getParamPlaceholder(t, param)}
                errorText={paramError}
                disabled={readOnly}
              />
            )}
            {canUseWorkflowValues && paramError ? (
              <p className="text-xs font-medium text-destructive">{paramError}</p>
            ) : null}
            {param.key === 'link_code' ? (
              <p className="text-[10px] leading-4.5 text-muted-foreground">
                {t('notificationSms.help.linkCode' as never)}
              </p>
            ) : null}
          </div>
        );
      })}

      <NotificationSMSPreview template={selectedTemplate} templateParams={value.templateParams} />
    </div>
  );
}

function buildTemplateParams(
  template: NotificationSMSTemplate | undefined,
  currentParams: Record<string, string>
): Record<string, string> {
  const params = template?.params ?? [];
  if (params.length === 0) {
    return { ...currentParams };
  }

  return params.reduce<Record<string, string>>((next, param) => {
    next[param.key] = currentParams[param.key] ?? '';
    return next;
  }, {});
}

function getLocalParamError(
  t: ReturnType<typeof useT<'common'>>,
  param: NotificationSMSTemplateParam,
  value: string,
  allowWorkflowToken: boolean
): string | undefined {
  const trimmed = value.trim();
  const label = getParamLabel(t, param);
  if (!trimmed) {
    if (param.required) {
      return t(
        'notificationSms.validation.paramRequired' as never,
        {
          label,
        } as never
      );
    }
    return undefined;
  }
  if (allowWorkflowToken && isWorkflowValueToken(trimmed)) {
    return undefined;
  }
  if (param.max_length && [...trimmed].length > param.max_length) {
    return t(
      'notificationSms.validation.paramTooLong' as never,
      {
        label,
        max: param.max_length,
      } as never
    );
  }
  if (param.pattern) {
    try {
      if (!new RegExp(param.pattern).test(trimmed)) {
        return t(
          'notificationSms.validation.paramInvalid' as never,
          {
            label,
          } as never
        );
      }
    } catch {
      return undefined;
    }
  }
  return undefined;
}

function getParamPlaceholder(
  t: ReturnType<typeof useT<'common'>>,
  param: NotificationSMSTemplateParam
): string {
  if (param.key === 'notification_title') {
    return t('notificationSms.placeholders.notificationTitle' as never);
  }
  if (param.key === 'link_code') {
    return t('notificationSms.placeholders.linkCode' as never);
  }
  return t(
    'notificationSms.placeholders.param' as never,
    {
      label: getParamLabel(t, param),
    } as never
  );
}

function getTemplateLabel(
  t: ReturnType<typeof useT<'common'>>,
  template: NotificationSMSTemplate
): string {
  if (template.key === NOTIFICATION_SMS_TEMPLATE) {
    return t('notificationSms.templates.pendingActionNotification' as never);
  }
  return template.name || template.key;
}

function getParamLabel(
  t: ReturnType<typeof useT<'common'>>,
  param: NotificationSMSTemplateParam
): string {
  if (param.key === 'notification_title') {
    return t('notificationSms.params.notificationTitle' as never);
  }
  if (param.key === 'link_code') {
    return t('notificationSms.params.linkCode' as never);
  }
  return param.label || param.key;
}
