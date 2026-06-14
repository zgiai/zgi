'use client';

import type { HTMLAttributes, ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';
import { AlertCircle, CheckCircle2, Circle, ExternalLink, Info, Loader2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import type {
  OperationCardAction,
  OperationCardMetaItem,
  OperationCardTone,
} from '@/components/aichat/operation-cards/types';

type BadgeVariant =
  | 'default'
  | 'secondary'
  | 'destructive'
  | 'outline'
  | 'subtle'
  | 'success'
  | 'warning'
  | 'info';

const toneBadgeVariant: Record<OperationCardTone, BadgeVariant> = {
  neutral: 'secondary',
  info: 'info',
  success: 'success',
  warning: 'warning',
  destructive: 'destructive',
};

const toneTextClassName: Record<OperationCardTone, string> = {
  neutral: 'text-muted-foreground',
  info: 'text-info',
  success: 'text-success',
  warning: 'text-warning',
  destructive: 'text-destructive',
};

const toneSoftClassName: Record<OperationCardTone, string> = {
  neutral: 'border-border bg-muted/30',
  info: 'border-info/20 bg-info/5',
  success: 'border-success/20 bg-success/5',
  warning: 'border-warning/25 bg-warning/10',
  destructive: 'border-destructive/30 bg-destructive/10',
};

export function getToneTextClassName(tone: OperationCardTone = 'neutral') {
  return toneTextClassName[tone];
}

export function getToneSoftClassName(tone: OperationCardTone = 'neutral') {
  return toneSoftClassName[tone];
}

export function getToneBadgeVariant(tone: OperationCardTone = 'neutral') {
  return toneBadgeVariant[tone];
}

export function getToneIcon(tone: OperationCardTone): LucideIcon {
  if (tone === 'success') return CheckCircle2;
  if (tone === 'warning' || tone === 'destructive') return AlertCircle;
  if (tone === 'info') return Info;
  return Circle;
}

interface OperationStatusBadgeProps {
  label: ReactNode;
  tone?: OperationCardTone;
  loading?: boolean;
}

export function OperationStatusBadge({
  label,
  tone = 'neutral',
  loading = false,
}: OperationStatusBadgeProps) {
  const Icon = loading ? Loader2 : getToneIcon(tone);

  return (
    <Badge variant={getToneBadgeVariant(tone)} className="max-w-full">
      <Icon className={cn('size-3 shrink-0', loading && 'animate-spin')} />
      <span className="min-w-0 truncate">{label}</span>
    </Badge>
  );
}

interface OperationCardShellProps extends HTMLAttributes<HTMLDivElement> {
  compact?: boolean;
  children: ReactNode;
}

export function OperationCardShell({
  compact = false,
  className,
  children,
  ...props
}: OperationCardShellProps) {
  return (
    <Card
      className={cn(
        'w-full max-w-3xl overflow-hidden rounded-md border-border bg-background/95 shadow-sm',
        compact ? 'text-xs' : 'text-sm',
        className
      )}
      {...props}
    >
      <div className={cn(compact ? 'space-y-3 p-3' : 'space-y-4 p-4')}>{children}</div>
    </Card>
  );
}

interface OperationCardHeaderProps {
  icon: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  eyebrow?: ReactNode;
  badge?: ReactNode;
  compact?: boolean;
}

export function OperationCardHeader({
  icon,
  title,
  description,
  eyebrow,
  badge,
  compact = false,
}: OperationCardHeaderProps) {
  return (
    <div className="flex min-w-0 items-start gap-3">
      <div
        className={cn(
          'flex shrink-0 items-center justify-center rounded-md border bg-muted/30 text-muted-foreground',
          compact ? 'size-8' : 'size-9'
        )}
      >
        {icon}
      </div>
      <div className="min-w-0 flex-1 space-y-1">
        {eyebrow || badge ? (
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            {eyebrow ? (
              <span className="min-w-0 truncate text-xs font-medium text-muted-foreground">
                {eyebrow}
              </span>
            ) : null}
            {badge}
          </div>
        ) : null}
        <div className={cn('break-words font-semibold text-foreground', compact ? 'text-sm' : '')}>
          {title}
        </div>
        {description ? (
          <div className="whitespace-pre-wrap break-words text-xs leading-relaxed text-muted-foreground">
            {description}
          </div>
        ) : null}
      </div>
    </div>
  );
}

interface OperationMetaGridProps {
  items?: OperationCardMetaItem[];
  compact?: boolean;
}

export function OperationMetaGrid({ items, compact = false }: OperationMetaGridProps) {
  const visibleItems = items?.filter(item => item.label || item.value || item.description) ?? [];
  if (visibleItems.length === 0) return null;

  return (
    <dl className={cn('grid gap-2', compact ? 'sm:grid-cols-2' : 'sm:grid-cols-3')}>
      {visibleItems.map(item => (
        <div
          key={item.id}
          className={cn(
            'min-w-0 rounded-md border px-2.5 py-2',
            getToneSoftClassName(item.tone ?? 'neutral')
          )}
        >
          <dt className="flex min-w-0 items-center gap-1.5 text-[11px] font-medium text-muted-foreground">
            {item.icon ? <span className="shrink-0">{item.icon}</span> : null}
            <span className="min-w-0 truncate">{item.label}</span>
          </dt>
          {item.value ? (
            <dd className="mt-1 min-w-0 break-words font-medium text-foreground">{item.value}</dd>
          ) : null}
          {item.description ? (
            <dd className="mt-1 whitespace-pre-wrap break-words text-[11px] text-muted-foreground">
              {item.description}
            </dd>
          ) : null}
        </div>
      ))}
    </dl>
  );
}

interface OperationCardActionsProps {
  actions?: OperationCardAction[];
  align?: 'start' | 'end';
  compact?: boolean;
}

export function OperationCardActions({
  actions,
  align = 'end',
  compact = false,
}: OperationCardActionsProps) {
  const visibleActions = actions?.filter(action => action.label) ?? [];
  if (visibleActions.length === 0) return null;

  return (
    <div
      className={cn(
        'flex flex-wrap items-center gap-2',
        align === 'end' ? 'justify-end' : 'justify-start'
      )}
    >
      {visibleActions.map(action => (
        <OperationActionButton key={action.id} action={action} compact={compact} />
      ))}
    </div>
  );
}

function OperationActionButton({
  action,
  compact,
}: {
  action: OperationCardAction;
  compact: boolean;
}) {
  const isDisabled = Boolean(action.disabled || action.loading);
  const content = (
    <>
      {action.loading ? <Loader2 className="size-3.5 shrink-0 animate-spin" /> : action.icon}
      <span className="min-w-0 truncate">{action.label}</span>
      {action.href && action.external ? <ExternalLink className="size-3 shrink-0" /> : null}
    </>
  );
  const commonClassName = 'max-w-full min-w-0';

  if (action.href) {
    return (
      <Button
        asChild
        variant={action.variant ?? 'outline'}
        size={compact ? 'xs' : 'sm'}
        className={commonClassName}
        title={action.title ?? action.label}
      >
        <a
          href={action.href}
          target={action.external ? '_blank' : undefined}
          rel={action.external ? 'noreferrer' : undefined}
          aria-disabled={isDisabled}
          tabIndex={isDisabled ? -1 : undefined}
          onClick={event => {
            if (isDisabled) {
              event.preventDefault();
              return;
            }
            action.onClick?.();
          }}
        >
          {content}
        </a>
      </Button>
    );
  }

  return (
    <Button
      type="button"
      variant={action.variant ?? 'outline'}
      size={compact ? 'xs' : 'sm'}
      loading={action.loading}
      disabled={isDisabled}
      className={commonClassName}
      title={action.title ?? action.label}
      onClick={action.onClick}
    >
      {content}
    </Button>
  );
}
