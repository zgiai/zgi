'use client';

import {
  PermissionDeniedState,
  PermissionLoadingState,
} from '@/components/common/permission-gate-state';
import FileManagementContent from '@/components/files/file-management-content';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';

const FileManagementPage = () => {
  const { hasWorkspaceAccess, isLoading } = useAccountPermissions();
  const canViewFiles = hasWorkspaceAccess();

  if (isLoading) {
    return <PermissionLoadingState />;
  }

  if (!canViewFiles) {
    return <PermissionDeniedState />;
  }

  return (
    <div className="flex flex-col h-full overflow-y-auto">
      <FileManagementContent />
    </div>
  );
};

export default FileManagementPage;
