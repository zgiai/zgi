'use client';

import { use } from 'react';
import TableData from '@/components/db/table-data';
import {
  PermissionDeniedState,
  PermissionLoadingState,
} from '@/components/common/permission-gate-state';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { DATABASE_PERMISSION_ACTIONS } from '@/constants/permissions';

interface PageProps {
  params: Promise<{ dbId: string; tableId: string }>;
}

export default function DbTableDetailPage({ params }: PageProps) {
  const { dbId, tableId } = use(params);
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canOpenRecords = hasAnyPermission([
    ...DATABASE_PERMISSION_ACTIONS.recordView,
    ...DATABASE_PERMISSION_ACTIONS.recordCreate,
    ...DATABASE_PERMISSION_ACTIONS.recordUpdate,
    ...DATABASE_PERMISSION_ACTIONS.recordDelete,
  ]);

  if (isPermissionsLoading) {
    return <PermissionLoadingState />;
  }

  if (!canOpenRecords) {
    return <PermissionDeniedState />;
  }

  return (
    <div className="p-6 h-full min-h-0 flex flex-col w-full overflow-hidden">
      <TableData dbId={dbId} tableId={tableId} />
    </div>
  );
}
