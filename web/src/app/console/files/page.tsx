'use client';

import {
  PermissionDeniedState,
  PermissionLoadingState,
} from '@/components/common/permission-gate-state';
import FileManagementContent from '@/components/files/file-management-content';
import { FILE_VISIBLE_PERMISSION_CODES } from '@/constants/permissions';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';

const FileManagementPage = () => {
  const { hasAnyPermission, isLoading } = useAccountPermissions();
  const canViewFiles = hasAnyPermission(FILE_VISIBLE_PERMISSION_CODES);

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
