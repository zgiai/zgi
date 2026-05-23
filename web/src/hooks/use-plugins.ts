import { pluginService } from '@/services/plugin.service';
import type {
  MarketplaceBrandingSettings,
  MarketplacePlugin,
  MarketplacePluginCategory,
} from '@/services/types/plugin';

import { useCallback } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { PLUGIN_KEYS } from '@/hooks/query-keys';
import { useLocale } from '@/hooks/use-locale';

/**
 * Hook to get marketplace plugins using new API
 */
export interface UseMarketplacePluginsParams {
  page?: number;
  page_size?: number;
  category?: MarketplacePluginCategory;
  search?: string;
  developer_id?: string;
  locale?: string;
  sort?: 'downloads' | 'newest' | 'rating';
  is_featured?: boolean;
  is_official?: boolean;
}

export interface UseMarketplacePluginsOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
}

export interface UseMarketplacePluginsReturn {
  plugins: MarketplacePlugin[];
  total: number;
  page: number;
  page_size: number;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
}

export function useMarketplacePlugins(
  params: UseMarketplacePluginsParams,
  options: UseMarketplacePluginsOptions = {}
): UseMarketplacePluginsReturn {
  const { enabled = true, staleTime = 5 * 60 * 1000, gcTime = 10 * 60 * 1000 } = options;

  const query = useQuery({
    queryKey: PLUGIN_KEYS.marketplaceList(params),
    queryFn: async () => {
      const response = await pluginService.getMarketplacePlugins({
        page: params.page,
        page_size: params.page_size,
        category: params.category,
        search: params.search,
        developer_id: params.developer_id,
        locale: params.locale,
        sort: params.sort,
        is_featured: params.is_featured,
        is_official: params.is_official,
      });
      return response.data;
    },
    enabled,
    staleTime,
    gcTime,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
  });

  const data = query.data ?? {
    items: [],
    total: 0,
    page: 1,
    page_size: 20,
  };

  const errorMessage = (query.error as { message?: string } | null)?.message ?? null;

  return {
    plugins: data.items,
    total: data.total,
    page: data.page,
    page_size: data.page_size,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: errorMessage,
    refetch: async () => {
      await query.refetch();
    },
  };
}

export function useMarketplaceBranding(): MarketplaceBrandingSettings {
  const query = useQuery({
    queryKey: PLUGIN_KEYS.marketplaceBranding(),
    queryFn: () => pluginService.getMarketplaceBrandingConfig(),
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
  });

  return query.data ?? {};
}

/**
 * Hook to get marketplace plugin detail
 */
export function useMarketplacePluginDetail(id: string | null, enabled = true) {
  const { locale } = useLocale();

  const query = useQuery({
    queryKey: [...PLUGIN_KEYS.marketplaceDetail(id || ''), locale],
    queryFn: async () => {
      if (!id) return null;
      const response = await pluginService.getMarketplacePluginDetail(id, { locale });
      return response.data;
    },
    enabled: enabled && !!id,
    staleTime: 5 * 60 * 1000, // 5 minutes
    gcTime: 10 * 60 * 1000, // 10 minutes
  });

  return {
    plugin: query.data,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: (query.error as { message?: string } | null)?.message ?? null,
    refetch: query.refetch,
  };
}

/**
 * Hook to get marketplace plugin versions
 */
export function useMarketplacePluginVersions(
  pluginId: string | null,
  params?: {
    page?: number;
    page_size?: number;
  },
  enabled = true
) {
  const query = useQuery({
    queryKey: PLUGIN_KEYS.marketplaceVersions(pluginId || '', params || {}),
    queryFn: async () => {
      if (!pluginId) return null;
      const response = await pluginService.getMarketplacePluginVersions(pluginId, params);
      return response.data;
    },
    enabled: enabled && !!pluginId,
    staleTime: 5 * 60 * 1000, // 5 minutes
    gcTime: 10 * 60 * 1000, // 10 minutes
  });

  return {
    versions: query.data?.items ?? [],
    total: query.data?.total ?? 0,
    page: query.data?.page ?? 1,
    page_size: query.data?.page_size ?? 20,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: (query.error as { message?: string } | null)?.message ?? null,
    refetch: query.refetch,
  };
}

/**
 * Hook to install plugin from marketplace and check installation status
 */
export function useInstallPluginFromMarketplace(versionId: string | null, enabled = true) {
  const queryClient = useQueryClient();

  // Query to check installation status
  const installationStatusQuery = useQuery({
    queryKey: PLUGIN_KEYS.installationStatus(versionId || ''),
    queryFn: async () => {
      if (!versionId) return false;
      const response = await pluginService.getInstalledPlugins();
      const installedPluginsList = response.data || [];

      // Installation state is keyed by marketplace version id in the current UI
      const isInstalled = installedPluginsList.some(p => p.version_id === versionId);

      return isInstalled;
    },
    enabled: enabled && !!versionId,
    staleTime: 1 * 60 * 1000, // 1 minute
    gcTime: 5 * 60 * 1000, // 5 minutes
  });

  // Mutation to install plugin
  const installMutation = useMutation({
    mutationFn: async (params: { plugin_id: string; version_id: string }) => {
      return pluginService.installPluginFromMarketplace(params);
    },
    onSuccess: async () => {
      // Invalidate installation status query to refetch
      await queryClient.invalidateQueries({
        queryKey: PLUGIN_KEYS.installationStatus(versionId || ''),
      });
    },
  });

  // Mutation to uninstall plugin
  const uninstallMutation = useMutation({
    mutationFn: async (versionId: string) => {
      return pluginService.uninstallPluginByVersionId(versionId);
    },
    onSuccess: async () => {
      // Invalidate installation status query to refetch
      await queryClient.invalidateQueries({
        queryKey: PLUGIN_KEYS.installationStatus(versionId || ''),
      });
    },
  });

  // Function to install plugin
  const installPlugin = useCallback(
    async (pluginId: string, versionId: string) => {
      return installMutation.mutateAsync({ plugin_id: pluginId, version_id: versionId });
    },
    [installMutation]
  );

  // Function to uninstall plugin
  const uninstallPlugin = useCallback(
    async (versionId: string) => {
      return uninstallMutation.mutateAsync(versionId);
    },
    [uninstallMutation]
  );

  return {
    isInstalled: installationStatusQuery.data ?? false,
    isLoading: installationStatusQuery.isLoading,
    isInstalling: installMutation.isPending,
    isUninstalling: uninstallMutation.isPending,
    installPlugin,
    uninstallPlugin,
    error: (installationStatusQuery.error || installMutation.error || uninstallMutation.error) as {
      message?: string;
    } | null,
    refetchStatus: installationStatusQuery.refetch,
  };
}
