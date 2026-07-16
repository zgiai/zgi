'use client';

import React from 'react';
import { usePathname } from 'next/navigation';
import { ShieldAlert, Loader2 } from 'lucide-react';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import { useT } from '@/i18n';
import { WorkspaceRequiredState } from '@/components/common/workspace-required-state';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { isOrganizationScopedWorkRoute } from '@/routes/access';

export default function ConsoleWorkLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const t = useT();
  const currentWorkspace = useCurrentWorkspace();
  const {
    isLoading: isCapabilitiesLoading,
    canUseOrganizationScope,
    canUseWorkspaceScope,
    isWorkspaceRequired,
  } = useAccountCapabilities();

  if (isCapabilitiesLoading) {
    return (
      <div className="flex h-full w-full items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (isOrganizationScopedWorkRoute(pathname) && canUseOrganizationScope) {
    return <>{children}</>;
  }

  if (!isOrganizationScopedWorkRoute(pathname) && (isWorkspaceRequired || !currentWorkspace)) {
    return <WorkspaceRequiredState />;
  }

  if (
    (isOrganizationScopedWorkRoute(pathname) && !canUseOrganizationScope) ||
    (!isOrganizationScopedWorkRoute(pathname) && !canUseWorkspaceScope)
  ) {
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
