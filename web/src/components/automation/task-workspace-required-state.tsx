'use client';

import * as React from 'react';
import { Check, ChevronsUpDown, Sparkles, User, Users } from 'lucide-react';
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
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { useWorkspaceStore } from '@/store/workspace-store';
import { useJoinedWorkspaces } from '@/hooks/workspace/use-joined-workspaces';
import { useUpdateCurrentWorkspace } from '@/hooks/workspace/use-update-current-workspace';
import type { Workspace } from '@/store/workspace-store';

interface TaskWorkspaceRequiredStateProps {
  className?: string;
}

/**
 * @component TaskWorkspaceRequiredState
 * @category Feature
 * @status Stable
 * @description Empty state shown when the user has not selected a workspace for automation tasks.
 * @usage Render inside the task workbench before calling workspace-scoped automation hooks.
 */
export function TaskWorkspaceRequiredState({ className }: TaskWorkspaceRequiredStateProps) {
  const t = useT('automation');
  const workspaces = useWorkspaceStore.use.workspaces();
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const isOrganizationMode = useWorkspaceStore.use.isOrganizationMode();
  const { mutate: updateWorkspace, isPending } = useUpdateCurrentWorkspace();

  useJoinedWorkspaces({ syncToStore: true });

  const currentLabel = isOrganizationMode
    ? t('workspaceRequired.personalSpace')
    : currentWorkspace?.name ?? t('workspaceRequired.switchWorkspace');
  const CurrentIcon = isOrganizationMode ? User : Users;

  const handleSelectWorkspace = React.useCallback(
    (workspace: Workspace | null) => {
      updateWorkspace(workspace);
    },
    [updateWorkspace]
  );

  return (
    <div
      className={cn(
        'flex h-full min-h-0 items-center justify-center bg-[radial-gradient(circle_at_top,_hsl(var(--primary)/0.14),_transparent_55%)] p-4 md:p-8',
        className
      )}
    >
      <Card
        className="w-full max-w-2xl border-border/70 bg-background/95 shadow-2xl shadow-primary/5 backdrop-blur"
        padding="none"
      >
        <CardContent className="grid gap-8 p-8 md:grid-cols-[1.1fr_0.9fr]">
          <div className="space-y-5">
            <div className="inline-flex h-12 w-12 items-center justify-center rounded-2xl bg-primary/10 text-primary">
              <Sparkles className="size-6" />
            </div>
            <div className="space-y-2">
              <h2 className="text-2xl font-semibold tracking-tight text-foreground">
                {t('workspaceRequired.title')}
              </h2>
              <p className="text-sm leading-6 text-muted-foreground">
                {t('workspaceRequired.description')}
              </p>
            </div>
          </div>

          <div className="rounded-2xl border border-border/70 bg-muted/30 p-4">
            <p className="mb-2 text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground">
              {t('workspaceRequired.switchWorkspace')}
            </p>
            {workspaces.length > 0 ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="outline"
                    className="h-12 w-full justify-between rounded-xl border-border/80 bg-background"
                    disabled={isPending}
                  >
                    <span className="flex min-w-0 items-center gap-3">
                      <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                        <CurrentIcon className="size-4" />
                      </span>
                      <span className="truncate text-sm font-medium">{currentLabel}</span>
                    </span>
                    <ChevronsUpDown className="size-4 text-muted-foreground" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-[320px]">
                  <DropdownMenuLabel>{t('workspaceRequired.switchWorkspace')}</DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    className="flex cursor-pointer items-center justify-between"
                    onClick={() => handleSelectWorkspace(null)}
                  >
                    <span className="flex min-w-0 items-center gap-2">
                      <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                        <User className="size-4" />
                      </span>
                      <span className="truncate text-sm">
                        {t('workspaceRequired.personalSpace')}
                      </span>
                    </span>
                    {isOrganizationMode ? <Check className="size-4 text-primary" /> : null}
                  </DropdownMenuItem>
                  {workspaces.map(workspace => (
                    <DropdownMenuItem
                      key={workspace.id}
                      className="flex cursor-pointer items-center justify-between"
                      onClick={() => handleSelectWorkspace(workspace)}
                      title={workspace.name}
                    >
                      <span className="flex min-w-0 items-center gap-2">
                        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                          <Users className="size-4" />
                        </span>
                        <span className="truncate text-sm">{workspace.name}</span>
                      </span>
                      {!isOrganizationMode && currentWorkspace?.id === workspace.id ? (
                        <Check className="size-4 text-primary" />
                      ) : null}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            ) : (
              <p className="text-sm leading-6 text-muted-foreground">
                {t('workspaceRequired.noWorkspaces')}
              </p>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
