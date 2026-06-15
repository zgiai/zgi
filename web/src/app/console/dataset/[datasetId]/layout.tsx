'use client';

import * as React from 'react';
import { useParams, usePathname } from 'next/navigation';
import { useDataset } from '@/hooks/dataset/use-datasets';
import { useT } from '@/i18n';
import { Database, Search, Cog, ShieldAlert, Network, Pencil } from 'lucide-react';
import { getSidebarCollapsed, saveSidebarCollapsed } from '@/utils/ui-local';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useIsInitialized } from '@/store/auth-store';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { WorkspaceMismatchGuard } from '@/components/common/workspace-mismatch-guard';
import { ICON_BG } from '@/lib/config';
import {
  ResourceSidebar,
  ResourceSidebarHeader,
  type ResourceSidebarNavItem,
} from '@/components/common/resource-sidebar';
import { EditDatasetDialog } from '@/components/datasets/dialog/edit-dataset-dialog';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Button } from '@/components/ui/button';

export default function DatasetDetailLayout({ children }: { children: React.ReactNode }) {
  function DatasetModelsPreloader() {
    useAvailableModels({ use_case: 'text-chat' });
    useAvailableModels({ use_case: 'embedding' });
    useAvailableModels({ use_case: 'rerank' });
    return null;
  }
  const isAuthReady = useIsInitialized();
  const { datasetId } = useParams<{ datasetId: string }>();
  const pathname = usePathname();
  const { data, isLoading } = useDataset(datasetId, {
    // Refetch periodically to update available_document_count when documents finish indexing
    refetchInterval: 10000,
    refetchIntervalInBackground: false,
  });
  const t = useT();

  // Permission checking
  const { hasPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canView = hasPermission('knowledge_base.view');
  const canManage = hasPermission('knowledge_base.manage');

  // Get dataset details for conditional rendering
  const dataset = data?.data;

  // Persist collapsed state to localStorage
  const [isCollapsed, setIsCollapsed] = React.useState<boolean>(() =>
    getSidebarCollapsed('dataset', true)
  );
  const [editOpen, setEditOpen] = React.useState(false);
  React.useEffect(() => {
    saveSidebarCollapsed('dataset', isCollapsed);
  }, [isCollapsed]);

  const toggleCollapse = () => setIsCollapsed(prev => !prev);

  // Check if dataset has completed documents
  // const hasCompletedDocuments = (dataset?.available_document_count ?? 0) > 0;

  // Check if graph flow is enabled for this dataset
  const isGraphEnabled = dataset?.enable_graph_flow ?? false;

  // Dynamic nav items based on permissions and feature flags
  const navItems: ResourceSidebarNavItem[] = React.useMemo(() => {
    const items: ResourceSidebarNavItem[] = [
      {
        title: t('datasets.documentsTitle'),
        href: `/console/dataset/${datasetId}/documents`,
        icon: Database,
      },
      {
        title: t('datasets.hitTestingTitle'),
        href: `/console/dataset/${datasetId}/hit-testing`,
        icon: Search,
      },
    ];

    // Only show Knowledge Graph when graph flow is enabled
    if (isGraphEnabled) {
      items.push({
        title: t('datasets.knowledgeGraphTitle'),
        href: `/console/dataset/${datasetId}/graph`,
        icon: Network,
      });
    }

    // Only show settings when user has manage permission
    if (canManage) {
      items.push({
        title: t('datasets.settingsTitle'),
        href: `/console/dataset/${datasetId}/settings`,
        icon: Cog,
      });
    }

    return items;
  }, [datasetId, canManage, isGraphEnabled, t]);

  // Access denied state
  if (!isPermissionsLoading && !canView) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4 text-center p-8">
        <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center">
          <ShieldAlert className="w-8 h-8 text-muted-foreground" />
        </div>
        <div className="space-y-2">
          <h2 className="text-lg font-semibold text-foreground">{t('common.accessDenied')}</h2>
          <p className="text-sm text-muted-foreground max-w-md">
            {t('common.unauthorizedDescription')}
          </p>
        </div>
      </div>
    );
  }

  const iconType = dataset?.icon_type;
  const textIcon = dataset?.icon || dataset?.name?.slice(0, 2).toUpperCase() || 'DS';
  const iconBackground = dataset?.icon_background || ICON_BG;
  const imgSrc = iconType === 'image' ? dataset?.icon_url : undefined;

  return (
    <WorkspaceMismatchGuard
      isLoading={isLoading}
      targetWorkspaceId={dataset?.workspace_id || ''}
      targetWorkspaceName={dataset?.workspace?.name}
    >
      <div className="flex h-full bg-background min-w-0">
        <ResourceSidebar
          isCollapsed={isCollapsed}
          onToggleCollapse={toggleCollapse}
          expandLabel={t('navigation.expand')}
          collapseLabel={t('navigation.collapse')}
          header={
            <ResourceSidebarHeader
              isCollapsed={isCollapsed}
              iconType={iconType}
              iconSrc={imgSrc}
              icon={textIcon}
              iconBackground={iconBackground}
              showIdentity={false}
              backHref="/console/dataset"
              backLabel={t('datasets.backToDatasetList')}
            />
          }
          navItems={navItems}
          pathname={pathname}
        />

        {/* Main Content Area */}
        <div className="flex flex-col grow min-w-0 relative">
          <div className="flex min-h-14 shrink-0 items-center justify-between gap-4 border-b bg-background px-6 py-3">
            <div className="flex min-w-0 items-center gap-3">
              <IconPreview
                iconType={iconType === 'image' ? 'image' : 'text'}
                src={iconType === 'image' ? imgSrc : ''}
                icon={textIcon}
                iconBackground={iconBackground}
                editable={false}
                size="sidebar"
              />
              <div className="min-w-0">
                <div
                  className="truncate text-sm font-semibold leading-5 text-foreground"
                  title={dataset?.name}
                >
                  {dataset?.name || (isLoading ? t('datasets.loading') : '-')}
                </div>
                <div
                  className="truncate text-xs leading-5 text-muted-foreground"
                  title={dataset?.description || t('datasets.noDescription')}
                >
                  {dataset?.description || t('datasets.noDescription')}
                </div>
              </div>
            </div>

            {canManage && dataset ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-8 shrink-0 gap-1.5 px-2 text-muted-foreground hover:text-foreground"
                onClick={() => setEditOpen(true)}
              >
                <Pencil className="h-4 w-4" />
                {t('datasets.actions.edit')}
              </Button>
            ) : null}
          </div>
          <div className="flex-1 overflow-hidden">
            <div className="h-full overflow-y-auto" id="dataset-content-area">
              {isAuthReady && <DatasetModelsPreloader />}
              <div className="h-full">{children}</div>
            </div>
          </div>
        </div>
        <EditDatasetDialog open={editOpen} onOpenChange={setEditOpen} dataset={dataset} />
      </div>
    </WorkspaceMismatchGuard>
  );
}
