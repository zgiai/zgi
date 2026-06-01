'use client';

import React from 'react';
import { ShieldAlert, Loader2 } from 'lucide-react';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { WorkspaceRequiredState } from '@/components/common/workspace-required-state';
import { useCurrentWorkspace } from '@/store/workspace-store';

export default function ConsoleWorkLayout({ children }: { children: React.ReactNode }) {
  const t = useT();
  const currentWorkspace = useCurrentWorkspace();
  const { hasPermission, isLoading: isPermissionsLoading } = useAccountPermissions();

  if (!currentWorkspace) {
    return <WorkspaceRequiredState />;
  }

  if (isPermissionsLoading) {
    return (
      <div className="flex h-full w-full items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!hasPermission('workspace.view')) {
    return (
      <div className="flex flex-col items-center justify-center h-full w-full p-4 text-center">
        <ShieldAlert className="h-12 w-12 text-muted-foreground mb-4" />
        <h2 className="text-xl font-semibold mb-2">{t('common.accessDenied')}</h2>
        <p className="text-muted-foreground max-w-md">{t('common.unauthorizedDescription')}</p>
      </div>
    );
  }

  return <>{children}</>;
}
