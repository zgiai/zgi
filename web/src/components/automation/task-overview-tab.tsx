'use client';

import * as React from 'react';
import { BellRing, Eye, Mail, Smartphone, Workflow } from 'lucide-react';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { actionTypeRegistry, channelTypeRegistry } from './registry';
import { safeJson, summarizeRecipients } from './utils';
import type { TaskDetailViewData } from './types';

interface TaskOverviewTabProps {
  taskDetail: TaskDetailViewData;
  showUnsupportedHint: boolean;
}

function JsonBlock({ value }: { value: unknown }) {
  const json = safeJson(value);

  if (!json) {
    return null;
  }

  return (
    <pre className="overflow-x-auto rounded-xl border border-border/70 bg-muted/30 p-4 text-xs leading-6 text-muted-foreground">
      {json}
    </pre>
  );
}

/**
 * @component TaskOverviewTab
 * @category Feature
 * @status Stable
 * @description Read-only overview for a scheduled task, including schedule, action summaries, and raw config fallbacks.
 * @usage Render inside the detail panel's overview tab.
 */
export function TaskOverviewTab({ taskDetail, showUnsupportedHint }: TaskOverviewTabProps) {
  const t = useT('automation');
  const tCommon = useT('common');
  const translate = React.useCallback(
    (key: string, values?: Record<string, string | number>) => t(key as never, values as never),
    [t]
  );
  const [selectedActionIndex, setSelectedActionIndex] = React.useState<number | null>(null);
  const selectedAction =
    selectedActionIndex === null ? null : (taskDetail.actions[selectedActionIndex] ?? null);

  return (
    <div className="space-y-4">
      {showUnsupportedHint ? (
        <Card className="border-warning/30 bg-warning/5" padding="none">
          <CardContent className="p-4 text-sm leading-6 text-muted-foreground">
            {t('detail.unsupportedReadonly')}
          </CardContent>
        </Card>
      ) : null}

      <Card className="border-border/70" padding="none">
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            <BellRing className="size-4 text-primary" />
            {t('actions.title')}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2.5">
          {taskDetail.actions.map((action, index) => {
            const actionMeta =
              actionTypeRegistry[String((action as { action_type?: string }).action_type ?? '')];

            return (
              <div
                key={action.id || `task-action-${index}`}
                className="flex items-center justify-between gap-3 rounded-xl border border-border/70 bg-muted/15 px-3.5 py-2.5"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="break-words text-sm font-medium text-foreground">
                      {actionMeta ? translate(actionMeta.labelKey) : t('fallback.unknownAction')}
                    </span>
                    {action.enabled === false ? (
                      <Badge variant="secondary">{t('actions.disabled')}</Badge>
                    ) : null}
                  </div>
                </div>

                <Button
                  variant="ghost"
                  size="sm"
                  isIcon
                  onClick={() => setSelectedActionIndex(index)}
                  aria-label={tCommon('view')}
                  title={tCommon('view')}
                  className="h-8 w-8 rounded-lg"
                >
                  <Eye className="size-4" />
                </Button>
              </div>
            );
          })}
        </CardContent>
      </Card>

      {showUnsupportedHint ? (
        <Card className="border-border/70" padding="none">
          <CardHeader className="pb-3">
            <CardTitle className="text-base">{t('detail.rawConfig')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <JsonBlock value={taskDetail.task.schedule_config} />
            {taskDetail.actions.map((action, index) => (
              <JsonBlock key={`raw-action-${action.id || index}`} value={action.config} />
            ))}
          </CardContent>
        </Card>
      ) : null}

      <Dialog
        open={selectedAction !== null}
        onOpenChange={open => {
          if (!open) {
            setSelectedActionIndex(null);
          }
        }}
      >
        <DialogContent size="lg" className="rounded-2xl border-border bg-background p-0">
          {selectedAction ? (
            <>
              <DialogHeader className="border-b border-border pb-4">
                <DialogTitle>{t('actions.detailTitle')}</DialogTitle>
                <DialogDescription className="text-[11px] leading-5 text-muted-foreground">
                  {t('actions.detailDescription')}
                </DialogDescription>
              </DialogHeader>
              <DialogBody className="space-y-4 py-5">
                <div className="grid gap-3 md:grid-cols-2">
                  <div className="rounded-2xl border border-border/70 bg-muted/20 p-4">
                    <div className="mb-2 text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                      {t('actions.type')}
                    </div>
                    <p className="text-sm font-medium text-foreground">
                      {actionTypeRegistry[selectedAction.action_type]
                        ? translate(actionTypeRegistry[selectedAction.action_type].labelKey)
                        : t('fallback.unknownAction')}
                    </p>
                  </div>
                  {selectedAction.action_type === 'send_notification' ? (
                    <div className="rounded-2xl border border-border/70 bg-muted/20 p-4">
                      <div className="mb-2 text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                        {t('actions.channel')}
                      </div>
                      <p className="text-sm font-medium text-foreground">
                        {channelTypeRegistry[selectedAction.config.channel_type]
                          ? translate(
                              channelTypeRegistry[selectedAction.config.channel_type].labelKey
                            )
                          : t('fallback.unknownChannel')}
                      </p>
                    </div>
                  ) : (
                    <div className="rounded-2xl border border-border/70 bg-muted/20 p-4">
                      <div className="mb-2 text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                        {t('actions.versionStrategy')}
                      </div>
                      <p className="text-sm font-medium text-foreground">
                        {selectedAction.config.workflow_ref.version_strategy === 'pinned'
                          ? t('actions.versionPinned')
                          : t('actions.versionLatestPublished')}
                      </p>
                    </div>
                  )}
                </div>

                {selectedAction.action_type === 'send_notification' ? (
                  <>
                    <div className="rounded-2xl border border-border/70 bg-background p-4">
                      <div className="mb-2 flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                        {selectedAction.config.channel_type === 'sms' ? (
                          <Smartphone className="size-3.5" />
                        ) : (
                          <Mail className="size-3.5" />
                        )}
                        {t('actions.recipients')}
                      </div>
                      <p className="break-all text-sm leading-6 text-foreground">
                        {summarizeRecipients(selectedAction.config.to ?? [], translate)}
                      </p>
                    </div>

                    {selectedAction.config.channel_type === 'sms' ? (
                      <>
                        <div className="rounded-2xl border border-border/70 bg-background p-4">
                          <div className="mb-2 flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                            <Workflow className="size-3.5" />
                            {t('actions.smsNotificationTitle')}
                          </div>
                          <p className="break-words text-sm leading-6 text-foreground">
                            {selectedAction.config.template_params.notification_title ||
                              t('misc.notAvailable')}
                          </p>
                        </div>
                        <div className="rounded-2xl border border-border/70 bg-background p-4">
                          <div className="mb-2 text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                            {t('actions.smsLinkCode')}
                          </div>
                          <p className="break-all text-sm leading-6 text-foreground">
                            {selectedAction.config.template_params.link_suffix ||
                              t('misc.notAvailable')}
                          </p>
                        </div>
                      </>
                    ) : (
                      <>
                        <div className="rounded-2xl border border-border/70 bg-background p-4">
                          <div className="mb-2 flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                            <Workflow className="size-3.5" />
                            {t('actions.subject')}
                          </div>
                          <p className="break-words text-sm leading-6 text-foreground">
                            {selectedAction.config.subject || t('misc.notAvailable')}
                          </p>
                        </div>

                        <div className="rounded-2xl border border-border/70 bg-background p-4">
                          <div className="mb-2 flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                            <span>{t('actions.content')}</span>
                            <Badge
                              variant="outline"
                              className="rounded-full border-border/70 text-[10px] font-medium normal-case tracking-normal text-muted-foreground"
                            >
                              {selectedAction.config.body_type === 'text/plain'
                                ? t('actions.bodyTypePlainText')
                                : t('actions.bodyTypeHtml')}
                            </Badge>
                          </div>
                          <p className="whitespace-pre-wrap break-words text-sm leading-6 text-foreground">
                            {selectedAction.config.body || t('misc.notAvailable')}
                          </p>
                        </div>
                      </>
                    )}
                  </>
                ) : (
                  <>
                    <div className="rounded-2xl border border-border/70 bg-background p-4">
                      <div className="mb-2 flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                        <Workflow className="size-3.5" />
                        {t('actions.targetAgent')}
                      </div>
                      <p className="break-all text-sm leading-6 text-foreground">
                        {selectedAction.config.workflow_ref.agent_id || t('misc.notAvailable')}
                      </p>
                    </div>

                    {selectedAction.config.workflow_ref.version_strategy === 'pinned' ? (
                      <div className="rounded-2xl border border-border/70 bg-background p-4">
                        <div className="mb-2 text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                          {t('actions.versionUuid')}
                        </div>
                        <p className="break-all text-sm leading-6 text-foreground">
                          {selectedAction.config.workflow_ref.version_uuid ||
                            t('misc.notAvailable')}
                        </p>
                      </div>
                    ) : null}

                    <div className="rounded-2xl border border-border/70 bg-background p-4">
                      <div className="mb-2 text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                        {t('actions.timeoutSeconds')}
                      </div>
                      <p className="text-sm leading-6 text-foreground">
                        {selectedAction.config.execution?.timeout_seconds ?? t('misc.notAvailable')}
                      </p>
                    </div>

                    <div className="rounded-2xl border border-border/70 bg-background p-4">
                      <div className="mb-2 text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
                        {t('actions.workflowInputs')}
                      </div>
                      <JsonBlock value={selectedAction.config.inputs ?? {}} />
                    </div>
                  </>
                )}

                {showUnsupportedHint ? <JsonBlock value={selectedAction.config} /> : null}
              </DialogBody>
            </>
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}
