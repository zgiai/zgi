'use client';

import { AlertTriangle, Check, HelpCircle, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import type {
  OperationCardAction,
  OperationCardTone,
  OperationConfirmationCardProps,
  OperationConfirmationStatus,
} from '@/components/aichat/operation-cards/types';
import {
  OperationCardActions,
  OperationCardHeader,
  OperationCardShell,
  OperationMetaGrid,
  OperationStatusBadge,
  getToneSoftClassName,
  getToneTextClassName,
} from '@/components/aichat/operation-cards/primitives';

const CONFIRMATION_STATUS_FALLBACK_LABEL: Record<OperationConfirmationStatus, string> = {
  pending: 'Needs review',
  confirmed: 'Confirmed',
  rejected: 'Rejected',
  expired: 'Expired',
};

function getConfirmationTone(status: OperationConfirmationStatus): OperationCardTone {
  if (status === 'confirmed') return 'success';
  if (status === 'rejected' || status === 'expired') return 'destructive';
  return 'warning';
}

function withFallbackIcon(
  action: OperationCardAction | undefined,
  icon: OperationCardAction['icon']
) {
  if (!action) return undefined;
  return { ...action, icon: action.icon ?? icon };
}

export function OperationConfirmationCard({
  title = 'Confirm operation',
  description,
  status = 'pending',
  statusLabel,
  eyebrow,
  summary,
  items,
  warnings,
  confirmAction,
  cancelAction,
  actions,
  compact = false,
  className,
}: OperationConfirmationCardProps) {
  const tone = getConfirmationTone(status);
  const visibleWarnings = warnings ?? [];
  const mergedActions = [
    withFallbackIcon(cancelAction, <X className="size-3.5" />),
    withFallbackIcon(confirmAction, <Check className="size-3.5" />),
    ...(actions ?? []),
  ].filter((action): action is OperationCardAction => Boolean(action));

  return (
    <OperationCardShell compact={compact} className={className}>
      <OperationCardHeader
        compact={compact}
        icon={<HelpCircle className={cn('size-4', getToneTextClassName(tone))} />}
        title={title}
        description={description}
        eyebrow={eyebrow}
        badge={
          <OperationStatusBadge
            label={statusLabel ?? CONFIRMATION_STATUS_FALLBACK_LABEL[status]}
            tone={tone}
          />
        }
      />

      {summary ? (
        <div
          className={cn(
            'whitespace-pre-wrap break-words rounded-md border px-3 py-2.5 text-sm leading-relaxed',
            getToneSoftClassName(tone)
          )}
        >
          {summary}
        </div>
      ) : null}

      <OperationMetaGrid items={items} compact={compact} />

      {visibleWarnings.length > 0 ? (
        <div className="space-y-2">
          {visibleWarnings.map(warning => {
            const warningTone = warning.tone ?? 'warning';

            return (
              <div
                key={warning.id}
                className={cn(
                  'flex min-w-0 items-start gap-2 rounded-md border px-3 py-2 text-xs',
                  getToneSoftClassName(warningTone)
                )}
              >
                <span className={cn('mt-0.5 shrink-0', getToneTextClassName(warningTone))}>
                  {warning.icon ?? <AlertTriangle className="size-3.5" />}
                </span>
                <div className="min-w-0 flex-1">
                  {warning.title ? (
                    <div className="break-words font-medium text-foreground">{warning.title}</div>
                  ) : null}
                  {warning.description ? (
                    <div className="mt-1 whitespace-pre-wrap break-words text-muted-foreground">
                      {warning.description}
                    </div>
                  ) : null}
                </div>
              </div>
            );
          })}
        </div>
      ) : null}

      <OperationCardActions actions={mergedActions} compact={compact} />
    </OperationCardShell>
  );
}
