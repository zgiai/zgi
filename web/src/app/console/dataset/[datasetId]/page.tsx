'use client';

import { useEffect, useMemo } from 'react';
import { useParams, useRouter } from 'next/navigation';
import {
  PermissionDeniedState,
  PermissionLoadingState,
} from '@/components/common/permission-gate-state';
import { useDataset } from '@/hooks/dataset/use-datasets';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { KNOWLEDGE_BASE_PERMISSION_ACTIONS } from '@/constants/permissions';

export default function DatasetPage() {
  const { datasetId } = useParams<{ datasetId: string }>();
  const router = useRouter();
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();

  const canViewDocuments = hasAnyPermission([
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentView,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentCreate,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentUpdate,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentDelete,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.indexManage,
  ]);
  const canUseRetrievalTest = hasAnyPermission(KNOWLEDGE_BASE_PERMISSION_ACTIONS.retrievalTest);
  const canViewGraph = hasAnyPermission([
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.graphView,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.graphManage,
  ]);
  const canOpenSettings = hasAnyPermission([
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.update,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.indexManage,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.graphManage,
  ]);

  const needsDatasetForRouting = canViewGraph && !canViewDocuments && !canUseRetrievalTest;
  const { data, isLoading: isDatasetLoading } = useDataset(datasetId, {
    enabled: needsDatasetForRouting,
  });
  const isGraphEnabled = data?.data?.enable_graph_flow ?? false;

  const targetHref = useMemo(() => {
    if (canViewDocuments) return `/console/dataset/${datasetId}/documents`;
    if (canUseRetrievalTest) return `/console/dataset/${datasetId}/hit-testing`;
    if (canViewGraph && isGraphEnabled) return `/console/dataset/${datasetId}/graph`;
    if (canOpenSettings) return `/console/dataset/${datasetId}/settings`;
    return null;
  }, [
    canOpenSettings,
    canUseRetrievalTest,
    canViewDocuments,
    canViewGraph,
    datasetId,
    isGraphEnabled,
  ]);

  useEffect(() => {
    if (isPermissionsLoading) return;
    if (needsDatasetForRouting && isDatasetLoading) return;
    if (!targetHref) return;
    router.replace(targetHref);
  }, [isDatasetLoading, isPermissionsLoading, needsDatasetForRouting, router, targetHref]);

  if (isPermissionsLoading || (needsDatasetForRouting && isDatasetLoading)) {
    return <PermissionLoadingState />;
  }

  if (!targetHref) {
    return <PermissionDeniedState />;
  }

  return <PermissionLoadingState />;
}
