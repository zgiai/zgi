'use client';

import { use } from 'react';
import TableColumns from '@/components/db/table-columns';
import {
  PermissionDeniedState,
  PermissionLoadingState,
} from '@/components/common/permission-gate-state';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { DATABASE_PERMISSION_ACTIONS } from '@/constants/permissions';

interface PageProps {
  params: Promise<{ dbId: string; tableId: string }>;
}

export default function DbTableStructurePage({ params }: PageProps) {
  const { dbId, tableId } = use(params);
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canOpenSchema = hasAnyPermission([
    ...DATABASE_PERMISSION_ACTIONS.schemaView,
    ...DATABASE_PERMISSION_ACTIONS.schemaManage,
  ]);

  if (isPermissionsLoading) {
    return <PermissionLoadingState />;
  }

  if (!canOpenSchema) {
    return <PermissionDeniedState />;
  }

  return (
    <div className="p-6 h-full flex flex-col w-full overflow-hidden">
      <TableColumns dbId={dbId} tableId={tableId} />
    </div>
  );
}
