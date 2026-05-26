'use client';

import { useState, useEffect } from 'react';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { Skeleton } from '@/components/ui/skeleton';
import {
  useMarketplacePluginDetail,
  useMarketplacePluginVersions,
  useInstallPluginFromMarketplace,
} from '@/hooks/use-plugins';
import { Check } from 'lucide-react';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';

interface PluginDetailModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pluginId: string | null;
}

export default function PluginDetailModal({
  open,
  onOpenChange,
  pluginId,
}: PluginDetailModalProps) {
  const t = useT('market');

  // Fetch plugin detail
  const { plugin: detailedPlugin, isLoading: isDetailLoading } = useMarketplacePluginDetail(
    pluginId,
    open
  );

  // Fetch plugin versions
  const { versions: pluginVersions, isLoading: isLoadingVersions } = useMarketplacePluginVersions(
    pluginId,
    {
      page: 1,
      page_size: 20,
    },
    open
  );

  const plugin = detailedPlugin;
  const isLoadingDetail = isDetailLoading || isLoadingVersions;
  const showLoading = isLoadingDetail || (!plugin && open);

  // Selected version state
  const [selectedVersionId, setSelectedVersionId] = useState<string | null>(null);

  // Set default selected version to first one when versions are loaded
  useEffect(() => {
    if (pluginVersions.length > 0 && !selectedVersionId) {
      setSelectedVersionId(pluginVersions[0].id);
    }
  }, [pluginVersions, selectedVersionId]);

  // Use hook for installation and status checking
  const {
    isInstalled,
    isInstalling,
    isUninstalling,
    installPlugin,
    uninstallPlugin,
    refetchStatus,
  } = useInstallPluginFromMarketplace(plugin?.id ?? null, selectedVersionId, open && !!plugin);

  // Handle install plugin
  const handleInstall = async () => {
    if (!plugin?.id) {
      toast.error(t('plugins.installError'));
      return;
    }

    if (!selectedVersionId) {
      toast.error(t('plugins.installError'));
      return;
    }

    try {
      await installPlugin(plugin.id, selectedVersionId);
      toast.success(t('plugins.installSuccess'));
      // Refetch installation status after successful installation
      await refetchStatus();
    } catch (error) {
      const errorMessage =
        error && typeof error === 'object' && 'message' in error
          ? (error as { message?: string }).message
          : t('plugins.installFailed');
      toast.error(errorMessage);
    }
  };

  // Handle uninstall plugin
  const handleUninstall = async () => {
    if (!selectedVersionId) {
      toast.error(t('plugins.uninstallFailed'));
      return;
    }

    try {
      await uninstallPlugin(selectedVersionId);
      toast.success(t('plugins.uninstallSuccess'));
      // Refetch installation status after successful uninstallation
      await refetchStatus();
    } catch (error) {
      const errorMessage =
        error && typeof error === 'object' && 'message' in error
          ? (error as { message?: string }).message
          : t('plugins.uninstallFailed');
      toast.error(errorMessage);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogHeader>
        <DialogTitle />
      </DialogHeader>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto p-0">
        {showLoading ? (
          <>
            {/* Loading Header Skeleton */}
            <div className="relative px-6 pt-6 pb-4 border-b">
              <div className="flex items-start gap-3">
                <Skeleton className="w-12 h-12 rounded-lg shrink-0" />
                <div className="flex-1 min-w-0 space-y-2">
                  <Skeleton className="h-6 w-48" />
                  <Skeleton className="h-4 w-32" />
                </div>
              </div>
            </div>
            {/* Loading Content Skeleton */}
            <DialogBody className="space-y-6">
              {/* Description Skeleton */}
              <div className="space-y-2">
                <Skeleton className="h-6 w-24" />
                <Skeleton className="h-4 w-full" />
              </div>

              {/* Version List Skeleton */}
              <div className="space-y-2">
                <Skeleton className="h-6 w-24" />
                <div className="grid grid-cols-2 gap-3">
                  {[...Array(2)].map((_, i) => (
                    <div key={i} className="p-4 border rounded-lg space-y-2">
                      <div className="flex items-center gap-2">
                        <Skeleton className="h-5 w-20" />
                        <Skeleton className="h-5 w-16" />
                      </div>
                      <Skeleton className="h-4 w-full" />
                      <Skeleton className="h-4 w-full" />
                      <Skeleton className="h-4 w-2/3" />
                      <div className="flex flex-col gap-1">
                        <Skeleton className="h-3 w-32" />
                        <Skeleton className="h-3 w-24" />
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </DialogBody>
            {/* Loading Footer Skeleton */}
            <DialogFooter className="px-6 py-4 border-t">
              <Skeleton className="h-10 w-20" />
              <Skeleton className="h-10 w-24" />
            </DialogFooter>
          </>
        ) : plugin ? (
          <>
            <div className="relative px-6 pt-6 pb-4 border-b">
              <div className="flex items-start gap-3">
                {/* Icon */}
                {plugin.icon ? (
                  <div className="w-10 h-10 sm:w-12 sm:h-12 flex items-center justify-center shrink-0 rounded-lg bg-muted overflow-hidden">
                    <img
                      src={plugin.icon}
                      alt={plugin.name}
                      className="w-full h-full object-contain"
                      onError={e => {
                        const target = e.target as HTMLImageElement;
                        target.style.display = 'none';
                        const parent = target.parentElement;
                        if (parent && plugin) {
                          parent.innerHTML = `<div class="w-full h-full flex items-center justify-center text-lg font-semibold text-muted-foreground">${plugin.name.slice(0, 2)}</div>`;
                        }
                      }}
                    />
                  </div>
                ) : (
                  <div className="w-10 h-10 sm:w-12 sm:h-12 flex items-center justify-center shrink-0 rounded-lg bg-muted text-muted-foreground">
                    <span className="text-lg font-semibold">{plugin.name.slice(0, 2)}</span>
                  </div>
                )}
                {/* Title and Author */}
                <div className="flex-1 min-w-0">
                  <h3 className="text-lg font-semibold">{plugin.name}</h3>
                  {plugin.developer.organization_name && (
                    <p className="text-sm text-muted-foreground mt-1">
                      {t('plugins.modal.by')} {plugin.developer.organization_name}
                    </p>
                  )}
                </div>
              </div>
            </div>

            <DialogBody className="space-y-6">
              {/* Description */}
              {plugin.description && (
                <div className="space-y-2">
                  <h3 className="text-lg font-semibold">{t('plugins.modal.description')}</h3>
                  <p className="text-sm text-muted-foreground leading-relaxed">
                    {plugin.description}
                  </p>
                </div>
              )}

              {/* Version List */}
              <div className="space-y-2">
                <h3 className="text-lg font-semibold">{t('plugins.modal.versions')}</h3>
                {pluginVersions.length > 0 ? (
                  <div className="grid grid-cols-2 gap-3 max-h-96 overflow-y-auto">
                    {pluginVersions.map(version => {
                      const isSelected = selectedVersionId === version.id;
                      return (
                        <div
                          key={version.id}
                          onClick={() => setSelectedVersionId(version.id)}
                          className={cn(
                            'p-4 border rounded-lg hover:bg-muted/50 transition-colors cursor-pointer relative',
                            isSelected && 'border-primary'
                          )}
                        >
                          {isSelected && (
                            <div className="absolute top-2 right-2">
                              <div className="w-5 h-5 rounded-full bg-primary flex items-center justify-center">
                                <Check className="h-3 w-3 text-white" />
                              </div>
                            </div>
                          )}
                          <div className="flex flex-col space-y-2">
                            <div className="flex items-center gap-2">
                              <span className="text-base font-semibold">{version.version}</span>
                              {version.status === 'published' && (
                                <span className="px-2 py-0.5 text-xs rounded-md bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
                                  {t('plugins.modal.published')}
                                </span>
                              )}
                            </div>
                            {version.changelog && (
                              <p className="text-sm text-muted-foreground whitespace-pre-line line-clamp-2">
                                {version.changelog}
                              </p>
                            )}
                            <div className="flex gap-1 text-xs text-muted-foreground">
                              {version.published_at && (
                                <span>
                                  {t('plugins.modal.publishedAt')}:{' '}
                                  {new Date(version.published_at).toLocaleDateString()}
                                </span>
                              )}
                              {version.package_size && (
                                <span>
                                  {t('plugins.modal.packageSize')}:{' '}
                                  {(version.package_size / 1024).toFixed(2)} KB
                                </span>
                              )}
                            </div>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground">{t('plugins.modal.noVersions')}</p>
                )}
              </div>
            </DialogBody>

            <DialogFooter className="px-6 py-4 border-t">
              <Button
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={isInstalling || isUninstalling}
              >
                {t('plugins.modal.close')}
              </Button>
              {isInstalled ? (
                <Button
                  onClick={handleUninstall}
                  variant="destructive"
                  disabled={isInstalling || isUninstalling}
                >
                  {isUninstalling ? t('plugins.modal.uninstalling') : t('plugins.modal.uninstall')}
                </Button>
              ) : (
                <Button
                  onClick={handleInstall}
                  className="bg-primary text-primary-foreground"
                  disabled={isInstalling || isUninstalling || !selectedVersionId}
                >
                  {isInstalling ? t('plugins.modal.installing') : t('plugins.modal.add')}
                </Button>
              )}
            </DialogFooter>
          </>
        ) : (
          <div className="px-6 py-6 text-center text-muted-foreground">
            <p>{t('plugins.modal.noPluginData')}</p>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
