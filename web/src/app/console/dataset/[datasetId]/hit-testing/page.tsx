'use client';

import HitTestingPage from '@/components/datasets/hit-testing';
import {
  PermissionDeniedState,
  PermissionLoadingState,
} from '@/components/common/permission-gate-state';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { KNOWLEDGE_BASE_PERMISSION_ACTIONS } from '@/constants/permissions';

export default function DatasetHitTestingPage() {
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canUseRetrievalTest = hasAnyPermission(KNOWLEDGE_BASE_PERMISSION_ACTIONS.retrievalTest);

  if (isPermissionsLoading) {
    return <PermissionLoadingState />;
  }

  if (!canUseRetrievalTest) {
    return <PermissionDeniedState />;
  }

  return <HitTestingPage />;
}
