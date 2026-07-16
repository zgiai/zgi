'use client';

import * as React from 'react';
import { Check, ChevronsUpDown, Users } from 'lucide-react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useWorkspaceStore } from '@/store/workspace-store';
import { useJoinedWorkspaces } from '@/hooks/workspace/use-joined-workspaces';
import { useUpdateCurrentWorkspace } from '@/hooks/workspace/use-update-current-workspace';
import type { Workspace } from '@/store/workspace-store';
import { cn } from '@/lib/utils';

export type ModuleType = 'agents' | 'datasets' | 'databases' | 'files';

interface PersonalSpaceEmptyStateProps {
  /** Which module this empty state is for */
  moduleType: ModuleType;
  /** Custom icon to display (defaults to module-specific icon) */
  icon?: React.ReactNode;
  /** Additional CSS classes */
  className?: string;
}

/**
 * Empty state for personal workbench mode.
 * Provides an inline workspace switcher instead of a guided overlay.
 */
export function PersonalSpaceEmptyState({
  moduleType,
  icon,
  className,
}: PersonalSpaceEmptyStateProps) {
  const tCommon = useT('common');
  const tNavigation = useT('navigation');
  const workspaces = useWorkspaceStore.use.workspaces();
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const { mutate: updateWorkspace, isPending: isUpdatingWorkspace } = useUpdateCurrentWorkspace();

  useJoinedWorkspaces({ syncToStore: true });

  const hasWorkspaces = workspaces.length > 0;
  const emptyMessage = tCommon(`personalSpaceEmpty.${moduleType}`);
  const currentWorkspaceLabel = currentWorkspace?.name || tNavigation('switchWorkspace');

  const handleSelectWorkspace = React.useCallback(
    (workspace: Workspace) => {
      updateWorkspace(workspace);
    },
    [updateWorkspace]
  );

  return (
    <div
      className={cn(
        'flex w-full max-w-md flex-col items-center justify-center rounded-2xl border border-dashed border-border/80 bg-background/70 px-6 py-10 text-center shadow-sm',
        className
      )}
    >
      <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-primary/10 text-primary">
        {icon || <Users className="h-8 w-8" />}
      </div>

      <h3 className="mb-2 text-xl font-semibold text-foreground">{emptyMessage}</h3>
      <p className="mb-6 max-w-md text-sm leading-6 text-muted-foreground">
        {tCommon('personalSpaceEmpty.description')}
      </p>

      {hasWorkspaces ? (
        <div className="w-full rounded-xl border border-border/80 bg-muted/30 p-3 text-left">
          <p className="mb-2 text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
            {tNavigation('switchWorkspace')}
          </p>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="outline"
                disabled={isUpdatingWorkspace}
                className="h-11 w-full justify-between rounded-lg border-border/80 bg-background px-3 shadow-sm"
              >
                <div className="flex min-w-0 items-center gap-2">
                  <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                    <Users className="h-4 w-4" />
                  </div>
                  <span className="truncate text-sm font-medium">{currentWorkspaceLabel}</span>
                </div>
                <ChevronsUpDown className="h-4 w-4 shrink-0 text-muted-foreground" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="center" className="w-[320px]">
              <DropdownMenuLabel>{tNavigation('switchWorkspace')}</DropdownMenuLabel>
              <DropdownMenuSeparator />
              {workspaces.map(workspace => (
                <DropdownMenuItem
                  key={workspace.id}
                  onClick={() => handleSelectWorkspace(workspace)}
                  className="flex cursor-pointer items-center justify-between"
                  title={workspace.name}
                >
                  <div className="flex min-w-0 items-center gap-2">
                    <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                      <Users className="h-3.5 w-3.5" />
                    </div>
                    <span className="truncate text-xs">{workspace.name}</span>
                  </div>
                  {currentWorkspace?.id === workspace.id ? (
                    <Check className="h-4 w-4 text-primary" />
                  ) : null}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">
          {tCommon('personalSpaceEmpty.noWorkspacesHint')}
        </p>
      )}
    </div>
  );
}
