'use client';

import { CheckCircle2, Circle, CircleSlash, ListChecks, Loader2 } from 'lucide-react';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type { AIChatPlanStep } from '@/services/types/aichat';

function StepIcon({ status, active }: { status: AIChatPlanStep['status']; active: boolean }) {
  if (status === 'completed') return <CheckCircle2 className="size-3.5 shrink-0 text-emerald-600 dark:text-emerald-400" />;
  if (status === 'skipped') return <CircleSlash className="size-3.5 shrink-0 text-muted-foreground" />;
  if (status === 'in_progress') {
    return active ? (
      <Loader2 className="size-3.5 shrink-0 animate-spin text-primary" />
    ) : (
      <Circle className="size-3.5 shrink-0 text-primary" />
    );
  }
  return <Circle className="size-3.5 shrink-0 text-muted-foreground/60" />;
}

/**
 * The model's own checklist for the turn (ADR 0049): what it plans to do and
 * how far it has got. A step still in progress spins only while the turn runs.
 */
export function AIChatPlanChecklist({ steps, active }: { steps: AIChatPlanStep[]; active: boolean }) {
  const t = useT('webapp');
  if (steps.length === 0) return null;
  const done = steps.filter(step => step.status === 'completed' || step.status === 'skipped').length;
  return (
    <div className="mb-3 rounded-md border border-muted-foreground/15 bg-muted/20 px-3 py-2 text-sm">
      <div className="mb-1.5 flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <ListChecks className="size-3.5" />
        <span>{t('consoleChat.planTitle')}</span>
        <span className="font-normal">{t('consoleChat.planProgress', { done, total: steps.length })}</span>
      </div>
      <ol className="space-y-1">
        {steps.map((step, index) => (
          <li key={step.id ?? index} className="flex min-w-0 items-start gap-2">
            <span className="mt-1">
              <StepIcon status={step.status} active={active} />
            </span>
            <span
              className={cn(
                'min-w-0 break-words leading-6',
                step.status === 'completed' && 'text-muted-foreground',
                step.status === 'skipped' && 'text-muted-foreground line-through'
              )}
            >
              {step.step}
            </span>
          </li>
        ))}
      </ol>
    </div>
  );
}
