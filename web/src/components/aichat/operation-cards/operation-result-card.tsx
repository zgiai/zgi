'use client';

import { AlertCircle, CheckCircle2, ExternalLink, FileText, Info } from 'lucide-react';
import { cn } from '@/lib/utils';
import type {
  OperationCardTone,
  OperationResultCardProps,
  OperationResultStatus,
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

const RESULT_STATUS_FALLBACK_LABEL: Record<OperationResultStatus, string> = {
  success: 'Success',
  error: 'Error',
  warning: 'Warning',
  info: 'Info',
};

function getResultTone(status: OperationResultStatus): OperationCardTone {
  if (status === 'success') return 'success';
  if (status === 'error') return 'destructive';
  if (status === 'warning') return 'warning';
  return 'info';
}

function getResultIcon(status: OperationResultStatus) {
  if (status === 'success') return CheckCircle2;
  if (status === 'error' || status === 'warning') return AlertCircle;
  return Info;
}

export function OperationResultCard({
  title = 'Operation result',
  description,
  status = 'info',
  statusLabel,
  eyebrow,
  metrics,
  artifacts,
  details,
  actions,
  compact = false,
  className,
}: OperationResultCardProps) {
  const tone = getResultTone(status);
  const Icon = getResultIcon(status);
  const visibleDetails = details ?? [];

  return (
    <OperationCardShell compact={compact} className={className}>
      <OperationCardHeader
        compact={compact}
        icon={<Icon className={cn('size-4', getToneTextClassName(tone))} />}
        title={title}
        description={description}
        eyebrow={eyebrow}
        badge={
          <OperationStatusBadge
            label={statusLabel ?? RESULT_STATUS_FALLBACK_LABEL[status]}
            tone={tone}
          />
        }
      />

      <OperationMetaGrid items={metrics} compact={compact} />

      {visibleDetails.length > 0 ? (
        <div className="space-y-2">
          {visibleDetails.map(detail => {
            const detailTone = detail.tone ?? tone;

            return (
              <div
                key={detail.id}
                className={cn('rounded-md border px-3 py-2.5', getToneSoftClassName(detailTone))}
              >
                <div className="flex min-w-0 items-start gap-2">
                  {detail.icon ? (
                    <span className={cn('mt-0.5 shrink-0', getToneTextClassName(detailTone))}>
                      {detail.icon}
                    </span>
                  ) : null}
                  <div className="min-w-0 flex-1">
                    {detail.title ? (
                      <div className="break-words font-medium text-foreground">{detail.title}</div>
                    ) : null}
                    {detail.description ? (
                      <div className="mt-1 whitespace-pre-wrap break-words text-xs leading-relaxed text-muted-foreground">
                        {detail.description}
                      </div>
                    ) : null}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      ) : null}

      {artifacts?.length ? (
        <div className="space-y-2">
          {artifacts.map(artifact => {
            const artifactContent = (
              <>
                <span className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted/30 text-muted-foreground">
                  {artifact.icon ?? <FileText className="size-4" />}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium text-foreground">
                    {artifact.label}
                  </span>
                  {artifact.description ? (
                    <span className="mt-0.5 block line-clamp-2 text-xs text-muted-foreground">
                      {artifact.description}
                    </span>
                  ) : null}
                </span>
                {artifact.href && artifact.external ? (
                  <ExternalLink className="size-3.5 shrink-0 text-muted-foreground" />
                ) : null}
              </>
            );

            if (artifact.href) {
              return (
                <a
                  key={artifact.id}
                  href={artifact.href}
                  target={artifact.external ? '_blank' : undefined}
                  rel={artifact.external ? 'noreferrer' : undefined}
                  onClick={artifact.onClick}
                  className="flex min-w-0 items-center gap-2 rounded-md border bg-background/80 px-2.5 py-2 text-left transition-colors hover:bg-muted/40"
                >
                  {artifactContent}
                </a>
              );
            }

            return (
              <button
                key={artifact.id}
                type="button"
                onClick={artifact.onClick}
                disabled={!artifact.onClick}
                className="flex min-w-0 items-center gap-2 rounded-md border bg-background/80 px-2.5 py-2 text-left transition-colors enabled:hover:bg-muted/40 disabled:cursor-default"
              >
                {artifactContent}
              </button>
            );
          })}
        </div>
      ) : null}

      <OperationCardActions actions={actions} compact={compact} />
    </OperationCardShell>
  );
}
