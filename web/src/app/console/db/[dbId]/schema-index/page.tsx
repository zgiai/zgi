'use client';

import { use } from 'react';
import { DbSchemaIndexPanel } from '@/components/db/schema-index-panel';
import {
  PermissionDeniedState,
  PermissionLoadingState,
} from '@/components/common/permission-gate-state';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { DATABASE_PERMISSION_ACTIONS } from '@/constants/permissions';

interface PageProps {
  params: Promise<{ dbId: string }>;
}

export default function DbSchemaIndexPage({ params }: PageProps) {
  const { dbId } = use(params);
  const { hasAnyPermission, isLoading } = useAccountPermissions();
  const canManage = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.schemaManage);
  const canView = canManage || hasAnyPermission(DATABASE_PERMISSION_ACTIONS.schemaView);

  if (isLoading) return <PermissionLoadingState />;
  if (!canView) return <PermissionDeniedState />;

  return <DbSchemaIndexPanel key={dbId} dbId={dbId} canManage={canManage} />;
}
