import { BaseService } from '@/lib/http/services';
import { MARKETPLACE_CHANNEL } from '@/lib/config';
import type {
  InstalledPlugin,
  UninstallResult,
  MarketplacePluginCategory,
  MarketplacePluginListResponse,
  MarketplacePlugin,
  MarketplaceBrandingSettings,
  MarketplacePluginVersionListResponse,
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

  async getMarketplaceBrandingConfig(): Promise<MarketplaceBrandingSettings> {
    const response = await this.getMarketplaceBrandingSettings();
    const settings = response.data?.settings ?? {};

    return {
      official_logo_url: settings['marketplace.official_logo_url'],
      blue_v_icon_url: settings['marketplace.blue_v_icon_url'],
      yellow_v_icon_url: settings['marketplace.yellow_v_icon_url'],
      feedback_enabled: settings['marketplace.feedback_enabled'] !== 'false',
      feedback_url: settings['marketplace.feedback_url'],
      upload_application_enabled: settings['marketplace.upload_application_enabled'] !== 'false',
      upload_application_url: settings['marketplace.upload_application_url'],
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
}

export const pluginService = new PluginService();
export default pluginService;
