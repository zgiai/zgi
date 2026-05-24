import { BaseService } from '@/lib/http/services';
import { MARKETPLACE_CHANNEL } from '@/lib/config';
import type {
  InstalledPlugin,
  UninstallResult,
  MarketplacePluginCategory,
  MarketplaceCategory,
  MarketplacePluginListResponse,
  MarketplacePlugin,
  MarketplaceBrandingSettings,
  MarketplacePluginVersionListResponse,
  MarketplacePluginFavoriteStatus,
  SubmitMarketplacePluginFeedbackRequest,
} from './types/plugin';
import type { ApiResponseData } from './types/common';

/**
 * PluginService
 * ---------------------------------------------------------------------------
 * Handles all plugin-related API requests.
 * API Reference: see `docs/api/plugin-api.md`
 */
class PluginService extends BaseService {
  constructor() {
    super({
      endpoint: 'main',
      basePath: '/console/api/workspaces',
    });
  }

  /* ------------------------------------------------------------------------ */
  /* Install APIs                                                             */
  /* ------------------------------------------------------------------------ */

  /**
   * Install a plugin from marketplace for the current workspace
   * POST /console/api/workspaces/current/runner/management/plugins/install-from-marketplace
   */
  installPluginFromMarketplace(data: {
    plugin_id: string;
    version_id: string;
  }): Promise<ApiResponseData<{ result: string }>> {
    return this.request(
      'post',
      '/current/runner/management/plugins/install-from-marketplace',
      data
    );
  }

  /**
   * Get installed plugins for the current workspace
   * GET /console/api/workspaces/current/runner/management/plugins/installed
   */
  getInstalledPlugins(): Promise<ApiResponseData<InstalledPlugin[]>> {
    return this.request('get', '/current/runner/management/plugins/installed');
  }

  /**
   * Uninstall a plugin by version ID from the current workspace
   * DELETE /console/api/workspaces/current/runner/management/plugins/:id
   */
  uninstallPluginByVersionId(versionId: string): Promise<ApiResponseData<UninstallResult>> {
    return this.request('delete', `/current/runner/management/plugins/${versionId}`);
  }

  /**
   * Get plugins from marketplace
   */
  getMarketplacePlugins(params: {
    page?: number;
    page_size?: number;
    category?: MarketplacePluginCategory;
    search?: string;
    developer_id?: string;
    locale?: string;
    sort?: 'downloads' | 'newest';
    is_featured?: boolean;
    is_official?: boolean;
  }): Promise<ApiResponseData<MarketplacePluginListResponse>> {
    return this.request('get', 'v1/market/plugins', undefined, {
      skipAuth: true,
      skipErrorHandling: true,
      endpoint: 'market',
      timeout: 60000,
      params: {
        page: params.page?.toString(),
        page_size: params.page_size?.toString(),
        category: params.category,
        search: params.search,
        developer_id: params.developer_id,
        locale: params.locale,
        sort: params.sort,
        is_featured: params.is_featured !== undefined ? params.is_featured.toString() : undefined,
        is_official: params.is_official !== undefined ? params.is_official.toString() : undefined,
        channel: MARKETPLACE_CHANNEL || undefined,
      },
    });
  }

  /**
   * Get visible marketplace categories.
   */
  getMarketplaceCategories(params?: {
    locale?: string;
  }): Promise<ApiResponseData<{ items: MarketplaceCategory[] }>> {
    return this.request('get', 'v1/market/categories', undefined, {
      skipAuth: true,
      skipErrorHandling: true,
      endpoint: 'market',
      timeout: 60000,
      params: {
        locale: params?.locale,
      },
    });
  }

  submitMarketplacePluginFeedback(
    data: SubmitMarketplacePluginFeedbackRequest
  ): Promise<ApiResponseData<{ id: string }>> {
    return this.request('post', 'v1/market/feedback', data, {
      skipAuth: true,
      skipErrorHandling: true,
      endpoint: 'market',
      timeout: 60000,
    });
  }

  /**
   * Get public marketplace branding settings from console.
   * GET /v1/public/settings
   */
  getMarketplaceBrandingSettings(): Promise<
    ApiResponseData<{ settings: Record<string, string>; updated: string }>
  > {
    return this.request('get', 'v1/public/settings', undefined, {
      skipAuth: true,
      skipErrorHandling: true,
      endpoint: 'market',
      timeout: 60000,
    });
  }

