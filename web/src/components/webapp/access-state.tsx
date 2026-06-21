'use client';

import { Loader2, LogIn, ShieldAlert, WifiOff } from 'lucide-react';
import type { ReactNode } from 'react';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { WebAppRuntimeCapability } from '@/services/types/webapp';

export type WebAppAccessStateKind = 'loading' | 'login_required' | 'no_access' | 'offline';

interface WebAppAccessStateProps {
  kind: WebAppAccessStateKind;
  className?: string;
  onLogin?: () => void;
}

const OFFLINE_REASONS = new Set([
  'denied_disabled_surface',
  'denied_missing_surface',
  'offline',
]);

export function getWebAppAccessStateKind(
  capability?: WebAppRuntimeCapability | null
): WebAppAccessStateKind | null {
  if (!capability || capability.allowed) return null;

  if (capability.reason === 'login_required') {
    return 'login_required';
  }
  if (capability.reason === 'no_access' || capability.reason === 'denied_no_matching_grant') {
    return 'no_access';
  }
  if (OFFLINE_REASONS.has(capability.reason)) {
    return 'offline';
  }
  return 'no_access';
}

export function WebAppAccessState({ kind, className, onLogin }: WebAppAccessStateProps) {
  const t = useT('webapp');
  const isLoginRequired = kind === 'login_required';
  const icon = accessStateIcon(kind);
  const toneClass =
    kind === 'offline'
      ? 'bg-destructive/10 text-destructive ring-destructive/20'
      : 'bg-primary/10 text-primary ring-primary/20';

  return (
    <div
      className={cn(
        'flex h-full min-h-80 w-full flex-col items-center justify-center px-6 text-center',
        className
      )}
    >
      <div
        className={cn(
          'flex size-14 items-center justify-center rounded-full ring-1',
          kind === 'loading' ? 'bg-muted text-muted-foreground ring-border' : toneClass
        )}
      >
        {icon}
      </div>
      <h1 className="mt-4 text-lg font-semibold text-foreground">
        {t(`access.${kind}.title`)}
      </h1>
      <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">
        {t(`access.${kind}.description`)}
      </p>
      {isLoginRequired && onLogin ? (
        <Button type="button" className="mt-5 gap-2" onClick={onLogin}>
          <LogIn className="size-4" />
          {t('header.login')}
        </Button>
      ) : null}
    </div>
  );
}

function accessStateIcon(kind: WebAppAccessStateKind): ReactNode {
  switch (kind) {
    case 'loading':
      return <Loader2 className="size-7 animate-spin" />;
    case 'offline':
      return <WifiOff className="size-7" />;
    case 'login_required':
    case 'no_access':
      return <ShieldAlert className="size-7" />;
  }
}
