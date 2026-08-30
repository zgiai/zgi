'use client';

import type { ReactNode } from 'react';
import { Loader2, LockKeyhole } from 'lucide-react';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import type { PermissionCode } from '@/constants/permissions';

export function DigitalHumanFeatureGate({
  children,
  permission = 'digital_human.view',
}: {
  children: ReactNode;
  permission?: PermissionCode;
}) {
  const t = useT('digitalHumans');
  const permissions = useAccountPermissions();

  if (permissions.isLoading) {
    return (
      <div className="flex min-h-[50vh] items-center justify-center" aria-label={t('loading')}>
        <Loader2 className="size-6 animate-spin text-muted-foreground" />
      </div>
    );
  }
  if (!permissions.hasPermission(permission)) {
    return (
      <FeatureState
        icon={LockKeyhole}
        title={t('forbidden.title')}
        description={t('forbidden.description')}
      />
    );
  }
  return children;
}

function FeatureState({
  icon: Icon,
  title,
  description,
}: {
  icon: typeof LockKeyhole;
  title: string;
  description: string;
}) {
  return (
    <div className="mx-auto flex min-h-[50vh] max-w-lg flex-col items-center justify-center px-6 text-center">
      <div className="mb-4 rounded-full bg-muted p-4">
        <Icon className="size-7 text-muted-foreground" />
      </div>
      <h1 className="text-xl font-semibold">{title}</h1>
      <p className="mt-2 text-sm text-muted-foreground">{description}</p>
    </div>
  );
}
