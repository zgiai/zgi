'use client';

import * as React from 'react';
import { AlertCircle, Sparkles } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

interface TaskEditorAIDraftCardProps {
  value: string;
  disabled: boolean;
  loading: boolean;
  summary?: string;
  warnings: string[];
  missingFields: string[];
  onChange: (value: string) => void;
  onGenerate: () => void;
}

function formatMissingFieldLabel(
  field: string,
  t: (key: string, values?: Record<string, string | number>) => string
): string {
  const actionMatch = /^actions\.(\d+)\.(.+)$/.exec(field);
  const actionIndex = actionMatch?.[1] ?? '';
  const actionField = actionMatch?.[2] ?? '';

  if (actionField) {
    const mappedActionField =
      {
        to: 'recipients',
        subject: 'subject',
        body: 'content',
        notification_title: 'smsTitle',
        workflow_agent_id: 'workflow',
        sms_link_suffix: 'smsLinkCode',
      }[actionField] ?? 'action';

    return t(`aiDraft.fields.${mappedActionField}` as never, { index: actionIndex });
  }

  const mappedField =
    {
      name: 'name',
      actions: 'actions',
      'schedule.run_at': 'runAt',
      'schedule.cron_expr': 'cron',
      'schedule.timezone': 'timezone',
    }[field] ?? 'unknown';

  return t(`aiDraft.fields.${mappedField}` as never);
}

export function TaskEditorAIDraftCard({
  value,
  disabled,
  loading,
  summary,
  warnings,
  missingFields,
  onChange,
  onGenerate,
}: TaskEditorAIDraftCardProps) {
  const t = useT('automation');
  const examples = [
    t('aiDraft.examples.weekly'),
    t('aiDraft.examples.once'),
    t('aiDraft.examples.daily'),
  ];
  const translateField = t as unknown as (
    key: string,
    values?: Record<string, string | number>
  ) => string;
  const formattedMissingFields = missingFields.map(field =>
    formatMissingFieldLabel(field, translateField)
  );
  const canGenerate = value.trim().length > 0 && !disabled && !loading;

  return (
    <Card className="border-primary/20 bg-primary/[0.03]" padding="none">
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-3">
          <div className="space-y-1">
            <CardTitle className="flex items-center gap-2 text-base">
              <Sparkles className="size-4 text-primary" />
              {t('aiDraft.title')}
            </CardTitle>
            <p className="text-xs leading-5 text-muted-foreground">{t('aiDraft.description')}</p>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <Textarea
          value={value}
          onChange={event => onChange(event.target.value)}
          placeholder={t('aiDraft.placeholder')}
          disabled={disabled || loading}
          className="min-h-[92px] bg-background"
        />
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex flex-wrap gap-2">
            {examples.map(example => (
              <Button
                key={example}
                type="button"
                variant="outline"
                size="sm"
                className="h-7 rounded-full bg-background px-3 text-xs font-normal"
                disabled={disabled || loading}
                onClick={() => onChange(example)}
              >
                {example}
              </Button>
            ))}
          </div>
          <Button
            type="button"
            size="sm"
            onClick={event => {
              event.preventDefault();
              onGenerate();
            }}
            loading={loading}
            disabled={!canGenerate}
          >
            <Sparkles className="size-4" />
            {t('aiDraft.generate')}
          </Button>
        </div>

        {summary ? (
          <div className="rounded-lg border border-border/70 bg-background px-3 py-2 text-xs leading-5 text-muted-foreground">
            {summary}
          </div>
        ) : null}

        {warnings.length > 0 || missingFields.length > 0 ? (
          <Alert className="border-amber-300/70 bg-amber-50/70 text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">
            <AlertCircle className="size-4" />
            <AlertTitle>{t('aiDraft.reviewTitle')}</AlertTitle>
            <AlertDescription className="space-y-2">
              {missingFields.length > 0 ? (
                <div>
                  <span className="font-medium">{t('aiDraft.missingFields')}</span>
                  <span className="ml-1">{formattedMissingFields.join(' / ')}</span>
                </div>
              ) : null}
              {warnings.length > 0 ? (
                <ul className={cn('space-y-1', missingFields.length > 0 && 'pt-1')}>
                  {warnings.map(warning => (
                    <li key={warning}>{warning}</li>
                  ))}
                </ul>
              ) : null}
            </AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
    </Card>
  );
}
