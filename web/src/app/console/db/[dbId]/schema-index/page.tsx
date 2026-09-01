'use client';

import { use } from 'react';
import { DbSchemaIndexPanel } from '@/components/db/schema-index-panel';
import {
  PermissionDeniedState,
  PermissionLoadingState,
} from '@/components/common/permission-gate-state';
import { useDbEffectivePermissions } from '@/hooks/db/use-db-effective-permissions';
import { DATABASE_PERMISSION_ACTIONS } from '@/constants/permissions';

interface PageProps {
  params: Promise<{ dbId: string }>;
}

export default function DbSchemaIndexPage({ params }: PageProps) {
  const { dbId } = use(params);
  const { hasAnyPermission, isLoading } = useDbEffectivePermissions(dbId);
  const canManage = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.schemaManage);
  const canView = canManage || hasAnyPermission(DATABASE_PERMISSION_ACTIONS.schemaView);

  if (isLoading) return <PermissionLoadingState />;
  if (!canView) return <PermissionDeniedState />;

  return <DbSchemaIndexPanel key={dbId} dbId={dbId} canManage={canManage} />;
}
