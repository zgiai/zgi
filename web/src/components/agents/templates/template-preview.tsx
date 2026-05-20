'use client';

import Link from 'next/link';
import { AlertTriangle, ArrowLeft, CheckCircle2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import type { AgentTemplateRuntimeStatus } from './types';

interface TemplatePreviewProps {
  title: string;
  description: string;
  iconLabel: string;
  kindLabel: string;
  complexityLabel: string;
  runtimeStatusLabel: string;
  runtimeStatus: AgentTemplateRuntimeStatus;
  requirements: string[];
  setupRequirements: string[];
  categories: string[];
  runHint: string;
  recommendedPrompts: Array<{ href: string; title: string }>;
  isCreating: boolean;
  disabled: boolean;
  confirmDisabled?: boolean;
  labels: {
    back: string;
    label: string;
    overview: string;
    runtimeCheck: string;
    dependencies: string;
    categories: string;
    recommendedPrompts: string;
    afterCreateTitle: string;
    afterCreateDescription: string;
    readyTitle: string;
    readyDescription: string;
    setupTitle: string;
    setupDescription: string;
    confirm: string;
  };
  onBack: () => void;
  onConfirm: () => void;
}

export function TemplatePreview({
  title,
  description,
  iconLabel,
  kindLabel,
  complexityLabel,
  runtimeStatusLabel,
  runtimeStatus,
  requirements,
  setupRequirements,
  categories,
  runHint,
  recommendedPrompts,
  isCreating,
  disabled,
  confirmDisabled,
  labels,
  onBack,
  onConfirm,
}: TemplatePreviewProps) {
  const requiresSetup = runtimeStatus === 'requires-setup';
  const StatusIcon = requiresSetup ? AlertTriangle : CheckCircle2;
  const setupItems = setupRequirements.length > 0 ? setupRequirements : requirements;

  return (
    <section className="space-y-3">
      <button
        type="button"
        disabled={disabled}
        onClick={onBack}
        className="inline-flex h-8 items-center gap-1.5 rounded-md px-2 text-xs font-medium text-muted-foreground transition-colors hover:bg-background hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 disabled:cursor-not-allowed disabled:opacity-60"
      >
        <ArrowLeft className="size-3.5" />
        {labels.back}
      </button>

      <div className="overflow-hidden rounded-lg border bg-background shadow-sm">
        <div className="border-b px-4 py-4">
          <div className="flex items-start gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-muted/40 text-sm font-semibold text-foreground">
              {iconLabel}
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-xs font-semibold text-muted-foreground">{labels.label}</div>
              <h2 className="mt-1 text-base font-semibold leading-6 text-foreground">{title}</h2>
              <p className="mt-1 max-w-3xl text-xs leading-5 text-muted-foreground">
                {description}
              </p>
              <div className="mt-3 flex flex-wrap gap-1.5">
                <span className="rounded border bg-muted/40 px-2 py-0.5 text-[11px] font-semibold text-muted-foreground">
                  {kindLabel}
                </span>
                <span className="rounded border bg-background px-2 py-0.5 text-[11px] font-semibold text-muted-foreground">
                  {complexityLabel}
                </span>
                <span
                  className={cn(
                    'rounded border px-2 py-0.5 text-[11px] font-semibold',
                    requiresSetup
                      ? 'border-amber-200 bg-amber-50 text-amber-800'
                      : 'border-emerald-200 bg-emerald-50 text-emerald-700'
                  )}
                >
                  {runtimeStatusLabel}
                </span>
              </div>
            </div>
          </div>
        </div>

        <div className="grid gap-4 px-4 py-4 lg:grid-cols-[minmax(0,1fr)_280px]">
          <div className="space-y-4">
            <section>
              <h3 className="text-sm font-semibold text-foreground">{labels.overview}</h3>
              <p className="mt-2 text-xs leading-5 text-muted-foreground">{description}</p>
            </section>

            <section className="rounded-lg border bg-muted/15 px-3 py-3">
              <h3 className="text-sm font-semibold text-foreground">{labels.afterCreateTitle}</h3>
              <p className="mt-1.5 text-xs leading-5 text-muted-foreground">
                {labels.afterCreateDescription}
              </p>
            </section>

            <section>
              <h3 className="text-sm font-semibold text-foreground">{labels.dependencies}</h3>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {requirements.map(requirement => (
                  <span
                    key={requirement}
                    className="rounded border bg-background px-2 py-1 text-[11px] font-medium leading-4 text-muted-foreground"
                  >
                    {requirement}
                  </span>
                ))}
              </div>
            </section>

            {recommendedPrompts.length > 0 ? (
              <section>
                <h3 className="text-sm font-semibold text-foreground">
                  {labels.recommendedPrompts}
                </h3>
                <div className="mt-2 flex flex-wrap gap-2">
                  {recommendedPrompts.map(prompt => (
                    <Link
                      key={prompt.href}
                      href={prompt.href}
                      className="rounded border bg-background px-2 py-1 text-[11px] font-medium leading-4 text-primary hover:bg-primary/5"
                    >
                      {prompt.title}
                    </Link>
                  ))}
                </div>
              </section>
            ) : null}
          </div>

          <aside className="space-y-3">
            <section
              className={cn(
                'rounded-lg border px-3 py-3',
                requiresSetup
                  ? 'border-amber-200 bg-amber-50/70 text-amber-950'
                  : 'border-emerald-200 bg-emerald-50/70 text-emerald-950'
              )}
            >
              <div className="mb-2 text-[11px] font-semibold text-current/70">
                {labels.runtimeCheck}
              </div>
              <div className="flex items-start gap-2">
                <StatusIcon className="mt-0.5 size-4 shrink-0" />
                <div>
                  <h3 className="text-sm font-semibold">
                    {requiresSetup ? labels.setupTitle : labels.readyTitle}
                  </h3>
                  <p className="mt-1 text-xs leading-5 opacity-80">
                    {requiresSetup ? labels.setupDescription : labels.readyDescription}
                  </p>
                </div>
              </div>
              <div className="mt-3 rounded-md border border-current/15 bg-background/70 px-2.5 py-2 text-xs leading-5">
                {runHint}
              </div>
              {requiresSetup ? (
                <div className="mt-2 flex flex-wrap gap-1">
                  {setupItems.map(requirement => (
                    <span
                      key={requirement}
                      className="rounded border border-current/15 bg-background/70 px-1.5 py-0.5 text-[11px] font-medium leading-4"
                    >
                      {requirement}
                    </span>
                  ))}
                </div>
              ) : null}
            </section>

            <section className="rounded-lg border bg-background px-3 py-3">
              <h3 className="text-sm font-semibold text-foreground">{labels.categories}</h3>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {categories.map(category => (
                  <span
                    key={category}
                    className="rounded border bg-muted/30 px-2 py-0.5 text-[11px] font-medium leading-4 text-muted-foreground"
                  >
                    {category}
                  </span>
                ))}
              </div>
            </section>
          </aside>
        </div>

        <div className="flex flex-col-reverse gap-2 border-t bg-muted/10 px-4 py-3 sm:flex-row sm:justify-end">
          <Button variant="outline" disabled={disabled} onClick={onBack}>
            {labels.back}
          </Button>
          <Button loading={isCreating} disabled={confirmDisabled ?? disabled} onClick={onConfirm}>
            {labels.confirm}
          </Button>
        </div>
      </div>
    </section>
  );
}
