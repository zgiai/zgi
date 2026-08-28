'use client';

import { AlertCircle, CheckCircle2, LoaderCircle } from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { IntegrationCapabilityAvailability } from '@/services/types/integration';

type CapabilityAvailabilityState =
  | IntegrationCapabilityAvailability
  | 'checking'
  | 'status_unavailable';

interface ProviderCapabilityAvailabilityProps {
  state: CapabilityAvailabilityState;
  compatibleConnectionCount?: number;
  showGuidance?: boolean;
  className?: string;
}

export function ProviderCapabilityAvailability({
  state,
  compatibleConnectionCount = 0,
  showGuidance = true,
  className,
}: ProviderCapabilityAvailabilityProps) {
  const t = useT('integrations');
  const available = state === 'available';
  const checking = state === 'checking';
  const Icon = available ? CheckCircle2 : checking ? LoaderCircle : AlertCircle;

  return (
    <div className={cn('min-w-0 space-y-1.5', className)}>
      <span
        className={cn(
          'inline-flex max-w-full items-center gap-1.5 rounded-full px-2 py-1 text-[11px] font-medium',
          available && 'bg-success/10 text-success',
          checking && 'bg-muted text-muted-foreground',
          !available && !checking && 'bg-warning/10 text-warning'
        )}
      >
        <Icon className={cn('size-3 shrink-0', checking && 'animate-spin')} aria-hidden="true" />
        <span className="truncate">{t(`capabilities.availability.${state}`)}</span>
      </span>
      {showGuidance ? (
        <p className="text-[11px] leading-4 text-muted-foreground">
          {available
            ? t('capabilities.availableConnections', { count: compatibleConnectionCount })
            : t(`capabilities.remediation.${state}`)}
        </p>
      ) : null}
    </div>
  );
}
