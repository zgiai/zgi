'use client';

import * as React from 'react';
import Link from 'next/link';
import { ChevronsUpDown, Check, Loader2, Settings, Users } from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useJoinedWorkspaces } from '@/hooks/workspace/use-joined-workspaces';
import { useUpdateCurrentWorkspace } from '@/hooks/workspace/use-update-current-workspace';
import { useCurrentUser } from '@/store/auth-store';
import { useWorkspaceStore } from '@/store';
import { canManageOrganizationWorkspaces } from '@/utils/workspace-access';
import type { Workspace } from '@/store';

interface WorkspaceSwitcherProps {
  isCollapsed?: boolean;
}

/**
 * Workspace switcher component for sidebar
 * Displays current workspace selection and allows switching between workspaces
 */
export function WorkspaceSwitcher({ isCollapsed }: WorkspaceSwitcherProps) {
  const t = useT('navigation');
  const tCommon = useT('common');
  const user = useCurrentUser();
  const workspaces = useWorkspaceStore.use.workspaces();
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const isOrganizationMode = useWorkspaceStore.use.isOrganizationMode();
  const { mutate: updateWorkspace } = useUpdateCurrentWorkspace();

  // Fetch joined workspaces from API and sync to store
  const { isLoading, isFetching } = useJoinedWorkspaces({ syncToStore: true });
  const canManageWorkspaces = canManageOrganizationWorkspaces(user);
  const isLoadingWorkspaces = (isLoading || isFetching) && workspaces.length === 0;

  const handleSelectWorkspace = (workspace: Workspace) => {
    updateWorkspace(workspace);
  };

  const getWorkspaceDisplayName = (workspace?: Pick<Workspace, 'name'> | null) => {
    if (!workspace?.name) return '';
    return workspace.name === 'Default Workspace' ? t('defaultWorkspace') : workspace.name;
  };

  const displayName = getWorkspaceDisplayName(currentWorkspace) || t('switchWorkspace');

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          className={cn(
            'flex items-center w-full rounded-md text-xs font-medium transition-colors',
            'bg-muted/40 p-1 text-foreground hover:bg-muted',
            isCollapsed ? 'justify-center' : 'justify-between'
          )}
          aria-label={t('switchWorkspace')}
          data-testid="workspace-switcher-trigger"
        >
          <div
            className={cn(
              'flex items-center',
              isCollapsed ? 'justify-center' : 'min-w-0 flex-1 gap-1'
            )}
          >
            {isCollapsed && !isOrganizationMode ? (
              <div className="flex h-8 w-8 items-center justify-center rounded-md bg-background text-muted-foreground shrink-0">
                <span className="text-xs leading-none">{displayName?.slice(0, 2)}</span>
              </div>
            ) : (
              <div className="flex h-8 w-8 items-center justify-center rounded-md bg-background text-muted-foreground shrink-0">
                <Users size={16} />
              </div>
            )}
            {!isCollapsed && (
              <span className="text-[11px] font-medium leading-[14px] text-left break-words line-clamp-2 flex-1 min-w-0 text-foreground">
                {displayName}
              </span>
            )}
          </div>
          {!isCollapsed && (
            <ChevronsUpDown size={16} className="text-muted-foreground shrink-0 ml-0.5" />
          )}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align={isCollapsed ? 'center' : 'start'}
        side="right"
        sideOffset={8}
        className="w-48 overflow-hidden"
        style={{
          maxHeight: 'min(22rem, var(--radix-dropdown-menu-content-available-height))',
        }}
      >
        <DropdownMenuLabel className="text-xs">{t('switchWorkspace')}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        <div
          className="overflow-y-auto pr-1"
          style={{
            maxHeight:
              'min(16rem, calc(var(--radix-dropdown-menu-content-available-height) - 4.5rem))',
          }}
        >
          {isLoadingWorkspaces ? (
            <DropdownMenuItem disabled className="text-xs text-muted-foreground">
              <Loader2 className="h-3 w-3 animate-spin" />
              {tCommon('workspaceSelector.loading')}
            </DropdownMenuItem>
          ) : workspaces.length > 0 ? (
            workspaces.map(workspace => (
              <DropdownMenuItem
                key={workspace.id}
                onClick={() => handleSelectWorkspace(workspace)}
                className="flex items-center justify-between cursor-pointer text-xs"
                title={getWorkspaceDisplayName(workspace)}
              >
                <div className="flex items-center gap-1.5 w-0 grow">
                  <div className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md bg-primary/10">
                    <Users className="h-3 w-3 text-primary" />
                  </div>
                  <span className="truncate text-[11px] break-all text-ellipsis">
                    {getWorkspaceDisplayName(workspace)}
                  </span>
                </div>
                {!isOrganizationMode && currentWorkspace?.id === workspace.id && (
                  <Check size={14} className="text-primary" />
                )}
              </DropdownMenuItem>
            ))
          ) : (
            <>
              <DropdownMenuItem
                disabled
                className="whitespace-normal text-xs leading-5 text-muted-foreground"
              >
                {canManageWorkspaces
                  ? tCommon('workspaceSelector.noWorkspacesAdmin')
                  : tCommon('workspaceSelector.noWorkspacesMember')}
              </DropdownMenuItem>
              {canManageWorkspaces ? (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem asChild className="cursor-pointer text-xs">
                    <Link href="/dashboard/organization/workspaces">
                      <Settings className="h-3.5 w-3.5" />
                      {tCommon('workspaceRequired.manageWorkspaces')}
                    </Link>
                  </DropdownMenuItem>
                </>
              ) : null}
            </>
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
