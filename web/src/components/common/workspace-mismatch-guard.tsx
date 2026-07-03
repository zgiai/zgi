'use client';

import React from 'react';
import { ShieldAlert, RefreshCw } from 'lucide-react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { useWorkspaceStore } from '@/store/workspace-store';
import { useUpdateCurrentWorkspace } from '@/hooks/workspace/use-update-current-workspace';

interface WorkspaceMismatchGuardProps {
  /** Whether the resource detail is still loading */
  isLoading?: boolean;
  /** The workspace ID the resource belongs to */
  targetWorkspaceId: string;
  /** Optional workspace name (if already fetched) */
  targetWorkspaceName?: string;
  /** Content to render if workspaces match */
  children: React.ReactNode;
}

/**
 * Guard component that checks if the current selected workspace matches the required workspace for a resource.
 * If they don't match, it shows an access denied message with a "Switch Workspace" button if the user is a member of the target workspace.
 */
export function WorkspaceMismatchGuard({
  isLoading,
  targetWorkspaceId,
  targetWorkspaceName,
  children,
}: WorkspaceMismatchGuardProps) {
  const t = useT();

  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const contextStatus = useWorkspaceStore.use.contextStatus();
  const workspaces = useWorkspaceStore.use.workspaces();
  const updateWorkspace = useUpdateCurrentWorkspace();

  if (isLoading) {
    return (
      <div className="flex h-full w-full items-center justify-center min-h-[400px]">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    );
  }

  // If match or no target defined yet, pass through.
  if (!targetWorkspaceId || (contextStatus === 'ready' && currentWorkspace?.id === targetWorkspaceId)) {
    return <>{children}</>;
  }

  // Find the target workspace in local list to see if user has access to it
  const targetWorkspace = workspaces.find(tw => tw.id === targetWorkspaceId);
  const workspaceName = targetWorkspaceName || targetWorkspace?.name || 'Unknown Workspace';
  const currentWorkspaceName = currentWorkspace?.name || t('navigation.switchWorkspace');

  const handleSwitch = () => {
    if (targetWorkspace) {
      updateWorkspace.mutate(targetWorkspace);
    }
  };

  return (
    <div className="flex flex-col items-center justify-center h-full min-h-[400px] gap-4 text-center p-8 bg-background">
      <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center">
        <ShieldAlert className="w-8 h-8 text-muted-foreground" />
      </div>
      <div className="space-y-2">
        <h2 className="text-xl font-semibold text-foreground">
          {t('common.workspaceMismatch.title')}
        </h2>
        <p className="text-muted-foreground max-w-md">
          {t('common.workspaceMismatch.description', { workspaceName, currentWorkspaceName })}
        </p>
        <p className="text-sm text-muted-foreground">{t('common.workspaceMismatch.actionHint')}</p>
      </div>

      {targetWorkspace && (
        <Button
          onClick={handleSwitch}
          className="gap-2 mt-2"
          variant="default"
          disabled={updateWorkspace.isPending}
          loading={updateWorkspace.isPending}
        >
          <RefreshCw className="h-4 w-4" />
          {t('common.workspaceMismatch.switchButton')}
        </Button>
      )}
    </div>
  );
}
