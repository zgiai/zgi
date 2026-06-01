'use client';

import { WorkspaceRequiredState } from '@/components/common/workspace-required-state';
import { useT } from '@/i18n';

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

  return (
    <WorkspaceRequiredState
      className={className}
      title={t('workspaceRequired.title')}
      description={t('workspaceRequired.description')}
    />
  );
}