  async getMarketplaceBrandingConfig(locale?: string): Promise<MarketplaceBrandingSettings> {
    const response = await this.getMarketplaceBrandingSettings();
    const settings = response.data?.settings ?? {};
    const suffix = locale?.toLowerCase().startsWith('en') ? 'en_us' : 'zh_hans';
    const localizedTip = (baseKey: string) =>
      settings[`marketplace.metrics.${baseKey}_tip_${suffix}`] ||
      settings[`marketplace.metrics.${baseKey}_tip`];

    return {
      official_logo_url: settings['marketplace.official_logo_url'],
      blue_v_icon_url: settings['marketplace.blue_v_icon_url'],
      yellow_v_icon_url: settings['marketplace.yellow_v_icon_url'],
      feedback_enabled: settings['marketplace.feedback_enabled'] !== 'false',
      upload_application_enabled: settings['marketplace.upload_application_enabled'] !== 'false',
      metric_icon_urls: {
        downloads: settings['marketplace.metrics.download_icon_url'],
        runs: settings['marketplace.metrics.run_icon_url'],
        runtime: settings['marketplace.metrics.runtime_icon_url'],
        success: settings['marketplace.metrics.success_icon_url'],
        favorites: settings['marketplace.metrics.favorite_icon_url'],
      },
      metric_enabled: {
        downloads: settings['marketplace.metrics.downloads_enabled'] !== 'false',
        runs: settings['marketplace.metrics.runs_enabled'] !== 'false',
        runtime: settings['marketplace.metrics.runtime_enabled'] !== 'false',
        success: settings['marketplace.metrics.success_enabled'] !== 'false',
        favorites: settings['marketplace.metrics.favorites_enabled'] !== 'false',
      },
      metric_base_values: {
        downloads: parseNumberSetting(settings['marketplace.metrics.download_base']),
        runs: parseNumberSetting(settings['marketplace.metrics.run_base']),
        favorites: parseNumberSetting(settings['marketplace.metrics.favorite_base']),
      },
      metric_tips: {
        downloads: localizedTip('download'),
        runs: localizedTip('run'),
        runtime: localizedTip('runtime'),
        success: localizedTip('success'),
        favorites: localizedTip('favorite'),
      },
    };
  }

  /**
   * Get plugin detail from marketplace
   * GET /v1/market/plugins/{id}
   */
  getMarketplacePluginDetail(
    id: string,
    params?: {
      locale?: string;
    }
  ): Promise<ApiResponseData<MarketplacePlugin>> {
    return this.request('get', `v1/market/plugins/${id}`, undefined, {
      skipAuth: true,
      skipErrorHandling: true,
      endpoint: 'market',
      params: {
        locale: params?.locale,
      },
    });
  }

  /**
   * Get plugin versions from marketplace
   * GET /v1/market/plugins/{plugin_id}/versions
   */
  getMarketplacePluginVersions(
    pluginId: string,
    params?: {
      page?: number;
      page_size?: number;
    }
  ): Promise<ApiResponseData<MarketplacePluginVersionListResponse>> {
    return this.request('get', `v1/market/plugins/${pluginId}/versions`, undefined, {
      skipAuth: true,
      skipErrorHandling: true,
      endpoint: 'market',
      params: {
        page: params?.page,
        page_size: params?.page_size,
      },
    });
  }

  getMarketplacePluginFavoriteStatus(
    pluginId: string,
    submitterId?: string | null
  ): Promise<ApiResponseData<MarketplacePluginFavoriteStatus>> {
    return this.request('get', `v1/market/plugins/${pluginId}/favorite`, undefined, {
      skipAuth: true,
      skipErrorHandling: true,
      endpoint: 'market',
      params: { submitter_id: submitterId || undefined },
    });
  }

  favoriteMarketplacePlugin(
    pluginId: string,
    submitterId: string
  ): Promise<ApiResponseData<MarketplacePluginFavoriteStatus>> {
    return this.request(
      'post',
      `v1/market/plugins/${pluginId}/favorite`,
      { submitter_id: submitterId },
      {
        skipAuth: true,
        skipErrorHandling: true,
        endpoint: 'market',
      }
    );
  }

  unfavoriteMarketplacePlugin(
    pluginId: string,
    submitterId: string
  ): Promise<ApiResponseData<MarketplacePluginFavoriteStatus>> {
    return this.request('delete', `v1/market/plugins/${pluginId}/favorite`, undefined, {
      skipAuth: true,
      skipErrorHandling: true,
      endpoint: 'market',
      params: { submitter_id: submitterId },
    });
  }
}

export const pluginService = new PluginService();
export default pluginService;

function parseNumberSetting(value?: string) {
  const parsed = Number(value || 0);
  return Number.isFinite(parsed) ? parsed : 0;
}
