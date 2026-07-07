'use client';

import { Loader2, ShieldAlert } from 'lucide-react';
import { useT } from '@/i18n';

interface PermissionGateStateProps {
  className?: string;
}

export function PermissionLoadingState({ className }: PermissionGateStateProps) {
  return (
    <div className={className ?? 'flex h-full w-full items-center justify-center'}>
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  );
}

export function PermissionDeniedState({ className }: PermissionGateStateProps) {
  const t = useT();

  return (
    <div
      className={
        className ?? 'flex h-full w-full flex-col items-center justify-center p-4 text-center'
      }
    >
      <ShieldAlert className="mb-4 h-12 w-12 text-muted-foreground" />
      <h2 className="mb-2 text-xl font-semibold">{t('common.accessDenied')}</h2>
      <p className="max-w-md text-muted-foreground">{t('common.unauthorizedDescription')}</p>
    </div>
  );
}
