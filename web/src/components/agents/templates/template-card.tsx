'use client';

import type { LucideIcon } from 'lucide-react';
import {
  AlertTriangle,
  ArrowRight,
  Bot,
  CheckCircle2,
  Loader2,
  MessageSquareText,
  Workflow,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import type { AgentTemplate, AgentTemplateKind, AgentTemplateRuntimeStatus } from './types';

const KIND_ICON: Record<AgentTemplateKind, LucideIcon> = {
  chatflow: MessageSquareText,
  workflow: Workflow,
  agent: Bot,
};

interface TemplateCardProps {
  template: AgentTemplate;
  title: string;
  description: string;
  kindLabel: string;
  complexityLabel: string;
  runtimeStatusLabel: string;
  runtimeStatus: AgentTemplateRuntimeStatus;
  requirementSummary: string;
  runHint: string;
  isCreating: boolean;
  disabled: boolean;
  onSelect: (template: AgentTemplate) => void;
}

export function TemplateCard({
  template,
  title,
  description,
  kindLabel,
  complexityLabel,
  runtimeStatusLabel,
  runtimeStatus,
  requirementSummary,
  runHint,
  isCreating,
  disabled,
  onSelect,
}: TemplateCardProps) {
  const KindIcon = KIND_ICON[template.kind];
  const requiresSetup = runtimeStatus === 'requires-setup';
  const StatusIcon = requiresSetup ? AlertTriangle : CheckCircle2;

  return (
    <button
      type="button"
      aria-busy={isCreating}
      disabled={disabled}
      onClick={() => onSelect(template)}
      className={cn(
        'group relative flex min-h-[178px] w-full flex-col rounded-lg border bg-background p-3 text-left shadow-sm transition-all',
        'hover:-translate-y-0.5 hover:border-primary/30 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30',
        disabled && 'cursor-not-allowed opacity-60 hover:translate-y-0 hover:shadow-sm'
      )}
    >
      <div className="flex items-start gap-2.5">
        <div
          className={cn(
            'relative flex size-8 shrink-0 items-center justify-center rounded-md border border-border bg-muted/40 text-[11px] font-semibold text-foreground'
          )}
        >
          {template.iconLabel}
          <span className="absolute -bottom-1 -right-1 flex size-4 items-center justify-center rounded bg-background text-primary shadow-sm ring-1 ring-border">
            <KindIcon className="size-2.5" />
          </span>
        </div>
        <div className="min-w-0">
          <div className="line-clamp-2 text-sm font-semibold leading-[18px] text-foreground">
            {title}
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-1">
            <span className="rounded border bg-muted/40 px-1.5 py-px text-[10px] font-semibold uppercase leading-4 tracking-normal text-muted-foreground">
              {kindLabel}
            </span>
            <span className="rounded border bg-background px-1.5 py-px text-[10px] font-semibold leading-4 text-muted-foreground">
              {complexityLabel}
            </span>
            <span
              className={cn(
                'rounded border px-1.5 py-px text-[10px] font-semibold leading-4',
                !requiresSetup
                  ? 'border-emerald-200 bg-emerald-50 text-emerald-700'
                  : 'border-amber-200 bg-amber-50 text-amber-800'
              )}
            >
              {runtimeStatusLabel}
            </span>
          </div>
        </div>
      </div>
      <p className="mt-2.5 line-clamp-2 text-xs leading-[18px] text-muted-foreground">
        {description}
      </p>
      {requirementSummary ? (
        <p className="mt-1.5 line-clamp-1 text-[11px] leading-4 text-muted-foreground">
          {requirementSummary}
        </p>
      ) : null}
      <div
        className={cn(
          'mt-auto flex min-h-8 items-center gap-1.5 rounded-md border px-2 py-1 text-[11px] leading-4',
          requiresSetup
            ? 'border-amber-200 bg-amber-50/80 text-amber-900'
            : 'border-emerald-200 bg-emerald-50/70 text-emerald-800'
        )}
      >
        <StatusIcon className="size-3 shrink-0" />
        <span className="line-clamp-1 min-w-0 flex-1">{runHint}</span>
        <ArrowRight className="size-3 shrink-0 opacity-60 transition-transform group-hover:translate-x-0.5" />
      </div>
      {isCreating ? (
        <div className="absolute inset-0 flex items-center justify-center rounded-lg bg-background/70 backdrop-blur-[1px]">
          <Loader2 className="size-5 animate-spin text-primary" />
        </div>
      ) : null}
    </button>
  );
}
